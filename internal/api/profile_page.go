package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverperson"
)

// profileSegmentPageSize SPEC <discovery> Profile page: 10 items per page.
const profileSegmentPageSize = 10

// AtIdentifierResolver resolves a handle-or-DID AT identifier to a bidirectionally-verified identity; identity.Directory satisfies it via Lookup.
type AtIdentifierResolver interface {
	Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error)
}

// PersonRecordsInspector is the narrow slice of DiscoverPersonInspector the profile page needs; *discoverperson.Inspector satisfies it directly.
type PersonRecordsInspector interface {
	Records(ctx context.Context, did string, viewerKeys map[string]struct{}) discoverperson.Records
}

// ProfileCountsWire is the profile header's write/read/share counts, each the length of that section's deduped Records list.
type ProfileCountsWire struct {
	Writes int `json:"writes"`
	Reads  int `json:"reads"`
	Shares int `json:"shares"`
}

// ProfileWire is the profile page header. FollowRkey is an explicit null when the viewer doesn't follow this person, not omitted, so the frontend never has to distinguish "missing field" from "not followed".
type ProfileWire struct {
	DID         string            `json:"did"`
	Handle      string            `json:"handle"`
	DisplayName string            `json:"displayName,omitempty"`
	Avatar      string            `json:"avatar,omitempty"`
	Description string            `json:"description,omitempty"`
	IsSelf      bool              `json:"isSelf"`
	FollowRkey  *string           `json:"followRkey"`
	Counts      ProfileCountsWire `json:"counts"`
}

// profileSourceSegmentWire is one page of a writes or reads segment.
type profileSourceSegmentWire struct {
	Items      []DiscoverPersonSourceWire `json:"items"`
	NextCursor string                     `json:"nextCursor,omitempty"`
}

// profileShareSegmentWire is one page of the shares segment.
type profileShareSegmentWire struct {
	Items      []DiscoverPersonShareWire `json:"items"`
	NextCursor string                    `json:"nextCursor,omitempty"`
}

// ProfileHandler serves a person's profile header: identity, follow state, and section counts. {id} accepts a handle or a DID; either way it's bidirectionally verified through the identity directory before anything is read. SPEC <discovery> Profile page.
func ProfileHandler(resolver AtIdentifierResolver, src ProfileSource, follows FollowsIndexReader, subs DiscoverSubscriptionsReader, inspector PersonRecordsInspector) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}

		ident, ok := resolveProfileIdentity(w, r, resolver)
		if !ok {
			return
		}
		didStr := ident.DID.String()
		selfDIDStr := sess.Data.AccountDID.String()
		isSelf := didStr == selfDIDStr

		var profile profiles.Profile
		var err error
		if isSelf {
			profile, err = src.Refresh(r.Context(), ident.DID)
		} else {
			profile, err = src.Get(r.Context(), ident.DID)
		}
		if err != nil {
			slog.Warn("/api/profile/{id}: profile load failed", "did", didStr, "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "could not resolve identity")
			return
		}

		var followRkey *string
		if !isSelf {
			if row, err := follows.GetUserFollowBySubjectDID(r.Context(), db.GetUserFollowBySubjectDIDParams{
				Did:        selfDIDStr,
				SubjectDid: didStr,
			}); err == nil {
				rkey := row.Rkey
				followRkey = &rkey
			} else if !errors.Is(err, sql.ErrNoRows) {
				slog.Warn("/api/profile/{id}: follow lookup failed", "err", err)
				writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
				return
			}
		}

		viewerKeys, ok := loadViewerKeys(w, r, subs, selfDIDStr)
		if !ok {
			return
		}
		records := inspector.Records(r.Context(), didStr, viewerKeys)

		writeJSON(w, ProfileWire{
			DID:         didStr,
			Handle:      profile.Handle,
			DisplayName: derefOptString(profile.DisplayName),
			Avatar:      derefOptString(profile.Avatar),
			Description: derefOptString(profile.Description),
			IsSelf:      isSelf,
			FollowRkey:  followRkey,
			Counts: ProfileCountsWire{
				Writes: len(records.Writes),
				Reads:  len(records.Reads),
				Shares: len(records.Shares),
			},
		})
	})
}

// ProfileSegmentHandler serves one paginated segment (writes, reads, or shares) of a person's records. Any other segment value 404s, same as an unresolvable identity, so the two "not found" cases are indistinguishable to the client. SPEC <discovery> Profile page.
func ProfileSegmentHandler(resolver AtIdentifierResolver, subs DiscoverSubscriptionsReader, inspector PersonRecordsInspector, metadata ShareMetadataResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}

		segment := r.PathValue("segment")
		if segment != "writes" && segment != "reads" && segment != "shares" {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return
		}

		ident, ok := resolveProfileIdentity(w, r, resolver)
		if !ok {
			return
		}
		didStr := ident.DID.String()

		viewerKeys, ok := loadViewerKeys(w, r, subs, sess.Data.AccountDID.String())
		if !ok {
			return
		}
		records := inspector.Records(r.Context(), didStr, viewerKeys)
		cursor := r.URL.Query().Get("cursor")

		switch segment {
		case "writes":
			items, next := discoverperson.Page(records.Writes, cursor, profileSegmentPageSize)
			writeJSON(w, profileSourceSegmentWire{Items: discoverPersonSourceWires(items), NextCursor: next})
		case "reads":
			items, next := discoverperson.Page(records.Reads, cursor, profileSegmentPageSize)
			writeJSON(w, profileSourceSegmentWire{Items: discoverPersonSourceWires(items), NextCursor: next})
		case "shares":
			items, next := discoverperson.Page(records.Shares, cursor, profileSegmentPageSize)
			writeJSON(w, profileShareSegmentWire{Items: discoverPersonShareWires(r.Context(), metadata, items), NextCursor: next})
		}
	})
}

// resolveProfileIdentity parses {id} as a handle or DID and bidirectionally resolves it; any failure collapses to 404 so "malformed" and "doesn't exist" don't leak the distinction.
func resolveProfileIdentity(w http.ResponseWriter, r *http.Request, resolver AtIdentifierResolver) (*identity.Identity, bool) {
	raw := r.PathValue("id")
	atid, err := syntax.ParseAtIdentifier(raw)
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return nil, false
	}
	ident, err := resolver.Lookup(r.Context(), atid)
	if err != nil {
		slog.Warn("resolveProfileIdentity: lookup failed", "id", raw, "err", err)
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return nil, false
	}
	return ident, true
}

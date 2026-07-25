package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
)

const followCollection = lexicon.Follow

// FollowWire is the on-the-wire follow shape; handle/avatar/displayName are omitted, the frontend joins them per-DID via /api/profiles/{did}.
type FollowWire struct {
	URI        string `json:"uri"`
	Rkey       string `json:"rkey"`
	SubjectDID string `json:"subjectDid"`
	CreatedAt  string `json:"createdAt"`
}

// FollowsIndexReader is the slice of *db.Queries the follow handlers read from.
type FollowsIndexReader interface {
	GetUserFollow(ctx context.Context, arg db.GetUserFollowParams) (db.UserFollow, error)
	GetUserFollowBySubjectDID(ctx context.Context, arg db.GetUserFollowBySubjectDIDParams) (db.UserFollow, error)
	ListUserFollows(ctx context.Context, did string) ([]db.UserFollow, error)
}

// FollowsIndexWriter is the slice used for writes.
type FollowsIndexWriter interface {
	UpsertUserFollow(ctx context.Context, arg db.UpsertUserFollowParams) error
	DeleteUserFollow(ctx context.Context, arg db.DeleteUserFollowParams) error
}

// HandleResolver resolves a handle to a full identity, bidirectionally verified.
type HandleResolver interface {
	LookupHandle(ctx context.Context, handle syntax.Handle) (*identity.Identity, error)
}

// --- POST /api/follows ---

type followsCreateRequest struct {
	Handle string `json:"handle"`
}

// FollowsCreateHandler writes a blue.morgen.graph.follow record, idempotent on (did, subjectDid). SPEC <social-layer>: never touches subscriptions or the digest.
func FollowsCreateHandler(reader FollowsIndexReader, writer FollowsIndexWriter, pds atprepo.Writer, resolver HandleResolver, disp RepairDispatcher, memo DiscoverInvalidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body followsCreateRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		raw := strings.TrimPrefix(strings.TrimSpace(body.Handle), "@")
		if raw == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "handle is required")
			return
		}
		handle, err := syntax.ParseHandle(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "that doesn't look like a handle")
			return
		}

		ident, err := resolver.LookupHandle(r.Context(), handle)
		if err != nil {
			slog.Warn("/api/follows: handle resolution failed", "handle", handle, "err", err)
			writeError(w, http.StatusUnprocessableEntity, codeUnprocessable, "couldn't find that handle")
			return
		}
		subjectDID := ident.DID.String()
		didStr := sess.Data.AccountDID.String()
		if subjectDID == didStr {
			writeError(w, http.StatusUnprocessableEntity, codeUnprocessable, "you can't follow yourself")
			return
		}

		if existing, err := reader.GetUserFollowBySubjectDID(r.Context(), db.GetUserFollowBySubjectDIDParams{
			Did:        didStr,
			SubjectDid: subjectDID,
		}); err == nil {
			writeJSON(w, followRowToWire(existing))
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("/api/follows: dedupe probe failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		record := map[string]any{
			"subject":   subjectDID,
			"createdAt": now,
		}
		if err := lexicon.ValidateRecord(followCollection, record); err != nil {
			slog.Warn("/api/follows: record failed lexicon validation", "err", err)
			writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
			return
		}
		ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(followCollection), record)
		if err != nil {
			slog.Warn("/api/follows: PDS create failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		rkey := atprepo.RkeyFromATURI(ref.URI)

		mirrorOrRepair(r.Context(), disp, sess, "/api/follows: Tier-1 upsert", func() error {
			return writer.UpsertUserFollow(r.Context(), db.UpsertUserFollowParams{
				Did:        didStr,
				Rkey:       rkey,
				AtUri:      ref.URI,
				SubjectDid: subjectDID,
				CreatedAt:  now,
				UpdatedAt:  now,
			})
		})
		invalidateDiscover(memo, didStr)

		writeJSONStatus(w, http.StatusCreated, FollowWire{
			URI:        ref.URI,
			Rkey:       rkey,
			SubjectDID: subjectDID,
			CreatedAt:  now,
		})
	})
}

// --- DELETE /api/follows/{rkey} ---

// FollowsRepoLister is the PDS surface the DELETE handler needs: writes plus listing, to sweep duplicate follow records for the same subject.
type FollowsRepoLister interface {
	atprepo.Writer
	atprepo.Lister
}

// FollowsDeleteHandler tombstones every follow record for the subject, not just the given rkey: two devices can each write a
// duplicate before syncing, and a leftover would let the next reconcile resurrect the follow the user just removed.
func FollowsDeleteHandler(reader FollowsIndexReader, writer FollowsIndexWriter, pds FollowsRepoLister, disp RepairDispatcher, memo DiscoverInvalidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "rkey is required")
			return
		}
		didStr := sess.Data.AccountDID.String()
		row, err := reader.GetUserFollow(r.Context(), db.GetUserFollowParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Collapse "not yours" and "doesn't exist" to 404 to avoid leaking existence.
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/follows DELETE: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(followCollection))
		if err != nil {
			slog.Warn("/api/follows DELETE: list failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		for _, rec := range records {
			if subject, _ := rec.Value["subject"].(string); subject != row.SubjectDid {
				continue
			}
			if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(followCollection), atprepo.RkeyFromATURI(rec.URI)); err != nil {
				slog.Warn("/api/follows DELETE: PDS delete failed", "uri", rec.URI, "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return
			}
		}

		mirrorOrRepair(r.Context(), disp, sess, "/api/follows DELETE: Tier-1 delete", func() error {
			return writer.DeleteUserFollow(r.Context(), db.DeleteUserFollowParams{Did: didStr, Rkey: rkey})
		})
		invalidateDiscover(memo, didStr)
		w.WriteHeader(http.StatusNoContent)
	})
}

// --- GET /api/follows ---

// FollowsListHandler returns who the user follows, served from the local Tier-1 index so it survives reload without a PDS round-trip.
func FollowsListHandler(reader FollowsIndexReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rows, err := reader.ListUserFollows(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/follows GET: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		out := make([]FollowWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, followRowToWire(row))
		}
		writeJSON(w, out)
	})
}

func followRowToWire(row db.UserFollow) FollowWire {
	return FollowWire{
		URI:        row.AtUri,
		Rkey:       row.Rkey,
		SubjectDID: row.SubjectDid,
		CreatedAt:  row.CreatedAt,
	}
}

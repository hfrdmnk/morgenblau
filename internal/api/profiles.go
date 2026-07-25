package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
)

const (
	// maxBatchProfiles caps one /api/profiles request; the frontend chunks its DID list to match.
	maxBatchProfiles = 50
	// profileBatchFanoutLimit bounds concurrent cache misses so one wide batch can't flood the PDS.
	profileBatchFanoutLimit = 8
)

// ProfileSource is the slice of *profiles.Cache the handlers depend on.
type ProfileSource interface {
	Get(ctx context.Context, did syntax.DID) (profiles.Profile, error)
	Refresh(ctx context.Context, did syntax.DID) (profiles.Profile, error)
}

// meResponse is the session user's profile plus session-health flags for calm prompting.
type meResponse struct {
	profiles.Profile
	// NeedsReauth is true when the session predates the standardfeed scopes; standard-record writes will 403 until the user re-logs-in.
	NeedsReauth bool `json:"needsReauth"`
}

// MeProfileHandler returns the session user's profile plus needsReauth; cache-first, because every hard page load blocks on this call and a PDS round-trip would stall the shell.
func MeProfileHandler(src ProfileSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		p, err := src.Get(r.Context(), sess.Data.AccountDID)
		if err != nil {
			if errors.Is(err, profiles.ErrHandleInvalid) {
				slog.Warn("/api/profiles/me: handle.invalid", "did", sess.Data.AccountDID)
			} else {
				slog.Warn("/api/profiles/me: profile load failed", "did", sess.Data.AccountDID, "err", err)
			}
			writeError(w, http.StatusInternalServerError, codeInternalError, "could not resolve identity")
			return
		}
		writeJSON(w, meResponse{Profile: p, NeedsReauth: !scopes.HasStandardWrite(sess)})
	})
}

// ProfileByDIDHandler resolves any DID through the cache, delegating to self-bypass when it's the session's own DID so results stay consistent with /api/profiles/me.
func ProfileByDIDHandler(src ProfileSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.PathValue("did")
		did, err := syntax.ParseDID(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid did")
			return
		}

		sess := auth.SessionFromContext(r.Context())
		selfBypass := sess != nil && sess.Data != nil && sess.Data.AccountDID == did

		var profile profiles.Profile
		if selfBypass {
			profile, err = src.Refresh(r.Context(), did)
		} else {
			profile, err = src.Get(r.Context(), did)
		}
		if err != nil {
			slog.Warn("/api/profiles/{did}: profile load failed", "did", did, "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "could not resolve identity")
			return
		}
		writeJSON(w, profile)
	})
}

// profilesBatchResponse keys every requested DID to its profile; an unresolvable DID keeps its key with a null value so the client can tell "asked and failed" from "never asked".
type profilesBatchResponse struct {
	Profiles map[string]*profiles.Profile `json:"profiles"`
}

// ProfilesBatchHandler resolves up to maxBatchProfiles DIDs through the cache in one round-trip, sparing list views a request per row. Cache-first: one stale profile is cheaper than a fan-out of PDS calls.
func ProfilesBatchHandler(src ProfileSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dids, ok := parseBatchDIDs(w, r.URL.Query().Get("dids"))
		if !ok {
			return
		}

		// slots[i] is written only by goroutine i, so no lock is needed.
		slots := make([]*profiles.Profile, len(dids))
		g, gctx := errgroup.WithContext(r.Context())
		g.SetLimit(profileBatchFanoutLimit)
		for i, did := range dids {
			g.Go(func() error {
				p, err := src.Get(gctx, did)
				if err != nil {
					slog.Warn("/api/profiles: profile load failed", "did", did, "err", err)
					return nil // one unresolvable DID degrades to null, never fails the batch
				}
				slots[i] = &p
				return nil
			})
		}
		_ = g.Wait()

		out := make(map[string]*profiles.Profile, len(dids))
		for i, did := range dids {
			out[did.String()] = slots[i]
		}
		writeJSON(w, profilesBatchResponse{Profiles: out})
	})
}

// parseBatchDIDs splits the comma-separated ?dids= list, rejecting an empty or oversized batch and any malformed DID. The client percent-encodes each DID individually, so query decoding has already run by the time we split.
func parseBatchDIDs(w http.ResponseWriter, raw string) ([]syntax.DID, bool) {
	if raw == "" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "dids is required")
		return nil, false
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxBatchProfiles {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "too many dids")
		return nil, false
	}
	dids := make([]syntax.DID, len(parts))
	for i, part := range parts {
		did, err := syntax.ParseDID(part)
		if err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid did")
			return nil, false
		}
		dids[i] = did
	}
	return dids, true
}

package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
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

// MeProfileHandler returns the session user's profile plus needsReauth; always refreshes so Bluesky profile edits surface immediately.
func MeProfileHandler(src ProfileSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		p, err := src.Refresh(r.Context(), sess.Data.AccountDID)
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

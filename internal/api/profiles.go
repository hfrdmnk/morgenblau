package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/oauth/scopes"
)

// ProfileSource is the slice of *profiles.Cache the handlers depend on.
type ProfileSource interface {
	Get(ctx context.Context, did syntax.DID) (profiles.Profile, error)
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
		writeJSON(w, meResponse{Profile: p, NeedsReauth: !scopes.HasStandardSubscriptionWrite(sess)})
	})
}

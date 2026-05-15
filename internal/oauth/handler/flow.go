package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
)

// ClientApp is the slice of indigo's *oauth.ClientApp this package depends on.
// Keeping it an interface lets handler tests stub the OAuth dance without
// reaching for a real authorization server.
type ClientApp interface {
	StartAuthFlow(ctx context.Context, identifier string) (string, error)
	ProcessCallback(ctx context.Context, params url.Values) (*oauth.ClientSessionData, error)
	Logout(ctx context.Context, did syntax.DID, sessionID string) error
}

// LoginSyncStarter fires a fire-and-forget refresh job after a successful
// OAuth callback. Optional — when nil, the callback simply skips the sync.
type LoginSyncStarter interface {
	StartLoginRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

// LoginHandler reads the `handle` form field, kicks off the OAuth dance,
// and redirects the user to the AS authorize URL.
func LoginHandler(app ClientApp) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		handle := strings.TrimSpace(r.PostFormValue("handle"))
		if handle == "" {
			http.Error(w, "handle is required", http.StatusBadRequest)
			return
		}
		redirectURL, err := app.StartAuthFlow(r.Context(), handle)
		if err != nil {
			slog.Warn("StartAuthFlow failed", "err", err)
			http.Error(w, "could not start sign-in: "+err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
	})
}

// CallbackHandler completes the OAuth dance and sets the session cookie.
// When a LoginSyncStarter is provided, the callback fires a sync_user job
// (trigger=login) so the refresh pill on /consume picks it up immediately.
func CallbackHandler(app ClientApp, sealer *cookie.Sealer, starter LoginSyncStarter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := app.ProcessCallback(r.Context(), r.URL.Query())
		if err != nil {
			var asErr *oauth.AuthRequestCallbackError
			if errors.As(err, &asErr) {
				slog.Warn("authorization server returned error", "code", asErr.ErrorCode, "desc", asErr.ErrorDescription)
			} else {
				slog.Warn("ProcessCallback failed", "err", err)
			}
			http.Error(w, "could not complete sign-in", http.StatusBadRequest)
			return
		}
		sealer.Set(w, sess.AccountDID.String(), sess.SessionID)
		if starter != nil {
			// Fire-and-forget. Don't block the redirect on a network call.
			if _, err := starter.StartLoginRefresh(r.Context(), sess.AccountDID, sess.SessionID); err != nil {
				slog.Warn("login refresh dispatch failed", "err", err)
			}
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

// LogoutHandler revokes at the AS (best-effort), deletes the server-side
// session, clears the cookie, and redirects to /. The public welcome lives
// there; an authed user would be bounced from /login by the route table.
func LogoutHandler(app ClientApp, sealer *cookie.Sealer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		didStr, sid, ok := sealer.Get(r)
		if ok {
			did, err := syntax.ParseDID(didStr)
			if err == nil {
				if err := app.Logout(r.Context(), did, sid); err != nil {
					slog.Warn("Logout failed (cookie cleared anyway)", "err", err)
				}
			}
		}
		sealer.Clear(w)
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

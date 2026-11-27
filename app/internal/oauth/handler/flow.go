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

	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/cookie"
)

type ClientApp interface {
	StartAuthFlow(ctx context.Context, identifier string) (string, error)
	ProcessCallback(ctx context.Context, params url.Values) (*oauth.ClientSessionData, error)
	Logout(ctx context.Context, did syntax.DID, sessionID string) error
}

type LoginSyncStarter interface {
	StartLoginRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}

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
			query := url.Values{"error": {"handle"}, "handle": {handle}}
			http.Redirect(w, r, "/login?"+query.Encode(), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
	})
}

func CallbackHandler(app ClientApp, sealer *cookie.Sealer, starter LoginSyncStarter, allowed auth.Allowlist) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := app.ProcessCallback(r.Context(), r.URL.Query())
		if err != nil {
			var asErr *oauth.AuthRequestCallbackError
			if errors.As(err, &asErr) {
				slog.Warn("authorization server returned error", "code", asErr.ErrorCode, "desc", asErr.ErrorDescription)
			} else {
				slog.Warn("ProcessCallback failed", "err", err)
			}
			http.Redirect(w, r, "/login?error=callback", http.StatusSeeOther)
			return
		}
		if !allowed.Allows(sess.AccountDID) {
			if err := app.Logout(r.Context(), sess.AccountDID, sess.SessionID); err != nil {
				slog.Warn("denied alpha session cleanup failed", "err", err)
			}
			http.Error(w, "This account is not invited to the Morgenblau alpha.", http.StatusForbidden)
			return
		}
		sealer.Set(w, sess.AccountDID.String(), sess.SessionID)
		if starter != nil {
			if _, err := starter.StartLoginRefresh(r.Context(), sess.AccountDID, sess.SessionID); err != nil {
				slog.Warn("login refresh dispatch failed", "err", err)
			}
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

type LogoutClient interface {
	Logout(context.Context, syntax.DID, string) error
}

func LogoutHandler(app LogoutClient, sealer *cookie.Sealer, locker SessionLocker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		didStr, sid, ok := sealer.Get(r)
		if ok {
			did, err := syntax.ParseDID(didStr)
			if err == nil {
				if locker != nil {
					unlock := locker.LockSession(did, sid)
					defer unlock()
				}
				if err := app.Logout(r.Context(), did, sid); err != nil {
					slog.Warn("Logout failed (cookie cleared anyway)", "err", err)
				}
			}
		}
		sealer.Clear(w)
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}

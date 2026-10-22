package handler

import (
	"context"
	"log/slog"
	"net/http"

	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/session"
)

type DevLogin interface {
	LoginDev(context.Context) (*session.Session, error)
}

func DevLoginHandler(login DevLogin, sealer *cookie.Sealer, starter LoginSyncStarter, allowed auth.Allowlist) http.Handler {
	return http.NewCrossOriginProtection().Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"enabled":true}`))
		case http.MethodPost:
			sess, err := login.LoginDev(r.Context())
			if err != nil {
				http.Error(w, "Could not sign in to the development account. Check the server credentials and try again.", http.StatusBadGateway)
				return
			}
			if !allowed.Allows(sess.Data.AccountDID) {
				http.Error(w, "This account is not invited to the Morgenblau alpha.", http.StatusForbidden)
				return
			}
			sealer.Set(w, sess.Data.AccountDID.String(), sess.Data.SessionID)
			if starter != nil {
				if _, err := starter.StartLoginRefresh(r.Context(), sess.Data.AccountDID, sess.Data.SessionID); err != nil {
					slog.Warn("development login refresh dispatch failed", "err", err)
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
}

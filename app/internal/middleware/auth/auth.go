package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/session"
)

type Resumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*session.Session, error)
}

type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}

type Middleware func(http.Handler) http.Handler

const maxAPIBodyBytes = 1 << 20

const maxFeedbackBodyBytes = 13 << 20

func New(resumer Resumer, locker SessionLocker, sealer *cookie.Sealer, allowed Allowlist, admins Admins) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			if path == "/api/feedback" {
				r.Body = http.MaxBytesReader(w, r.Body, maxFeedbackBodyBytes)
			} else if strings.HasPrefix(path, "/api/") {
				r.Body = http.MaxBytesReader(w, r.Body, maxAPIBodyBytes)
			}

			var sess *session.Session
			didStr, sid, ok := sealer.Get(r)
			if ok {
				if did, err := syntax.ParseDID(didStr); err == nil && allowed.Allows(did) {
					if holdsSessionLock(r) {
						unlock := locker.LockSession(did, sid)
						defer unlock()
					}
					s, err := resumer.ResumeSession(r.Context(), did, sid)
					switch {
					case err == nil:
						sess = s
					case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
						slog.Debug("resume session transient", "err", err)
					default:
						sealer.Clear(w)
					}
				} else {
					sealer.Clear(w)
				}
			}

			if IsAdminPath(path) {
				serveAdmin(next, w, r, sess, admins)
				return
			}

			if redirect, isPublic := publicRoutes[path]; isPublic {
				if sess != nil && redirect != "" {
					http.Redirect(w, r, redirect, http.StatusFound)
					return
				}
				serve(next, w, r, sess)
				return
			}

			if isInfra(path) || isPublicPrefix(path) {
				serve(next, w, r, sess)
				return
			}

			if sess != nil {
				serve(next, w, r, sess)
				return
			}
			refuseUnauthenticated(w, r)
		})
	}
}

func refuseUnauthenticated(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeUnauthenticated(w)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func serveAdmin(next http.Handler, w http.ResponseWriter, r *http.Request, sess *session.Session, admins Admins) {
	switch {
	case sess == nil:
		refuseUnauthenticated(w, r)
	case !admins.Includes(sess.Data.AccountDID):
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":"not_found","message":"not found"}`+"\n")
	default:
		next.ServeHTTP(w, r.WithContext(WithAdmin(WithSession(r.Context(), sess))))
	}
}

func writeUnauthenticated(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = io.WriteString(w, `{"code":"unauthenticated","message":"Your session ended. Sign in again."}`+"\n")
}

func holdsSessionLock(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	p := r.URL.Path
	switch p {
	case "/api/subscriptions/resolve", "/api/subscriptions/preview", "/api/subscriptions/preview/article", "/api/subscriptions/profile":
		return false
	}
	return strings.HasPrefix(p, "/api/subscriptions") ||
		strings.HasPrefix(p, "/api/saves")
}

func serve(next http.Handler, w http.ResponseWriter, r *http.Request, sess *session.Session) {
	if sess != nil {
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
		return
	}
	next.ServeHTTP(w, r)
}

var publicRoutes = map[string]string{
	"/login": "/",
	"/about": "",
}

var publicPrefixes = []string{
	"/feed/",
	"/web/",
	"/user/",
}

func isPublicPrefix(path string) bool {
	for _, p := range publicPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

var infraExact = map[string]struct{}{
	"/api/health":     {},
	"/oauth/login":    {},
	"/oauth/callback": {},
	"/oauth/logout":   {},
}

var viteDevServerPrefixes = []string{"/@", "/node_modules/", "/src/"}

var infraPrefixes = append([]string{"/assets/"}, viteDevServerPrefixes...)

func isInfra(path string) bool {
	if _, ok := infraExact[path]; ok {
		return true
	}
	for _, p := range infraPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	if strings.HasPrefix(path, "/api/") {
		return false
	}
	last := strings.LastIndex(path, "/")
	if last >= 0 && strings.Contains(path[last:], ".") {
		return true
	}
	return false
}

type contextKey struct{}

func WithSession(ctx context.Context, sess *session.Session) context.Context {
	return context.WithValue(ctx, contextKey{}, sess)
}

func SessionFromContext(ctx context.Context) *session.Session {
	v, _ := ctx.Value(contextKey{}).(*session.Session)
	return v
}

// Package auth wraps an http.Handler with session-cookie gating. Unauthed API
// paths get 401 (frontend needs a status code); unauthed SPA paths redirect to /login.
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/oauth/cookie"
)

// Resumer is the slice of *oauth.ClientApp the middleware depends on.
type Resumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error)
}

// SessionLocker serialises the refresh cycle for a single (did, sid); indigo doesn't coalesce refreshes, so concurrent expired-session requests can boot a valid user.
type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}

type Middleware func(http.Handler) http.Handler

// maxAPIBodyBytes caps JSON bodies at 1 MB, clearing the largest legitimate payload (batch subscription create) with headroom.
const maxAPIBodyBytes = 1 << 20

// New builds the gating middleware; every path is authed by default except the named public/infra routes.
func New(resumer Resumer, locker SessionLocker, sealer *cookie.Sealer) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Caps bodies before decode so a large payload can't exhaust memory; api.decodeJSON turns the overflow into a 413.
			if strings.HasPrefix(path, "/api/") {
				r.Body = http.MaxBytesReader(w, r.Body, maxAPIBodyBytes)
			}

			// Resume failures (missing, garbage, tampered, dead row) collapse to unauthed; dead-row also clears the cookie so retries don't loop forever.
			var sess *oauth.ClientSession
			didStr, sid, ok := sealer.Get(r)
			if ok {
				if did, err := syntax.ParseDID(didStr); err == nil {
					// Only requests that make authenticated PDS calls can trigger a
					// lazy refresh or DPoP-nonce rotation mid-handler, so only they
					// hold the lock, spanning next.ServeHTTP so refreshed tokens persist first.
					if holdsSessionLock(r) {
						unlock := locker.LockSession(did, sid)
						defer unlock()
					}
					s, err := resumer.ResumeSession(r.Context(), did, sid)
					switch {
					case err == nil:
						sess = s
					case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
						// Client went away mid-resume; keep the cookie so the next request can retry.
						slog.Debug("resume session transient", "err", err)
					default:
						sealer.Clear(w)
					}
				} else {
					sealer.Clear(w)
				}
			}

			if redirect, isPublic := publicRoutes[path]; isPublic {
				if sess != nil && redirect != "" {
					http.Redirect(w, r, redirect, http.StatusFound)
					return
				}
				serve(next, w, r, sess)
				return
			}

			if isInfra(path) {
				serve(next, w, r, sess)
				return
			}

			if sess != nil {
				serve(next, w, r, sess)
				return
			}

			if strings.HasPrefix(path, "/api/") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}
}

// holdsSessionLock must mirror the PDS-mutating routes in server/routes.go; subscriptions/resolve, digest/refresh, and entries/extract don't write to the PDS.
func holdsSessionLock(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	p := r.URL.Path
	if p == "/api/subscriptions/resolve" {
		return false
	}
	return strings.HasPrefix(p, "/api/subscriptions") ||
		strings.HasPrefix(p, "/api/saves") ||
		strings.HasPrefix(p, "/api/shares") ||
		strings.HasPrefix(p, "/api/follows")
}

func serve(next http.Handler, w http.ResponseWriter, r *http.Request, sess *oauth.ClientSession) {
	if sess != nil {
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
		return
	}
	next.ServeHTTP(w, r)
}

var publicRoutes = map[string]string{
	"/":      "/digest",
	"/login": "/digest",
	"/about": "",
}

var infraExact = map[string]struct{}{
	"/api/health":     {},
	"/oauth/login":    {},
	"/oauth/callback": {},
	"/oauth/logout":   {},
}

var infraPrefixes = []string{
	"/assets/",
	"/@",             // /@vite/, /@react-refresh, /@id, /@fs (Vite dev)
	"/node_modules/", // Vite dev
	"/src/",          // Vite dev
}

func isInfra(path string) bool {
	if _, ok := infraExact[path]; ok {
		return true
	}
	for _, p := range infraPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	// Dotted-last-segment means a root-level static file (/favicon.svg), but API
	// paths carry dotted handles and did:web identifiers, so they never qualify.
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

// WithSession returns a context carrying sess; exported so tests can inject a session without going through the middleware.
func WithSession(ctx context.Context, sess *oauth.ClientSession) context.Context {
	return context.WithValue(ctx, contextKey{}, sess)
}

// SessionFromContext returns the injected session, nil if absent (shouldn't happen on a gated path behind the middleware).
func SessionFromContext(ctx context.Context) *oauth.ClientSession {
	v, _ := ctx.Value(contextKey{}).(*oauth.ClientSession)
	return v
}

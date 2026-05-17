// Package auth wraps an http.Handler with session-cookie gating.
//
// Routing decisions per request:
//   - Infrastructure paths (OAuth dance, /api/health, static assets) pass
//     through regardless of auth state.
//   - Public product routes: anon passes; authed → 302 to authedRedirect
//     when set, else passes.
//   - Authed product routes: anon → 302 /login; authed passes.
//   - Unknown SPA paths: anon → 302 /login (matches authed-route gating).
//   - Unknown /api/* paths gated and unauthed: 401 (FE needs a status code).
//
// Authed requests get the *oauth.ClientSession injected into the request
// context for downstream handlers.
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

// SessionLocker serialises GetSession → refresh → SaveSession for a single
// (did, sid). Required because indigo doesn't coalesce refreshes internally —
// two concurrent expired-session requests can both refresh, and the loser's
// invalid_grant boots a still-valid user.
type SessionLocker interface {
	LockSession(did syntax.DID, sid string) func()
}

// Middleware returns an http.Handler middleware.
type Middleware func(http.Handler) http.Handler

// New builds the gating middleware. Only public SPA paths are named here;
// every other product path is authed by default.
func New(resumer Resumer, locker SessionLocker, sealer *cookie.Sealer) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Try to resume any session present on the cookie. Failures
			// (missing, garbage, tampered, dead row) all collapse to
			// "unauthed" — and we clear the cookie on dead-row to stop
			// the user bouncing through resume failures forever.
			var sess *oauth.ClientSession
			didStr, sid, ok := sealer.Get(r)
			if ok {
				if did, err := syntax.ParseDID(didStr); err == nil {
					// Serialise concurrent refreshes for the same (did, sid) so
					// the loser of a race doesn't see invalid_grant.
					unlock := locker.LockSession(did, sid)
					s, err := resumer.ResumeSession(r.Context(), did, sid)
					unlock()
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

			// Public product route: authed → maybe redirect, else pass.
			if redirect, isPublic := publicRoutes[path]; isPublic {
				if sess != nil && redirect != "" {
					http.Redirect(w, r, redirect, http.StatusFound)
					return
				}
				serve(next, w, r, sess)
				return
			}

			// Infrastructure allowlist (OAuth dance, health, assets).
			if isInfra(path) {
				serve(next, w, r, sess)
				return
			}

			// Everything else is gated by default — covers authed product
			// routes and unknown SPA paths alike.
			if sess != nil {
				serve(next, w, r, sess)
				return
			}

			// Unauthed, gated. API paths get a status code; SPA paths get a redirect.
			if strings.HasPrefix(path, "/api/") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}
}

func serve(next http.Handler, w http.ResponseWriter, r *http.Request, sess *oauth.ClientSession) {
	if sess != nil {
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
		return
	}
	next.ServeHTTP(w, r)
}

var publicRoutes = map[string]string{
	"/":      "/consume",
	"/login": "/consume",
	"/about": "",
}

// Infrastructure paths that bypass auth entirely. These are not product routes.
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
	// Static assets are anything with a file extension in the last segment —
	// /favicon.svg, /icons.svg, /oauth-client-metadata.json, /robots.txt, etc.
	last := strings.LastIndex(path, "/")
	if last >= 0 && strings.Contains(path[last:], ".") {
		return true
	}
	return false
}

type contextKey struct{}

// WithSession returns a child context carrying sess. Exported so callers
// (including tests) can inject a session in context without going through
// the middleware.
func WithSession(ctx context.Context, sess *oauth.ClientSession) context.Context {
	return context.WithValue(ctx, contextKey{}, sess)
}

// SessionFromContext returns the *oauth.ClientSession injected by the
// middleware. Returns nil if none — but in handlers behind the middleware
// on a gated path this should never happen.
func SessionFromContext(ctx context.Context) *oauth.ClientSession {
	v, _ := ctx.Value(contextKey{}).(*oauth.ClientSession)
	return v
}

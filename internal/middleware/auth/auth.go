// Package auth wraps an http.Handler with session-cookie gating.
//
// Allowlisted paths (OAuth dance, public discovery, static assets, /api/health)
// pass through. Gated SPA paths 302 to /login; gated /api/* paths return 401
// (the FE needs a status code, not a redirect). Authed requests get the
// *oauth.ClientSession injected into the request context.
package auth

import (
	"context"
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

// Middleware returns an http.Handler middleware.
type Middleware func(http.Handler) http.Handler

// New builds the gating middleware.
func New(resumer Resumer, sealer *cookie.Sealer) Middleware {
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
					if s, err := resumer.ResumeSession(r.Context(), did, sid); err == nil {
						sess = s
					} else {
						sealer.Clear(w)
					}
				} else {
					sealer.Clear(w)
				}
			}

			// Authed user hitting /login → 302 / (no dead-end rule).
			if path == "/login" && sess != nil {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}

			if isAllowlisted(path) {
				if sess != nil {
					next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if sess != nil {
				next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
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

// Exact-match allowlist of paths that bypass authentication entirely.
var allowedExact = map[string]struct{}{
	"/api/health":     {},
	"/oauth/login":    {},
	"/oauth/callback": {},
	"/oauth/logout":   {},
	"/login":          {},
}

// Allowlisted prefixes — covers Vite-internal asset paths in dev and the
// embedded /assets/ directory in prod.
var allowedPrefixes = []string{
	"/assets/",
	"/@",            // /@vite/, /@react-refresh, /@id, /@fs (Vite dev)
	"/node_modules/", // Vite dev
	"/src/",          // Vite dev
}

func isAllowlisted(path string) bool {
	if _, ok := allowedExact[path]; ok {
		return true
	}
	for _, p := range allowedPrefixes {
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

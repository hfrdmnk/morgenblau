package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// cloudflareOriginHeader carries CLOUDFLARE_ORIGIN_SECRET, which a Cloudflare request header transform rule adds; see DEPLOY.md.
const cloudflareOriginHeader = "X-Morgenblau-Origin-Secret"

// requireCloudflareOrigin refuses requests that reached Fly without passing Cloudflare, such as ones sent to the Fly IP or the fly.dev host.
func requireCloudflareOrigin(next http.Handler, secret string) http.Handler {
	want := []byte(secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fly's health checks and the CA's HTTP-01 validation never pass Cloudflare.
		if r.Method == http.MethodGet && r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") ||
			subtle.ConstantTimeCompare([]byte(r.Header.Get(cloudflareOriginHeader)), want) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
}

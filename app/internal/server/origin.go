package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const cloudflareOriginHeader = "X-Morgenblau-Origin-Secret"

func requireCloudflareOrigin(next http.Handler, secret string) http.Handler {
	want := []byte(secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isFlyHealthCheckOrACMEChallenge(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get(cloudflareOriginHeader)), want) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
}

func isFlyHealthCheckOrACMEChallenge(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/")
}

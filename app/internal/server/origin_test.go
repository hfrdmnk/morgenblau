package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireCloudflareOrigin(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name, method, path, header string
		status                     int
	}{
		{"cloudflare request", "GET", "/api/digest", secret, http.StatusTeapot},
		{"cloudflare webhook", "POST", "/inbound/postshiba", secret, http.StatusTeapot},
		{"direct request", "GET", "/", "", http.StatusForbidden},
		{"wrong secret", "GET", "/api/digest", "0123456789abcdef0123456789abcdeX", http.StatusForbidden},
		{"secret with a suffix", "GET", "/api/digest", secret + "0", http.StatusForbidden},
		{"secret prefix", "GET", "/api/digest", secret[:16], http.StatusForbidden},
		{"Fly health check", "GET", "/api/health", "", http.StatusTeapot},
		{"health path on another verb", "POST", "/api/health", "", http.StatusForbidden},
		{"health path prefix", "GET", "/api/healthz", "", http.StatusForbidden},
		{"ACME HTTP-01 challenge", "GET", "/.well-known/acme-challenge/token-value", "", http.StatusTeapot},
		{"other well-known path", "GET", "/.well-known/oauth-protected-resource", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
			req := httptest.NewRequest(tc.method, "https://app.example.com"+tc.path, nil)
			if tc.header != "" {
				req.Header.Set(cloudflareOriginHeader, tc.header)
			}
			rr := httptest.NewRecorder()
			requireCloudflareOrigin(next, secret).ServeHTTP(rr, req)
			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d", rr.Code, tc.status)
			}
		})
	}
}

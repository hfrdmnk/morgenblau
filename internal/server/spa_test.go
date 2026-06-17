package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/frontend"
)

func TestEmbeddedDistCacheHeaders(t *testing.T) {
	h := embeddedDistHandler()

	dist, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		t.Fatalf("sub dist: %v", err)
	}
	entries, err := fs.ReadDir(dist, "assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("no embedded assets (run `bun run build`): %v", err)
	}
	assetPath := "/assets/" + entries[0].Name()

	tests := []struct {
		name string
		path string
		want string
	}{
		{"hashed asset is immutable", assetPath, "public, max-age=31536000, immutable"},
		{"html shell revalidates", "/", "no-cache"},
		{"unknown route falls back to shell", "/digest", "no-cache"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status for %s = %d, want 200", tt.path, rec.Code)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control for %s = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

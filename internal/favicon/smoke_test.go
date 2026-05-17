//go:build smoke

package favicon

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestDiscoverDominikHoferMe is a live network smoke test for the specific
// site that motivated this feature. Skipped unless `go test -tags=smoke`.
func TestDiscoverDominikHoferMe(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	got, err := Discover(context.Background(), client, "https://dominikhofer.me")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	t.Logf("dominikhofer.me favicon: %s", got)
	if got == "" {
		t.Fatal("empty URL")
	}
}

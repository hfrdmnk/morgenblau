package atxrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

func TestNew_SetsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := New(srv.URL, nil)
	var out map[string]any
	if err := client.Get(context.Background(), syntax.NSID("com.atproto.repo.listRecords"), nil, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}
}

func TestNew_NilHTTPClientKeepsDefault(t *testing.T) {
	client := New("https://example.test", nil)
	if client.Client != http.DefaultClient {
		t.Errorf("Client = %v, want http.DefaultClient", client.Client)
	}
}

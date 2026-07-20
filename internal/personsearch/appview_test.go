package personsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/safehttp"
)

func TestSearchActorsTypeahead_DecodesAndSendsParams(t *testing.T) {
	var (
		gotPath  string
		gotQuery string
		gotLimit string
		gotUA    string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("q")
		gotLimit = r.URL.Query().Get("limit")
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"actors":[
			{"did":"did:plc:aaaaaaaaaaaaaaaaaaaaaaaa","handle":"alice.example","displayName":"Alice","avatar":"https://cdn.example.com/av/alice.jpg"},
			{"did":"did:plc:bbbbbbbbbbbbbbbbbbbbbbbb","handle":"bob.example"}
		]}`))
	}))
	defer srv.Close()

	av := NewAppView(srv.URL, srv.Client())
	actors, err := av.SearchActorsTypeahead(context.Background(), "ali", 10)
	if err != nil {
		t.Fatalf("SearchActorsTypeahead: %v", err)
	}

	if gotPath != "/xrpc/app.bsky.actor.searchActorsTypeahead" {
		t.Errorf("path = %q, want the typeahead XRPC path", gotPath)
	}
	if gotQuery != "ali" {
		t.Errorf("q = %q, want %q", gotQuery, "ali")
	}
	if gotLimit != "10" {
		t.Errorf("limit = %q, want %q", gotLimit, "10")
	}
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}

	if len(actors) != 2 {
		t.Fatalf("got %d actors, want 2", len(actors))
	}
	if actors[0].DID != "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa" || actors[0].Handle != "alice.example" || actors[0].DisplayName != "Alice" {
		t.Errorf("actor[0] = %+v, want alice fields decoded", actors[0])
	}
	if actors[0].Avatar != "https://cdn.example.com/av/alice.jpg" {
		t.Errorf("avatar = %q, want the CDN URL passed through verbatim", actors[0].Avatar)
	}
	if actors[1].DisplayName != "" || actors[1].Avatar != "" {
		t.Errorf("actor[1] = %+v, want empty displayName/avatar for the missing fields", actors[1])
	}
}

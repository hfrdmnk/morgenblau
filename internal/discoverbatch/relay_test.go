package discoverbatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/safehttp"
)

func TestNormalizeRelayHost_BareHostGetsHTTPS(t *testing.T) {
	cases := []struct{ in, want string }{
		{"relay1.us-east.bsky.network", "https://relay1.us-east.bsky.network"},
		{"https://relay1.us-east.bsky.network", "https://relay1.us-east.bsky.network"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
	}
	for _, c := range cases {
		if got := normalizeRelayHost(c.in); got != c.want {
			t.Errorf("normalizeRelayHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNew_NormalizesBareRelayHost(t *testing.T) {
	b := New("relay.example", nil, nil, nil, nil)
	if b.relayEndpoint != "https://relay.example" {
		t.Errorf("relayEndpoint = %q, want https://relay.example", b.relayEndpoint)
	}
}

func TestEnumerateCollection_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("collection"); got != "site.standard.publication" {
			t.Errorf("collection = %q, want site.standard.publication", got)
		}
		if got := r.URL.Query().Get("limit"); got != "1000" {
			t.Errorf("limit = %q, want 1000", got)
		}
		cursor := r.URL.Query().Get("cursor")
		page++
		w.Header().Set("Content-Type", "application/json")
		switch cursor {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"repos":  []map[string]any{{"did": "did:plc:one"}, {"did": "did:plc:two"}},
				"cursor": "page2",
			})
		case "page2":
			json.NewEncoder(w).Encode(map[string]any{
				"repos": []map[string]any{{"did": "did:plc:three"}},
			})
		default:
			t.Fatalf("unexpected cursor %q", cursor)
		}
	}))
	defer srv.Close()

	got, err := EnumerateCollection(context.Background(), srv.Client(), srv.URL, "site.standard.publication")
	if err != nil {
		t.Fatalf("EnumerateCollection: %v", err)
	}
	if page != 2 {
		t.Errorf("pages fetched = %d, want 2", page)
	}
	want := []string{"did:plc:one", "did:plc:two", "did:plc:three"}
	if len(got) != len(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestEnumerateCollection_SendsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{}})
	}))
	defer srv.Close()

	if _, err := EnumerateCollection(context.Background(), srv.Client(), srv.URL, "site.standard.publication"); err != nil {
		t.Fatalf("EnumerateCollection: %v", err)
	}
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}
}

func TestEnumerateCollection_EmptyReposYieldsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{}})
	}))
	defer srv.Close()

	got, err := EnumerateCollection(context.Background(), srv.Client(), srv.URL, "blue.morgen.feed.subscription")
	if err != nil {
		t.Fatalf("EnumerateCollection: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want empty", got)
	}
}

func TestEnumerateAll_DedupesDIDsAcrossCollections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case "collection-a":
			json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{{"did": "did:plc:shared"}, {"did": "did:plc:only-a"}}})
		case "collection-b":
			json.NewEncoder(w).Encode(map[string]any{"repos": []map[string]any{{"did": "did:plc:shared"}, {"did": "did:plc:only-b"}}})
		default:
			t.Fatalf("unexpected collection %q", coll)
		}
	}))
	defer srv.Close()

	got, err := EnumerateAll(context.Background(), srv.Client(), srv.URL, []string{"collection-a", "collection-b"})
	if err != nil {
		t.Fatalf("EnumerateAll: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got = %v, want 3 deduped DIDs", got)
	}
	seen := map[string]bool{}
	for _, d := range got {
		seen[d] = true
	}
	for _, want := range []string{"did:plc:shared", "did:plc:only-a", "did:plc:only-b"} {
		if !seen[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

func TestEnumerateAll_CollectionFailurePropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := EnumerateAll(context.Background(), srv.Client(), srv.URL, []string{"collection-a"}); err == nil {
		t.Fatal("expected error to propagate from a failing collection enumeration")
	}
}

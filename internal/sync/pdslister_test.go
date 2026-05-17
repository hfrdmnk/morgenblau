package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atclient"
)

// TestListSubscriptions_PagesUntilCursorEmpty asserts the lister keeps paging
// past an empty page (cursor still set) and only stops when the cursor is
// empty. An early stop would let reconcile delete every local row missed by
// the partial snapshot.
func TestListSubscriptions_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	pages := []struct {
		records []map[string]any
		cursor  string
	}{
		{records: []map[string]any{
			{"uri": "at://x/c/r1", "cid": "b", "value": map[string]any{"feedUrl": "https://a/feed"}},
		}, cursor: "c1"},
		// Empty page but cursor still set — must NOT terminate paging.
		{records: nil, cursor: "c2"},
		{records: []map[string]any{
			{"uri": "at://x/c/r3", "cid": "b", "value": map[string]any{"feedUrl": "https://b/feed"}},
		}, cursor: ""},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if page >= len(pages) {
			t.Errorf("unexpected extra page request: %d", page)
			http.Error(w, "extra", http.StatusInternalServerError)
			return
		}
		p := pages[page]
		page++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records": p.records,
			"cursor":  p.cursor,
		})
	}))
	defer srv.Close()

	client := atclient.NewAPIClient(srv.URL)
	out, err := pageSubscriptions(context.Background(), client, "did:plc:alice")
	if err != nil {
		t.Fatalf("pageSubscriptions: %v", err)
	}
	if page != len(pages) {
		t.Errorf("pages fetched = %d, want %d", page, len(pages))
	}
	if len(out) != 2 {
		t.Fatalf("records = %d, want 2", len(out))
	}
	if out[0].FeedURL != "https://a/feed" || out[1].FeedURL != "https://b/feed" {
		t.Errorf("records = %+v", out)
	}
}

func TestListSubscriptions_StopsOnEmptyCursor(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records": []map[string]any{
				{"uri": "at://x/c/r", "cid": "b", "value": map[string]any{"feedUrl": "https://a/feed"}},
			},
			"cursor": "",
		})
	}))
	defer srv.Close()

	client := atclient.NewAPIClient(srv.URL)
	out, err := pageSubscriptions(context.Background(), client, "did:plc:alice")
	if err != nil {
		t.Fatalf("pageSubscriptions: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if len(out) != 1 {
		t.Errorf("records = %d, want 1", len(out))
	}
}

func TestToPDSSubscription_Mapping(t *testing.T) {
	r := recordEntry{
		URI: "at://did:plc:alice/app.skyreader.feed.subscription/3la",
		CID: "bafy",
		Value: map[string]any{
			"feedUrl":     "https://example.com/feed",
			"title":       "Example",
			"customTitle": "My Example",
		},
	}
	got := toPDSSubscription(r)
	if got.Rkey != "3la" {
		t.Errorf("Rkey = %q", got.Rkey)
	}
	if got.FeedURL != "https://example.com/feed" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
	if got.Title != "Example" || got.CustomTitle != "My Example" {
		t.Errorf("titles = %q / %q", got.Title, got.CustomTitle)
	}
}

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
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		CID: "bafy",
		Value: map[string]any{
			"feedUrl": "https://example.com/feed",
			"title":   "Example",
			"primary": true,
			// JSON arrays decode to []any, so tags land as []any{string...}.
			"tags": []any{"tech", "design"},
		},
	}
	got := toPDSSubscription(r)
	if got.Rkey != "3la" {
		t.Errorf("Rkey = %q", got.Rkey)
	}
	if got.FeedURL != "https://example.com/feed" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
	if got.Title != "Example" {
		t.Errorf("title = %q", got.Title)
	}
	if !got.Primary {
		t.Errorf("primary = false, want true")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "tech" || got.Tags[1] != "design" {
		t.Errorf("tags = %v, want [tech design]", got.Tags)
	}
}

func TestToPDSSubscription_MissingOptionalFields(t *testing.T) {
	got := toPDSSubscription(recordEntry{
		URI: "at://did:plc:example/blue.morgen.feed.subscription/3la",
		CID: "bafy",
		Value: map[string]any{
			"feedUrl": "https://example.com/feed",
		},
	})
	if got.Rkey != "3la" {
		t.Errorf("Rkey = %q", got.Rkey)
	}
	if got.FeedURL != "https://example.com/feed" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
	if got.Title != "" {
		t.Errorf("optional title = %q, want empty", got.Title)
	}
	// Absent primary/tags mean the record doesn't set them — the PDS is the
	// source of truth, so zero values are correct, not a data loss.
	if got.Primary {
		t.Errorf("primary = true, want false when absent")
	}
	if got.Tags != nil {
		t.Errorf("tags = %v, want nil when absent", got.Tags)
	}
}

func TestToPDSSave_Mapping(t *testing.T) {
	got := toPDSSave(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.save/3sk",
		CID: "bafy",
		Value: map[string]any{
			"itemUrl":   "https://example.com/post",
			"feedUrl":   "https://example.com/feed",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if got.Rkey != "3sk" {
		t.Errorf("Rkey = %q", got.Rkey)
	}
	if got.ItemURL != "https://example.com/post" {
		t.Errorf("ItemURL = %q", got.ItemURL)
	}
	if got.FeedURL != "https://example.com/feed" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
	if got.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q", got.CreatedAt)
	}
}

func TestToPDSSave_MissingOptionalFields(t *testing.T) {
	got := toPDSSave(recordEntry{
		URI:   "at://did:plc:alice/blue.morgen.feed.save/3sk",
		Value: map[string]any{"itemUrl": "https://example.com/post"},
	})
	if got.ItemURL != "https://example.com/post" {
		t.Errorf("ItemURL = %q", got.ItemURL)
	}
	// feedUrl and createdAt are optional on the save record.
	if got.FeedURL != "" {
		t.Errorf("FeedURL = %q, want empty", got.FeedURL)
	}
	if got.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", got.CreatedAt)
	}
}

func TestToPDSSubscription_TagsSkipsNonStrings(t *testing.T) {
	got := toPDSSubscription(recordEntry{
		URI: "at://did:plc:example/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"feedUrl": "https://example.com/feed",
			"tags":    []any{"keep", 42, nil, "also"},
		},
	})
	if len(got.Tags) != 2 || got.Tags[0] != "keep" || got.Tags[1] != "also" {
		t.Errorf("tags = %v, want [keep also]", got.Tags)
	}
}

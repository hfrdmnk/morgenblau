package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atclient"
)

// rssRecordValue builds a minimal rev-2 union-shaped record value.
func rssRecordValue(feedURL string) map[string]any {
	return map[string]any{
		"source": map[string]any{
			"$type":   "blue.morgen.feed.subscription#rssFeed",
			"feedUrl": feedURL,
		},
		"createdAt": "2026-06-01T00:00:00Z",
	}
}

// TestListSubscriptions_PagesUntilCursorEmpty proves an early stop on cursor would let reconcile delete rows missed by a partial snapshot.
func TestListSubscriptions_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	pages := []struct {
		records []map[string]any
		cursor  string
	}{
		{records: []map[string]any{
			{"uri": "at://x/c/r1", "cid": "b", "value": rssRecordValue("https://a/feed")},
		}, cursor: "c1"},
		// Empty page but cursor still set: must NOT terminate paging.
		{records: nil, cursor: "c2"},
		{records: []map[string]any{
			{"uri": "at://x/c/r3", "cid": "b", "value": rssRecordValue("https://b/feed")},
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
				{"uri": "at://x/c/r", "cid": "b", "value": rssRecordValue("https://a/feed")},
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

func TestToPDSSubscription_RSSVariant(t *testing.T) {
	r := recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		CID: "bafy",
		Value: map[string]any{
			"source": map[string]any{
				"$type":   "blue.morgen.feed.subscription#rssFeed",
				"feedUrl": "https://example.com/feed",
				"siteUrl": "https://example.com",
			},
			"title":     "Example",
			"primary":   true,
			"createdAt": "2026-06-01T00:00:00Z",
			// JSON arrays decode to []any, so tags land as []any{string...}.
			"tags": []any{"tech", "design"},
		},
	}
	got, ok := toPDSSubscription(r)
	if !ok {
		t.Fatal("expected ok for rssFeed variant")
	}
	if got.Kind != "rss" {
		t.Errorf("Kind = %q", got.Kind)
	}
	if got.Rkey != "3la" {
		t.Errorf("Rkey = %q", got.Rkey)
	}
	if got.FeedURL != "https://example.com/feed" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
	if got.SiteURL != "https://example.com" {
		t.Errorf("SiteURL = %q", got.SiteURL)
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

func TestToPDSSubscription_StandardPublicationVariant(t *testing.T) {
	got, ok := toPDSSubscription(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{
				"$type":       "blue.morgen.feed.subscription#standardPublication",
				"publication": "at://did:plc:pub/site.standard.publication/3p",
			},
			"title":     "My Title",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected ok for standardPublication variant")
	}
	if got.Kind != "standardfeed" {
		t.Errorf("Kind = %q", got.Kind)
	}
	if got.Publication != "at://did:plc:pub/site.standard.publication/3p" {
		t.Errorf("Publication = %q", got.Publication)
	}
	if got.FeedURL != "" {
		t.Errorf("FeedURL = %q, want empty", got.FeedURL)
	}
	if got.Title != "My Title" {
		t.Errorf("title = %q", got.Title)
	}
}

func TestToPDSSubscription_SkipsUnrecognizableRecords(t *testing.T) {
	cases := []struct {
		name  string
		value map[string]any
	}{
		{"v1 flat shape (no source)", map[string]any{"feedUrl": "https://example.com/feed"}},
		{"unknown variant", map[string]any{"source": map[string]any{"$type": "blue.morgen.feed.subscription#carrierPigeon", "coop": "x"}}},
		{"source not a map", map[string]any{"source": "https://example.com/feed"}},
		{"rss variant missing feedUrl", map[string]any{"source": map[string]any{"$type": "blue.morgen.feed.subscription#rssFeed"}}},
		{"standard variant missing publication", map[string]any{"source": map[string]any{"$type": "blue.morgen.feed.subscription#standardPublication"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := toPDSSubscription(recordEntry{
				URI:   "at://did:plc:alice/blue.morgen.feed.subscription/3la",
				Value: tc.value,
			})
			if ok {
				t.Fatal("expected record to be skipped")
			}
		})
	}
}

func TestToPDSSave_Mapping(t *testing.T) {
	got, ok := toPDSSave(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.save/3sk",
		CID: "bafy",
		Value: map[string]any{
			"itemUrl":   "https://example.com/post",
			"feedUrl":   "https://example.com/feed",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected ok for a valid save record")
	}
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

func TestToPDSSave_MissingOptionalFeedURL(t *testing.T) {
	got, ok := toPDSSave(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.save/3sk",
		Value: map[string]any{
			"itemUrl":   "https://example.com/post",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected ok when only the optional feedUrl is absent")
	}
	if got.ItemURL != "https://example.com/post" {
		t.Errorf("ItemURL = %q", got.ItemURL)
	}
	// feedUrl is optional on the save record.
	if got.FeedURL != "" {
		t.Errorf("FeedURL = %q, want empty", got.FeedURL)
	}
}

// TestToPDSSave_SkipsRecordMissingRequiredField proves lexicon validation rejects a save record missing the required createdAt field.
func TestToPDSSave_SkipsRecordMissingRequiredField(t *testing.T) {
	_, ok := toPDSSave(recordEntry{
		URI:   "at://did:plc:alice/blue.morgen.feed.save/3sk",
		Value: map[string]any{"itemUrl": "https://example.com/post"},
	})
	if ok {
		t.Fatal("expected record missing required createdAt to be skipped")
	}
}

func TestToPDSShare_RSSMapping(t *testing.T) {
	got, ok := toPDSShare(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.share/3sh",
		Value: map[string]any{
			"itemUrl":   "https://example.com/post",
			"feedUrl":   "https://example.com/feed",
			"comment":   "great read",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got.Rkey != "3sh" || got.ItemURL != "https://example.com/post" {
		t.Errorf("got = %+v", got)
	}
	if got.Document != "" {
		t.Errorf("Document = %q, want empty (rss share)", got.Document)
	}
	if got.FeedURL != "https://example.com/feed" || got.Comment != "great read" {
		t.Errorf("got = %+v", got)
	}
}

func TestToPDSShare_SidecarCarriesDocument(t *testing.T) {
	got, ok := toPDSShare(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.share/3sc",
		Value: map[string]any{
			"itemUrl":  "https://blog.example/post",
			"document": "at://did:plc:pub/site.standard.document/3da",
			"comment":  "loved it",
		},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got.Document != "at://did:plc:pub/site.standard.document/3da" {
		t.Errorf("Document = %q", got.Document)
	}
}

func TestToPDSShare_MissingItemURLSkipped(t *testing.T) {
	if _, ok := toPDSShare(recordEntry{
		URI:   "at://did:plc:alice/blue.morgen.feed.share/3sc",
		Value: map[string]any{"comment": "no url"},
	}); ok {
		t.Error("expected skip: itemUrl is required by the lexicon")
	}
}

func TestToPDSRecommend_Mapping(t *testing.T) {
	got, ok := toPDSRecommend(recordEntry{
		URI: "at://did:plc:alice/site.standard.graph.recommend/3rec",
		Value: map[string]any{
			"document":  "at://did:plc:pub/site.standard.document/3da",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got.Rkey != "3rec" || got.Document != "at://did:plc:pub/site.standard.document/3da" {
		t.Errorf("got = %+v", got)
	}
	if got.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q", got.CreatedAt)
	}
}

func TestToPDSFollow_Mapping(t *testing.T) {
	got, ok := toPDSFollow(recordEntry{
		URI: "at://did:plc:alice/blue.morgen.graph.follow/3fa",
		Value: map[string]any{
			"subject":   "did:plc:bob",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if got.Rkey != "3fa" || got.SubjectDID != "did:plc:bob" {
		t.Errorf("got = %+v", got)
	}
	if got.CreatedAt != "2026-06-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q", got.CreatedAt)
	}
}

func TestToPDSFollow_MissingSubjectSkipped(t *testing.T) {
	if _, ok := toPDSFollow(recordEntry{
		URI:   "at://did:plc:alice/blue.morgen.graph.follow/3fa",
		Value: map[string]any{"createdAt": "2026-06-01T00:00:00Z"},
	}); ok {
		t.Error("expected skip: subject is required by the lexicon")
	}
}

func TestToPDSRecommend_MissingDocumentSkipped(t *testing.T) {
	if _, ok := toPDSRecommend(recordEntry{
		URI:   "at://did:plc:alice/site.standard.graph.recommend/3rec",
		Value: map[string]any{"createdAt": "2026-06-01T00:00:00Z"},
	}); ok {
		t.Error("expected skip: document is required")
	}
}

// TestToPDSSubscription_InvalidTagType_Skipped proves a non-string tag element fails lexicon validation and skips the whole record, not just that element.
func TestToPDSSubscription_InvalidTagType_Skipped(t *testing.T) {
	_, ok := toPDSSubscription(recordEntry{
		URI: "at://did:plc:example/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{
				"$type":   "blue.morgen.feed.subscription#rssFeed",
				"feedUrl": "https://example.com/feed",
			},
			"createdAt": "2026-06-01T00:00:00Z",
			"tags":      []any{"keep", 42, nil, "also"},
		},
	})
	if ok {
		t.Fatal("expected record with a non-string tag element to be skipped")
	}
}

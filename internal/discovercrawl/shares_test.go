package discovercrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// sharesHandler serves canned records per collection, keyed like the real listRecords endpoint.
func sharesHandler(t *testing.T, byCollection map[string][]map[string]any) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": byCollection[coll]})
	})
}

func TestClient_CrawlShares_AggregatesRSSStandardfeedAndSkyreader(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenShareCollection: {
			{"uri": "at://" + followedDID + "/" + morgenShareCollection + "/1", "value": map[string]any{
				"itemUrl": "https://a.example/post", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
		standardRecommendCollection: {
			{"uri": "at://" + followedDID + "/" + standardRecommendCollection + "/1", "value": map[string]any{
				"document": "at://did:plc:pub/site.standard.document/1", "createdAt": "2026-07-02T00:00:00Z",
			}},
		},
		skyreaderShareCollection: {
			{"uri": "at://" + followedDID + "/" + skyreaderShareCollection + "/1", "value": map[string]any{
				"itemUrl": "https://c.example/post", "createdAt": "2026-07-03T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, sharesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	kinds := map[string]int{}
	for _, s := range got {
		kinds[s.Kind]++
	}
	if kinds["rss"] != 1 || kinds["standardfeed"] != 1 || kinds["skyreader"] != 1 {
		t.Errorf("kinds = %+v, want one of each", kinds)
	}
}

// TestClient_CrawlShares_FeedURLProvenanceCanonicalized proves normalization reaches share provenance, since feedkey.ResolveReactionKey keys on this field directly.
func TestClient_CrawlShares_FeedURLProvenanceCanonicalized(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenShareCollection: {
			{"uri": "at://" + followedDID + "/" + morgenShareCollection + "/1", "value": map[string]any{
				"itemUrl": "https://a.example/post", "feedUrl": "https://a.example:443/feed/", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, sharesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://a.example/feed" {
		t.Fatalf("got = %+v, want feedUrl canonicalized to https://a.example/feed", got)
	}
}

func TestClient_CrawlShares_RecommendPlusSidecarMergeToOneEntryWithComment(t *testing.T) {
	document := "at://did:plc:pub/site.standard.document/1"
	byCollection := map[string][]map[string]any{
		standardRecommendCollection: {
			{"uri": "at://" + followedDID + "/" + standardRecommendCollection + "/1", "value": map[string]any{
				"document": document, "createdAt": "2026-07-02T00:00:00Z",
			}},
		},
		morgenShareCollection: {
			{"uri": "at://" + followedDID + "/" + morgenShareCollection + "/1", "value": map[string]any{
				"itemUrl": "https://pub.example/article", "document": document,
				"comment": "worth reading", "createdAt": "2026-07-02T00:05:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, sharesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (recommend + sidecar merge): %+v", len(got), got)
	}
	s := got[0]
	if s.Kind != "standardfeed" || s.Document != document {
		t.Errorf("got = %+v", s)
	}
	if s.Comment != "worth reading" {
		t.Errorf("Comment = %q, want sidecar comment", s.Comment)
	}
	if s.ItemURL != "https://pub.example/article" {
		t.Errorf("ItemURL = %q, want sidecar itemUrl", s.ItemURL)
	}
	// Recommend's own createdAt is the display timestamp, not the sidecar's.
	if s.CreatedAt != "2026-07-02T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want recommend's createdAt", s.CreatedAt)
	}
}

func TestClient_CrawlShares_RecommendWithoutSidecarHasNoComment(t *testing.T) {
	document := "at://did:plc:pub/site.standard.document/1"
	byCollection := map[string][]map[string]any{
		standardRecommendCollection: {
			{"uri": "at://" + followedDID + "/" + standardRecommendCollection + "/1", "value": map[string]any{
				"document": document, "createdAt": "2026-07-02T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, sharesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares: %v", err)
	}
	if len(got) != 1 || got[0].Comment != "" {
		t.Fatalf("got = %+v, want a bare recommend with no comment", got)
	}
}

func TestClient_CrawlShares_DuplicateRecommendsCanonicalSmallestRkeyWins(t *testing.T) {
	document := "at://did:plc:pub/site.standard.document/1"
	byCollection := map[string][]map[string]any{
		standardRecommendCollection: {
			{"uri": "at://" + followedDID + "/" + standardRecommendCollection + "/2", "value": map[string]any{
				"document": document, "createdAt": "2026-07-05T00:00:00Z",
			}},
			{"uri": "at://" + followedDID + "/" + standardRecommendCollection + "/1", "value": map[string]any{
				"document": document, "createdAt": "2026-07-02T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, sharesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (duplicates collapse): %+v", len(got), got)
	}
	if got[0].CreatedAt != "2026-07-02T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want the smaller-rkey (earliest) recommend to win", got[0].CreatedAt)
	}
}

func TestClient_CrawlShares_MalformedRecordsSkippedNotFatal(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenShareCollection: {
			{"uri": "at://" + followedDID + "/" + morgenShareCollection + "/1", "value": map[string]any{
				"createdAt": "2026-07-01T00:00:00Z", // missing itemUrl
			}},
			{"uri": "at://" + followedDID + "/" + morgenShareCollection + "/2", "value": map[string]any{
				"itemUrl": "https://good.example/post", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
		standardRecommendCollection: {
			{"uri": "at://" + followedDID + "/" + standardRecommendCollection + "/1", "value": map[string]any{
				"createdAt": "2026-07-01T00:00:00Z", // missing document
			}},
		},
	}
	client, _ := newTestClient(t, sharesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares should not fail on malformed records: %v", err)
	}
	if len(got) != 1 || got[0].ItemURL != "https://good.example/post" {
		t.Fatalf("got = %+v, want only the well-formed record", got)
	}
}

func TestClient_CrawlShares_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != morgenShareCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		cursor := r.URL.Query().Get("cursor")
		page++
		switch cursor {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{map[string]any{
					"uri":   "at://" + followedDID + "/x/1",
					"value": map[string]any{"itemUrl": "https://a.example/post", "createdAt": "2026-07-01T00:00:00Z"},
				}},
				"cursor": "page2",
			})
		case "page2":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{map[string]any{
					"uri":   "at://" + followedDID + "/x/2",
					"value": map[string]any{"itemUrl": "https://b.example/post", "createdAt": "2026-07-01T00:00:00Z"},
				}},
			})
		default:
			t.Fatalf("unexpected cursor %q", cursor)
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlShares(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlShares: %v", err)
	}
	if page != 2 {
		t.Errorf("pages fetched = %d, want 2", page)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestClient_CrawlShares_UnknownPDSEndpointErrors(t *testing.T) {
	c := NewClient(&fakeResolver{}, http.DefaultClient, nil, nil, nil)
	did, _ := syntax.ParseDID("did:plc:nobody")
	if _, err := c.CrawlShares(context.Background(), did); err == nil {
		t.Fatal("expected error for unresolvable did")
	}
}

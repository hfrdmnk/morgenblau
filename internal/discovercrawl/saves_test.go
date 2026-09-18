package discovercrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func savesHandler(t *testing.T, byCollection map[string][]map[string]any) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": byCollection[coll]})
	})
}

func TestClient_CrawlSaves_AggregatesMorgenSkyreaderAndGlean(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenSaveCollection: {
			{"uri": "at://" + followedDID + "/" + morgenSaveCollection + "/1", "value": map[string]any{
				"itemUrl": "https://a.example/post", "feedUrl": "https://a.example/feed", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
		skyreaderSaveCollection: {
			{"uri": "at://" + followedDID + "/" + skyreaderSaveCollection + "/1", "value": map[string]any{
				"url": "https://b.example/post", "savedAt": "2026-07-02T00:00:00Z",
			}},
		},
		gleanSaveCollection: {
			{"uri": "at://" + followedDID + "/" + gleanSaveCollection + "/1", "value": map[string]any{
				"articleUrl": "https://c.example/post", "feedUrl": "https://c.example/feed", "createdAt": "2026-07-03T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, savesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	kinds := map[string]int{}
	for _, s := range got {
		kinds[s.Kind]++
	}
	if kinds["morgen"] != 1 || kinds["skyreader"] != 1 || kinds["glean"] != 1 {
		t.Errorf("kinds = %+v, want one of each", kinds)
	}
}

func TestClient_CrawlSaves_MorgenRecordCarriesFeedURLProvenance(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenSaveCollection: {
			{"uri": "at://" + followedDID + "/" + morgenSaveCollection + "/1", "value": map[string]any{
				"itemUrl": "https://a.example/post", "feedUrl": "https://a.example/feed", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, savesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://a.example/feed" {
		t.Fatalf("got = %+v, want feedUrl provenance carried through", got)
	}
}

// TestClient_CrawlSaves_FeedURLProvenanceCanonicalized proves save provenance normalization matches feedkey.ResolveReactionKey, which keys reactions on this field.
func TestClient_CrawlSaves_FeedURLProvenanceCanonicalized(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenSaveCollection: {
			{"uri": "at://" + followedDID + "/" + morgenSaveCollection + "/1", "value": map[string]any{
				"itemUrl": "https://a.example/post", "feedUrl": "https://a.example:443/feed/", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, savesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://a.example/feed" {
		t.Fatalf("got = %+v, want feedUrl canonicalized to https://a.example/feed", got)
	}
}

func TestClient_CrawlSaves_SkyreaderRecordHasNoFeedURL(t *testing.T) {
	byCollection := map[string][]map[string]any{
		skyreaderSaveCollection: {
			{"uri": "at://" + followedDID + "/" + skyreaderSaveCollection + "/1", "value": map[string]any{
				"url": "https://b.example/post", "savedAt": "2026-07-02T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, savesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "" || got[0].ItemURL != "https://b.example/post" {
		t.Fatalf("got = %+v, want a bare itemUrl-only save", got)
	}
}

func TestClient_CrawlSaves_MalformedRecordsSkippedNotFatal(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenSaveCollection: {
			{"uri": "at://" + followedDID + "/" + morgenSaveCollection + "/1", "value": map[string]any{
				"createdAt": "2026-07-01T00:00:00Z", // missing itemUrl
			}},
			{"uri": "at://" + followedDID + "/" + morgenSaveCollection + "/2", "value": map[string]any{
				"itemUrl": "https://good.example/post", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
		gleanSaveCollection: {
			{"uri": "at://" + followedDID + "/" + gleanSaveCollection + "/1", "value": map[string]any{
				"feedUrl": "https://x.example/feed", // missing articleUrl
			}},
		},
	}
	client, _ := newTestClient(t, savesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves should not fail on malformed records: %v", err)
	}
	if len(got) != 1 || got[0].ItemURL != "https://good.example/post" {
		t.Fatalf("got = %+v, want only the well-formed record", got)
	}
}

func TestClient_CrawlSaves_DedupesByItemURLAcrossLexicons(t *testing.T) {
	byCollection := map[string][]map[string]any{
		morgenSaveCollection: {
			{"uri": "at://" + followedDID + "/" + morgenSaveCollection + "/1", "value": map[string]any{
				"itemUrl": "https://same.example/post", "createdAt": "2026-07-01T00:00:00Z",
			}},
		},
		skyreaderSaveCollection: {
			{"uri": "at://" + followedDID + "/" + skyreaderSaveCollection + "/1", "value": map[string]any{
				"url": "https://same.example/post", "savedAt": "2026-07-02T00:00:00Z",
			}},
		},
	}
	client, _ := newTestClient(t, savesHandler(t, byCollection))

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (deduped across lexicons): %+v", len(got), got)
	}
}

func TestClient_CrawlSaves_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != morgenSaveCollection {
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
	got, err := client.CrawlSaves(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlSaves: %v", err)
	}
	if page != 2 {
		t.Errorf("pages fetched = %d, want 2", page)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestClient_CrawlSaves_UnknownPDSEndpointErrors(t *testing.T) {
	c := NewClient(&fakeResolver{}, http.DefaultClient, nil, nil, nil)
	did, _ := syntax.ParseDID("did:plc:nobody")
	if _, err := c.CrawlSaves(context.Background(), did); err == nil {
		t.Fatal("expected error for unresolvable did")
	}
}

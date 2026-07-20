package discovercrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func subRecord(uri, feedURL string) map[string]any {
	return map[string]any{"uri": uri, "value": map[string]any{"feedUrl": feedURL, "createdAt": "2026-07-01T00:00:00Z"}}
}

func TestClient_CrawlOwnForeignSubscriptions_AggregatesSkyreaderAndGleanOnly(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case skyreaderSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{
				subRecord("at://"+followedDID+"/x/1", "https://sky.example/feed"),
			}})
		case gleanSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{
				subRecord("at://"+followedDID+"/y/1", "https://glean.example/feed"),
			}})
		default:
			t.Fatalf("unexpected collection %q — self-foreign crawl must never fetch blue.morgen or standardfeed", coll)
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlOwnForeignSubscriptions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	byKey := map[string]ForeignSubscription{}
	for _, s := range got {
		byKey[s.Key] = s
	}
	sky, ok := byKey["https://sky.example/feed"]
	if !ok || sky.App != ForeignAppSkyreader || sky.Kind != "rss" {
		t.Errorf("skyreader entry = %+v, ok=%v, want App=skyreader Kind=rss", sky, ok)
	}
	glean, ok := byKey["https://glean.example/feed"]
	if !ok || glean.App != ForeignAppGlean || glean.Kind != "rss" {
		t.Errorf("glean entry = %+v, ok=%v, want App=glean Kind=rss", glean, ok)
	}
}

func TestClient_CrawlOwnForeignSubscriptions_EmptyBothCollectionsYieldsEmptySlice(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlOwnForeignSubscriptions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty", got)
	}
}

func TestClient_CrawlOwnForeignSubscriptions_DedupesAcrossAppsPreferringSkyreader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case skyreaderSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{subRecord("at://x/1", "https://both.example/feed")}})
		case gleanSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{subRecord("at://y/1", "https://both.example/feed")}})
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlOwnForeignSubscriptions: %v", err)
	}
	if len(got) != 1 || got[0].App != ForeignAppSkyreader {
		t.Fatalf("got = %+v, want one skyreader-credited entry", got)
	}
}

func TestClient_CrawlOwnForeignSubscriptions_SkipsRecordsWithoutFeedURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != skyreaderSubscriptionCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"records": []any{
			map[string]any{"uri": "at://x/1", "value": map[string]any{"createdAt": "2026-07-01T00:00:00Z"}}, // no feedUrl
			subRecord("at://x/2", "https://good.example/feed"),
		}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlOwnForeignSubscriptions(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlOwnForeignSubscriptions: %v", err)
	}
	if len(got) != 1 || got[0].Key != "https://good.example/feed" {
		t.Fatalf("got = %+v, want only the well-formed record", got)
	}
}

func TestClient_CrawlOwnForeignSubscriptions_GleanFailurePropagatesError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		if coll == gleanSubscriptionCollection {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	if _, err := client.CrawlOwnForeignSubscriptions(context.Background(), did); err == nil {
		t.Fatal("expected an error when the glean fetch fails")
	}
}

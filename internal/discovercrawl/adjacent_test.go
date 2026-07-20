package discovercrawl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func followRecord(uri, subject string) map[string]any {
	return map[string]any{"uri": uri, "value": map[string]any{"subject": subject, "createdAt": "2026-07-01T00:00:00Z"}}
}

func TestClient_CrawlAdjacentFollows_AggregatesBlueskyAndTangled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case bskyFollowCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{
				followRecord("at://"+followedDID+"/x/1", "did:plc:alice"),
			}})
		case tangledFollowCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{
				followRecord("at://"+followedDID+"/y/1", "did:plc:bob"),
			}})
		default:
			t.Fatalf("unexpected collection %q", coll)
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	byDID := map[string]string{}
	for _, f := range got {
		byDID[f.DID] = f.Network
	}
	if byDID["did:plc:alice"] != "bluesky" {
		t.Errorf("alice network = %q, want bluesky", byDID["did:plc:alice"])
	}
	if byDID["did:plc:bob"] != "tangled" {
		t.Errorf("bob network = %q, want tangled", byDID["did:plc:bob"])
	}
}

func TestClient_CrawlAdjacentFollows_DedupesAcrossNetworksPreferringBluesky(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case bskyFollowCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{followRecord("at://x/1", "did:plc:both")}})
		case tangledFollowCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{followRecord("at://y/1", "did:plc:both")}})
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:both" || got[0].Network != "bluesky" {
		t.Fatalf("got = %+v, want one bluesky-credited entry", got)
	}
}

func TestClient_CrawlAdjacentFollows_TangledFailureDegradesToSkipAndLog(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		if coll == tangledFollowCollection {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{followRecord("at://x/1", "did:plc:alice")}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows should not fail when only the tangled fetch errors: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" || got[0].Network != "bluesky" {
		t.Fatalf("got = %+v, want only the bluesky follow", got)
	}
}

func TestClient_CrawlAdjacentFollows_BlueskyFailurePropagatesError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		if coll == bskyFollowCollection {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	if _, err := client.CrawlAdjacentFollows(context.Background(), did); err == nil {
		t.Fatal("expected an error when the bluesky follow fetch fails")
	}
}

func TestClient_CrawlAdjacentFollows_SkipsMalformedRecordsNotFatal(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != bskyFollowCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"records": []any{
			map[string]any{"uri": "at://x/1", "value": map[string]any{"createdAt": "2026-07-01T00:00:00Z"}}, // no subject
			followRecord("at://x/2", "did:plc:good"),
		}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows should not fail on a malformed record: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:good" {
		t.Fatalf("got = %+v, want only the well-formed follow", got)
	}
}

func TestClient_CrawlAdjacentFollows_SkipsSelfFollow(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != bskyFollowCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"records": []any{followRecord("at://x/1", followedDID)}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want a self-follow record excluded", got)
	}
}

func TestClient_CrawlAdjacentFollows_BoundsCrawledSetAndDropsDeterministically(t *testing.T) {
	const total = maxAdjacentCrawlDIDs + 20
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != bskyFollowCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		records := make([]any, total)
		for i := 0; i < total; i++ {
			records[i] = followRecord(fmt.Sprintf("at://x/%d", i), fmt.Sprintf("did:plc:p%04d", i))
		}
		json.NewEncoder(w).Encode(map[string]any{"records": records})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAdjacentFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAdjacentFollows: %v", err)
	}
	if len(got) != maxAdjacentCrawlDIDs {
		t.Fatalf("len = %d, want the bound %d (account has %d follows)", len(got), maxAdjacentCrawlDIDs, total)
	}
}

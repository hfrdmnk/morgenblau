package discovercrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func TestClient_CrawlReaderNetworkFollows_AggregatesMorgenAndSkyreader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case morgenFollowCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{
				followRecord("at://"+followedDID+"/x/1", "did:plc:alice"),
			}})
		case skyreaderFollowCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{
				followRecord("at://"+followedDID+"/y/1", "did:plc:bob"),
			}})
		default:
			t.Fatalf("unexpected collection %q", coll)
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlReaderNetworkFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlReaderNetworkFollows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	dids := map[string]bool{}
	for _, f := range got {
		dids[f.DID] = true
	}
	if !dids["did:plc:alice"] || !dids["did:plc:bob"] {
		t.Errorf("got = %+v, want alice and bob", got)
	}
}

func TestClient_CrawlReaderNetworkFollows_DedupesAcrossCollections(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{followRecord("at://x/1", "did:plc:both")}})
		_ = coll
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlReaderNetworkFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlReaderNetworkFollows: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:both" {
		t.Fatalf("got = %+v, want a single deduped entry", got)
	}
}

func TestClient_CrawlReaderNetworkFollows_SkyreaderFailureDegradesToSkipAndLog(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		if coll == skyreaderFollowCollection {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{followRecord("at://x/1", "did:plc:alice")}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlReaderNetworkFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlReaderNetworkFollows should not fail when only skyreader errors: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" {
		t.Fatalf("got = %+v, want only the morgen follow", got)
	}
}

func TestClient_CrawlReaderNetworkFollows_MorgenFailurePropagatesError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		if coll == morgenFollowCollection {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	if _, err := client.CrawlReaderNetworkFollows(context.Background(), did); err == nil {
		t.Fatal("expected an error when the morgen follow fetch fails")
	}
}

func TestClient_CrawlReaderNetworkFollows_SkipsSelfFollowAndMalformedRecords(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != morgenFollowCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"records": []any{
			followRecord("at://x/1", followedDID),
			map[string]any{"uri": "at://x/2", "value": map[string]any{"createdAt": "2026-07-01T00:00:00Z"}}, // no subject
			followRecord("at://x/3", "did:plc:good"),
		}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlReaderNetworkFollows(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlReaderNetworkFollows: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:good" {
		t.Fatalf("got = %+v, want only the well-formed non-self follow", got)
	}
}

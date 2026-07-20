package discoverbatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/discovercrawl"
)

// fakeRelayByCollection serves distinct DID sets per collection query param, unlike fakeRelay which serves the same set regardless.
func fakeRelayByCollection(t *testing.T, byCollection map[string][]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		dids := byCollection[coll]
		repos := make([]map[string]any, len(dids))
		for i, d := range dids {
			repos[i] = map[string]any{"did": d}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"repos": repos})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBatch_Run_FollowPass_WritesFollowerRowsForEachCrawledRepo(t *testing.T) {
	relay := fakeRelayByCollection(t, map[string][]string{
		"site.standard.publication": {},
		"blue.morgen.graph.follow":  {"did:plc:alice"},
	})
	crawler := newFakeRepoCrawler()
	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{
		{DID: "did:plc:bob"}, {DID: "did:plc:carol"},
	}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:alice": "https://pds-a.example",
	}}, crawler, fakeEntries{})
	b.collections = []string{"site.standard.publication"}
	b.followCollections = []string{"blue.morgen.graph.follow"}
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rows := w.followRowsFor("did:plc:alice")
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2", rows)
	}
	subjects := map[string]bool{}
	for _, r := range rows {
		subjects[r.SubjectDid] = true
	}
	if !subjects["did:plc:bob"] || !subjects["did:plc:carol"] {
		t.Errorf("rows = %+v, want bob and carol", rows)
	}
}

func TestBatch_Run_FollowPass_SameDayRerunReplacesRatherThanAccumulates(t *testing.T) {
	relay := fakeRelayByCollection(t, map[string][]string{
		"blue.morgen.graph.follow": {"did:plc:alice"},
	})
	crawler := newFakeRepoCrawler()
	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:bob"}}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:alice": "https://pds-a.example",
	}}, crawler, fakeEntries{})
	b.collections = nil
	b.followCollections = []string{"blue.morgen.graph.follow"}
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if got := w.followRowsFor("did:plc:alice"); len(got) != 1 || got[0].SubjectDid != "did:plc:bob" {
		t.Fatalf("after first run = %+v", got)
	}

	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:carol"}}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	got := w.followRowsFor("did:plc:alice")
	if len(got) != 1 || got[0].SubjectDid != "did:plc:carol" {
		t.Fatalf("after second run = %+v, want only carol (diff/replace, not accumulate)", got)
	}
}

func TestBatch_Run_FollowPass_OneRepoCrawlFailureDegradesNotAborts(t *testing.T) {
	relay := fakeRelayByCollection(t, map[string][]string{
		"blue.morgen.graph.follow": {"did:plc:broken", "did:plc:ok"},
	})
	crawler := newFakeRepoCrawler()
	crawler.failFollowDIDs["did:plc:broken"] = true
	crawler.follows["did:plc:ok"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:dan"}}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:broken": "https://pds-a.example",
		"did:plc:ok":     "https://pds-a.example",
	}}, crawler, fakeEntries{})
	b.collections = nil
	b.followCollections = []string{"blue.morgen.graph.follow"}
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run should not fail when one repo's follow crawl fails: %v", err)
	}
	if got := w.followRowsFor("did:plc:ok"); len(got) != 1 {
		t.Errorf("ok repo rows = %+v, want 1", got)
	}
	if got := w.followRowsFor("did:plc:broken"); len(got) != 0 {
		t.Errorf("broken repo rows = %+v, want none written", got)
	}
}

func TestBatch_Run_FollowPass_TransientCrawlFailureKeepsPriorAggregates(t *testing.T) {
	relay := fakeRelayByCollection(t, map[string][]string{
		"blue.morgen.graph.follow": {"did:plc:alice"},
	})
	crawler := newFakeRepoCrawler()
	crawler.follows["did:plc:alice"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:bob"}}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:alice": "https://pds-a.example",
	}}, crawler, fakeEntries{})
	b.collections = nil
	b.followCollections = []string{"blue.morgen.graph.follow"}
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Alice's PDS flakes on this pass; her prior aggregate must survive rather than be wiped by a delete-then-insert of nothing.
	crawler.failFollowDIDs["did:plc:alice"] = true
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	if got := w.followRowsFor("did:plc:alice"); len(got) != 1 || got[0].SubjectDid != "did:plc:bob" {
		t.Fatalf("after failed crawl = %+v, want bob's row kept", got)
	}
}

func TestBatch_Run_FollowPass_RelayFailurePropagatesAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := New(srv.URL, srv.Client(), &fakeBatchResolver{}, newFakeRepoCrawler(), fakeEntries{})
	b.collections = nil
	b.followCollections = []string{"blue.morgen.graph.follow"}
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err == nil {
		t.Fatal("expected follow-collection relay failure to propagate")
	}
}

func TestBatch_Run_FollowPass_SharesThrottleAndConcurrencyWithSignalPass(t *testing.T) {
	relay := fakeRelayByCollection(t, map[string][]string{
		"site.standard.publication": {"did:plc:signal-repo"},
		"blue.morgen.graph.follow":  {"did:plc:follow-repo"},
	})
	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:signal-repo"] = []discovercrawl.Subscription{{Key: "https://a.example/feed", Kind: "rss"}}
	crawler.follows["did:plc:follow-repo"] = []discovercrawl.ReaderNetworkFollow{{DID: "did:plc:zoe"}}

	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: map[string]string{
		"did:plc:signal-repo": "https://pds-a.example",
		"did:plc:follow-repo": "https://pds-b.example",
	}}, crawler, fakeEntries{})
	b.collections = []string{"site.standard.publication"}
	b.followCollections = []string{"blue.morgen.graph.follow"}
	w := newFakeWriter()
	withFakeTx(b, w)

	n, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1 (signal repos only)", n)
	}
	if got := w.rowsFor("did:plc:signal-repo"); len(got) != 1 {
		t.Errorf("signal rows = %+v, want 1", got)
	}
	if got := w.followRowsFor("did:plc:follow-repo"); len(got) != 1 {
		t.Errorf("follow rows = %+v, want 1", got)
	}
}

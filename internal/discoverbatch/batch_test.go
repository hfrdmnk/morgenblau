package discoverbatch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

// fakeRelay serves a fixed DID set for every collection query, skipping per-collection fixtures.
func fakeRelay(t *testing.T, dids []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repos := make([]map[string]any, len(dids))
		for i, d := range dids {
			repos[i] = map[string]any{"did": d}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"repos": repos})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type fakeRepoCrawler struct {
	mu             sync.Mutex
	subs           map[string][]discovercrawl.Subscription
	pubs           map[string][]discovercrawl.AuthoredPublication
	shares         map[string][]discovercrawl.Share
	saves          map[string][]discovercrawl.Save
	follows        map[string][]discovercrawl.ReaderNetworkFollow
	failDIDs       map[string]bool
	failMethods    map[string]map[string]bool
	failFollowDIDs map[string]bool
	// calls records every crawl call in order, "<did>:<method>", to assert per-repo crawl-before-write ordering.
	calls []string
}

func newFakeRepoCrawler() *fakeRepoCrawler {
	return &fakeRepoCrawler{
		subs:           map[string][]discovercrawl.Subscription{},
		pubs:           map[string][]discovercrawl.AuthoredPublication{},
		shares:         map[string][]discovercrawl.Share{},
		saves:          map[string][]discovercrawl.Save{},
		follows:        map[string][]discovercrawl.ReaderNetworkFollow{},
		failDIDs:       map[string]bool{},
		failMethods:    map[string]map[string]bool{},
		failFollowDIDs: map[string]bool{},
	}
}

func (f *fakeRepoCrawler) record(did, method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, did+":"+method)
	if f.failDIDs[did] || f.failMethods[did][method] {
		return errors.New("simulated crawl failure")
	}
	return nil
}

func (f *fakeRepoCrawler) Crawl(_ context.Context, did syntax.DID) ([]discovercrawl.Subscription, error) {
	d := did.String()
	if err := f.record(d, "Crawl"); err != nil {
		return nil, err
	}
	return f.subs[d], nil
}

func (f *fakeRepoCrawler) CrawlAuthoredPublications(_ context.Context, did syntax.DID) ([]discovercrawl.AuthoredPublication, error) {
	d := did.String()
	if err := f.record(d, "CrawlAuthoredPublications"); err != nil {
		return nil, err
	}
	return f.pubs[d], nil
}

func (f *fakeRepoCrawler) CrawlShares(_ context.Context, did syntax.DID) ([]discovercrawl.Share, error) {
	d := did.String()
	if err := f.record(d, "CrawlShares"); err != nil {
		return nil, err
	}
	return f.shares[d], nil
}

func (f *fakeRepoCrawler) CrawlSaves(_ context.Context, did syntax.DID) ([]discovercrawl.Save, error) {
	d := did.String()
	if err := f.record(d, "CrawlSaves"); err != nil {
		return nil, err
	}
	return f.saves[d], nil
}

func (f *fakeRepoCrawler) CrawlReaderNetworkFollows(_ context.Context, did syntax.DID) ([]discovercrawl.ReaderNetworkFollow, error) {
	f.mu.Lock()
	d := did.String()
	f.calls = append(f.calls, d+":CrawlReaderNetworkFollows")
	fail := f.failFollowDIDs[d]
	f.mu.Unlock()
	if fail {
		return nil, errors.New("simulated follow crawl failure")
	}
	return f.follows[d], nil
}

type fakeBatchResolver struct {
	hostByDID map[string]string
}

func (f *fakeBatchResolver) LookupDID(_ context.Context, did syntax.DID) (*identity.Identity, error) {
	host, ok := f.hostByDID[did.String()]
	if !ok {
		return nil, errors.New("unknown did")
	}
	return &identity.Identity{
		DID:      did,
		Services: map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: host}},
	}, nil
}

type fakeEntries struct{}

func (fakeEntries) GetFeedURLByGuid(context.Context, string) (string, error) {
	return "", errors.New("not found")
}
func (fakeEntries) GetFeedURLByItemURL(context.Context, string) (string, error) {
	return "", errors.New("not found")
}

// fakeWriter mimics the aggregate tables' diff/replace semantics in memory: Delete clears a repo's rows, Insert appends.
type fakeWriter struct {
	mu            sync.Mutex
	byRepo        map[string][]db.InsertDiscoverTrendingSignalParams
	followsByRepo map[string][]db.InsertDiscoverTrendingFollowParams
	deletes       int
	inserts       int
	followDeletes int
	followInserts int
	failNext      bool
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{
		byRepo:        map[string][]db.InsertDiscoverTrendingSignalParams{},
		followsByRepo: map[string][]db.InsertDiscoverTrendingFollowParams{},
	}
}

func (f *fakeWriter) DeleteDiscoverTrendingSignalsForRepo(_ context.Context, repoDid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	delete(f.byRepo, repoDid)
	return nil
}

func (f *fakeWriter) InsertDiscoverTrendingSignal(_ context.Context, arg db.InsertDiscoverTrendingSignalParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return errors.New("simulated write failure")
	}
	f.inserts++
	f.byRepo[arg.RepoDid] = append(f.byRepo[arg.RepoDid], arg)
	return nil
}

func (f *fakeWriter) DeleteDiscoverTrendingFollowsForRepo(_ context.Context, repoDid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.followDeletes++
	delete(f.followsByRepo, repoDid)
	return nil
}

func (f *fakeWriter) InsertDiscoverTrendingFollow(_ context.Context, arg db.InsertDiscoverTrendingFollowParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.followInserts++
	f.followsByRepo[arg.RepoDid] = append(f.followsByRepo[arg.RepoDid], arg)
	return nil
}

func (f *fakeWriter) rowsFor(repoDID string) []db.InsertDiscoverTrendingSignalParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.InsertDiscoverTrendingSignalParams, len(f.byRepo[repoDID]))
	copy(out, f.byRepo[repoDID])
	return out
}

func (f *fakeWriter) followRowsFor(repoDID string) []db.InsertDiscoverTrendingFollowParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.InsertDiscoverTrendingFollowParams, len(f.followsByRepo[repoDID]))
	copy(out, f.followsByRepo[repoDID])
	return out
}

// withFakeTx wires a Batch's runTx straight at a fakeWriter, skipping real SQL since this file tests orchestration, not sqlc plumbing.
func withFakeTx(b *Batch, w *fakeWriter) *Batch {
	b.runTx = func(ctx context.Context, fn func(Writer) error) error {
		return fn(w)
	}
	return b
}

func newTestBatch(t *testing.T, relayDIDs []string, hostByDID map[string]string, crawler RepoCrawler) (*Batch, *httptest.Server) {
	t.Helper()
	relay := fakeRelay(t, relayDIDs)
	b := New(relay.URL, relay.Client(), &fakeBatchResolver{hostByDID: hostByDID}, crawler, fakeEntries{})
	b.collections = []string{"site.standard.publication"} // one collection is enough; relay ignores it anyway
	// Follow enumeration is off by default: fakeRelay returns the same DIDs for every collection, so a non-nil default would leak into every test.
	b.followCollections = nil
	return b, relay
}

func TestBatch_Run_WritesEachRepoAfterItsOwnCrawlCompletes(t *testing.T) {
	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://a.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"}}
	crawler.subs["did:plc:bob"] = []discovercrawl.Subscription{{Key: "https://b.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"}}

	b, _ := newTestBatch(t, []string{"did:plc:alice", "did:plc:bob"}, map[string]string{
		"did:plc:alice": "https://pds-a.example",
		"did:plc:bob":   "https://pds-b.example",
	}, crawler)
	w := newFakeWriter()
	withFakeTx(b, w)

	n, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}

	aliceRows := w.rowsFor("did:plc:alice")
	if len(aliceRows) != 1 || aliceRows[0].SourceKey != "https://a.example/feed" {
		t.Errorf("alice rows = %+v", aliceRows)
	}
	bobRows := w.rowsFor("did:plc:bob")
	if len(bobRows) != 1 || bobRows[0].SourceKey != "https://b.example/feed" {
		t.Errorf("bob rows = %+v", bobRows)
	}
	if w.deletes != 2 {
		t.Errorf("deletes = %d, want 2 (one per repo, before its insert)", w.deletes)
	}
}

func TestBatch_Run_NetworkPhaseCompletesBeforeWriteTransactionOpens(t *testing.T) {
	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://a.example/feed", Kind: "rss"}}

	b, _ := newTestBatch(t, []string{"did:plc:alice"}, map[string]string{"did:plc:alice": "https://pds-a.example"}, crawler)

	// The tx-open spy appends to the crawler's own call log (same mutex) so the
	// recorded order proves the tx opened strictly after all four crawls returned.
	b.runTx = func(ctx context.Context, fn func(Writer) error) error {
		crawler.mu.Lock()
		crawler.calls = append(crawler.calls, "did:plc:alice:tx-open")
		crawler.mu.Unlock()
		return fn(newFakeWriter())
	}

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	crawler.mu.Lock()
	order := append([]string(nil), crawler.calls...)
	crawler.mu.Unlock()

	if len(order) != 5 {
		t.Fatalf("order = %v, want 4 crawl calls + 1 tx-open", order)
	}
	if order[4] != "did:plc:alice:tx-open" {
		t.Errorf("order = %v, want tx-open last (network phase complete before the transaction opens)", order)
	}
}

func TestBatch_Run_SameDayRerunReplacesRatherThanAccumulates(t *testing.T) {
	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://old.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"}}

	b, _ := newTestBatch(t, []string{"did:plc:alice"}, map[string]string{"did:plc:alice": "https://pds-a.example"}, crawler)
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if got := w.rowsFor("did:plc:alice"); len(got) != 1 || got[0].SourceKey != "https://old.example/feed" {
		t.Fatalf("after first run = %+v", got)
	}

	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://new.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"}}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	got := w.rowsFor("did:plc:alice")
	if len(got) != 1 || got[0].SourceKey != "https://new.example/feed" {
		t.Fatalf("after second run = %+v, want only the new source key (diff/replace, not accumulate)", got)
	}
}

func TestBatch_Run_PartialCrawlFailureKeepsPriorSnapshot(t *testing.T) {
	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://old.example/feed", Kind: "rss"}}

	b, _ := newTestBatch(t, []string{"did:plc:alice"}, map[string]string{"did:plc:alice": "https://pds-a.example"}, crawler)
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	crawler.subs["did:plc:alice"] = []discovercrawl.Subscription{{Key: "https://new.example/feed", Kind: "rss"}}
	crawler.failMethods["did:plc:alice"] = map[string]bool{"CrawlShares": true}
	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	got := w.rowsFor("did:plc:alice")
	if len(got) != 1 || got[0].SourceKey != "https://old.example/feed" {
		t.Fatalf("after partial crawl failure = %+v, want prior snapshot preserved", got)
	}
	if w.deletes != 1 {
		t.Errorf("deletes = %d, want only the successful first run", w.deletes)
	}
}

func TestBatch_Run_OneRepoCrawlFailureDegradesNotAborts(t *testing.T) {
	crawler := newFakeRepoCrawler()
	crawler.failDIDs["did:plc:broken"] = true
	crawler.subs["did:plc:ok"] = []discovercrawl.Subscription{{Key: "https://ok.example/feed", Kind: "rss"}}

	b, _ := newTestBatch(t, []string{"did:plc:broken", "did:plc:ok"}, map[string]string{
		"did:plc:broken": "https://pds-a.example",
		"did:plc:ok":     "https://pds-a.example",
	}, crawler)
	w := newFakeWriter()
	withFakeTx(b, w)

	n, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run should not fail when one repo's crawl fails: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2 (both repos attempted)", n)
	}
	if got := w.rowsFor("did:plc:ok"); len(got) != 1 {
		t.Errorf("ok repo rows = %+v, want 1", got)
	}
	if got := w.rowsFor("did:plc:broken"); len(got) != 0 {
		t.Errorf("broken repo rows = %+v, want none written (all four signal kinds failed to crawl)", got)
	}
}

func TestBatch_Run_UnresolvableRepoDegradesNotAborts(t *testing.T) {
	crawler := newFakeRepoCrawler()
	crawler.subs["did:plc:ok"] = []discovercrawl.Subscription{{Key: "https://ok.example/feed", Kind: "rss"}}

	// did:plc:unresolvable is absent from the resolver's host map, so LookupDID fails as it would for a deleted identity.
	b, _ := newTestBatch(t, []string{"did:plc:unresolvable", "did:plc:ok"}, map[string]string{
		"did:plc:ok": "https://pds-a.example",
	}, crawler)
	w := newFakeWriter()
	withFakeTx(b, w)

	n, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2", n)
	}
	if got := w.rowsFor("did:plc:ok"); len(got) != 1 {
		t.Errorf("ok repo rows = %+v, want 1", got)
	}
}

func TestBatch_Run_EmptyEnumerationWritesNothing(t *testing.T) {
	crawler := newFakeRepoCrawler()
	b, _ := newTestBatch(t, nil, nil, crawler)
	w := newFakeWriter()
	withFakeTx(b, w)

	n, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if len(crawler.calls) != 0 {
		t.Errorf("calls = %v, want none", crawler.calls)
	}
}

func TestBatch_Run_RelayFailurePropagatesAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := New(srv.URL, srv.Client(), &fakeBatchResolver{}, newFakeRepoCrawler(), fakeEntries{})
	b.collections = []string{"site.standard.publication"}
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err == nil {
		t.Fatal("expected relay failure to propagate")
	}
}

func TestBatch_Run_RespectsGlobalConcurrencyBound(t *testing.T) {
	var inFlight, maxInFlight int64
	crawler := newFakeRepoCrawler()
	dids := make([]string, 20)
	hostByDID := map[string]string{}
	for i := range dids {
		did := "did:plc:repo" + string(rune('a'+i))
		dids[i] = did
		hostByDID[did] = "https://pds-" + string(rune('a'+i)) + ".example" // distinct hosts: only the global bound applies
	}

	b, _ := newTestBatch(t, dids, hostByDID, &countingCrawler{fakeRepoCrawler: crawler, inFlight: &inFlight, maxInFlight: &maxInFlight})
	b.globalConcurrency = 3
	w := newFakeWriter()
	withFakeTx(b, w)

	if _, err := b.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt64(&maxInFlight); got > 3 {
		t.Errorf("max concurrent repos = %d, want <= 3", got)
	}
}

// countingCrawler wraps fakeRepoCrawler with an artificial delay and concurrency tracking so global-bound tests observe real overlap.
type countingCrawler struct {
	*fakeRepoCrawler
	inFlight, maxInFlight *int64
}

func (c *countingCrawler) Crawl(ctx context.Context, did syntax.DID) ([]discovercrawl.Subscription, error) {
	cur := atomic.AddInt64(c.inFlight, 1)
	defer atomic.AddInt64(c.inFlight, -1)
	for {
		max := atomic.LoadInt64(c.maxInFlight)
		if cur <= max || atomic.CompareAndSwapInt64(c.maxInFlight, max, cur) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	return c.fakeRepoCrawler.Crawl(ctx, did)
}

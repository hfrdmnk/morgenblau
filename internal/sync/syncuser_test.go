package sync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

type fakeStore struct {
	mu          sync.Mutex
	rows        map[string]map[string]db.ListUserSubscriptionsForSyncRow // did -> rkey -> row
	deletes     []string
	upserts     int
	feedUps     int
	feedErr     func(feedURL string) error
	saves       map[string]map[string]db.ListUserSavesForSyncRow // did -> rkey -> row
	saveDeletes []string
	saveUpserts int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rows:  map[string]map[string]db.ListUserSubscriptionsForSyncRow{},
		saves: map[string]map[string]db.ListUserSavesForSyncRow{},
	}
}

func (s *fakeStore) ListUserSavesForSync(_ context.Context, did string) ([]db.ListUserSavesForSyncRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]db.ListUserSavesForSyncRow, 0, len(s.saves[did]))
	for _, r := range s.saves[did] {
		rows = append(rows, r)
	}
	return rows, nil
}

func (s *fakeStore) UpsertUserSave(_ context.Context, arg db.UpsertUserSaveParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveUpserts++
	if _, ok := s.saves[arg.Did]; !ok {
		s.saves[arg.Did] = map[string]db.ListUserSavesForSyncRow{}
	}
	s.saves[arg.Did][arg.Rkey] = db.ListUserSavesForSyncRow{
		Did:     arg.Did,
		Rkey:    arg.Rkey,
		AtUri:   arg.AtUri,
		ItemUrl: arg.ItemUrl,
		FeedUrl: arg.FeedUrl,
	}
	return nil
}

func (s *fakeStore) DeleteUserSave(_ context.Context, arg db.DeleteUserSaveParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveDeletes = append(s.saveDeletes, arg.Rkey)
	if m, ok := s.saves[arg.Did]; ok {
		delete(m, arg.Rkey)
	}
	return nil
}

func (s *fakeStore) ListUserSubscriptionsForSync(_ context.Context, did string) ([]db.ListUserSubscriptionsForSyncRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]db.ListUserSubscriptionsForSyncRow, 0, len(s.rows[did]))
	for _, r := range s.rows[did] {
		rows = append(rows, r)
	}
	return rows, nil
}

func (s *fakeStore) UpsertUserSubscription(_ context.Context, arg db.UpsertUserSubscriptionParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts++
	if _, ok := s.rows[arg.Did]; !ok {
		s.rows[arg.Did] = map[string]db.ListUserSubscriptionsForSyncRow{}
	}
	s.rows[arg.Did][arg.Rkey] = db.ListUserSubscriptionsForSyncRow{
		Did:     arg.Did,
		Rkey:    arg.Rkey,
		AtUri:   arg.AtUri,
		FeedUrl: arg.FeedUrl,
		Title:   arg.Title,
	}
	return nil
}

func (s *fakeStore) DeleteUserSubscription(_ context.Context, arg db.DeleteUserSubscriptionParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes = append(s.deletes, arg.Rkey)
	if m, ok := s.rows[arg.Did]; ok {
		delete(m, arg.Rkey)
	}
	return nil
}

func (s *fakeStore) UpsertFeed(_ context.Context, arg db.UpsertFeedParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feedUps++
	if s.feedErr != nil {
		if err := s.feedErr(arg.FeedUrl); err != nil {
			return err
		}
	}
	return nil
}

type fakeLister struct {
	calls int32
	delay time.Duration
	subs  []PDSSubscription
	saves []PDSSave
}

func (f *fakeLister) ListSubscriptions(_ context.Context, _ *oauth.ClientSession) ([]PDSSubscription, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.subs, nil
}

func (f *fakeLister) ListSaves(_ context.Context, _ *oauth.ClientSession) ([]PDSSave, error) {
	return f.saves, nil
}

type countingFetcher struct {
	mu      sync.Mutex
	delay   time.Duration
	fetched []string
}

func (f *countingFetcher) FetchAndStore(_ context.Context, url string) error {
	f.mu.Lock()
	f.fetched = append(f.fetched, url)
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return nil
}

func (f *countingFetcher) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.fetched))
	copy(out, f.fetched)
	return out
}

func newSession(did string) *oauth.ClientSession {
	d, _ := syntax.ParseDID(did)
	return &oauth.ClientSession{
		Data: &oauth.ClientSessionData{AccountDID: d, SessionID: "sid-1"},
	}
}

func TestSyncUser_ReconcileApplies_InsertsAndDeletes(t *testing.T) {
	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.ListUserSubscriptionsForSyncRow{
		"oldA": {Did: "did:plc:alice", Rkey: "oldA", AtUri: "at://x/a/oldA", FeedUrl: "https://feed/old"},
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/newB", Rkey: "newB", FeedURL: "https://feed/new", Title: "New"},
	}}
	fetcher := &countingFetcher{}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	if store.upserts != 1 {
		t.Errorf("upserts = %d, want 1", store.upserts)
	}
	if len(store.deletes) != 1 || store.deletes[0] != "oldA" {
		t.Errorf("deletes = %v, want [oldA]", store.deletes)
	}
}

func TestSyncUser_ReconcileSaves_InsertsAndDeletes(t *testing.T) {
	store := newFakeStore()
	// A local save the PDS no longer has → should be deleted.
	store.saves["did:plc:alice"] = map[string]db.ListUserSavesForSyncRow{
		"goneA": {Did: "did:plc:alice", Rkey: "goneA", AtUri: "at://x/s/goneA", ItemUrl: "https://item/old"},
	}
	feed := "https://feed/new"
	lister := &fakeLister{saves: []PDSSave{
		{URI: "at://x/s/newB", Rkey: "newB", ItemURL: "https://item/new", FeedURL: feed, CreatedAt: "2026-06-01T00:00:00Z"},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil)

	if err := eng.reconcileSaves(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	if store.saveUpserts != 1 {
		t.Errorf("saveUpserts = %d, want 1", store.saveUpserts)
	}
	if got, ok := store.saves["did:plc:alice"]["newB"]; !ok {
		t.Error("remote save newB was not inserted locally")
	} else if got.ItemUrl != "https://item/new" || got.FeedUrl == nil || *got.FeedUrl != feed {
		t.Errorf("inserted save = %+v, want item/new + feed/new", got)
	}
	if len(store.saveDeletes) != 1 || store.saveDeletes[0] != "goneA" {
		t.Errorf("saveDeletes = %v, want [goneA]", store.saveDeletes)
	}
	if _, ok := store.saves["did:plc:alice"]["goneA"]; ok {
		t.Error("stale local save goneA was not deleted")
	}
}

func TestSyncUser_DualTrackParallelism(t *testing.T) {
	// Configure the lister and fetcher to each take ~80ms. If sync_user runs
	// them sequentially, total wall-clock would approach 160ms+ish; in parallel,
	// it should finish around max(80, 80) + some scheduling fuzz.
	const delay = 80 * time.Millisecond

	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.ListUserSubscriptionsForSyncRow{
		"k1": {Did: "did:plc:alice", Rkey: "k1", AtUri: "at://x/a/k1", FeedUrl: "https://existing"},
	}
	lister := &fakeLister{delay: delay, subs: []PDSSubscription{
		{URI: "at://x/a/k1", Rkey: "k1", FeedURL: "https://existing"},
	}}
	fetcher := &countingFetcher{delay: delay}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil)

	t0 := time.Now()
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(t0)
	if elapsed > 2*delay {
		t.Errorf("dual-track ran serially: %v (want < %v)", elapsed, 2*delay)
	}
}

func TestSyncUser_Phase2FetchesOnlyNewURLs(t *testing.T) {
	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.ListUserSubscriptionsForSyncRow{
		"k1": {Did: "did:plc:alice", Rkey: "k1", AtUri: "at://x/a/k1", FeedUrl: "https://old"},
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/k1", Rkey: "k1", FeedURL: "https://old"},
		{URI: "at://x/a/new", Rkey: "new", FeedURL: "https://new"},
	}}
	fetcher := &countingFetcher{}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	seen := fetcher.seen()
	// Both URLs should be fetched: old via Phase 1B (snapshot), new via Phase 2.
	if len(seen) != 2 {
		t.Errorf("fetches = %v, want both URLs", seen)
	}
	// And `old` should only appear once, not twice.
	oldCount := 0
	for _, u := range seen {
		if u == "https://old" {
			oldCount++
		}
	}
	if oldCount != 1 {
		t.Errorf("old fetched %d times, want 1", oldCount)
	}
}

func TestSyncUser_FK_NotCalledOnTier2Failure(t *testing.T) {
	// When Tier-2 UpsertFeed fails for a newly-discovered URL, Phase 2 must
	// NOT fetch (and downstream UpsertFeedEntry) — otherwise the FK
	// feed_entries.feed_url → feeds.feed_url violates silently.
	store := newFakeStore()
	store.feedErr = func(url string) error {
		if url == "https://broken/feed" {
			return errors.New("tier-2 upsert failed")
		}
		return nil
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/ok", Rkey: "ok", FeedURL: "https://ok/feed"},
		{URI: "at://x/a/broken", Rkey: "broken", FeedURL: "https://broken/feed"},
	}}
	fetcher := &countingFetcher{}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	for _, u := range fetcher.seen() {
		if u == "https://broken/feed" {
			t.Errorf("broken URL was fetched: would have hit FK violation; fetched = %v", fetcher.seen())
		}
	}
	// The good URL should still be fetched via Phase 2.
	found := false
	for _, u := range fetcher.seen() {
		if u == "https://ok/feed" {
			found = true
		}
	}
	if !found {
		t.Errorf("ok URL was not fetched: %v", fetcher.seen())
	}
}

func TestSyncUser_InFlightGuard_Coalesces(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, &countingFetcher{}, &nopResumer{})

	did := mustDID("did:plc:alice")

	// The first call sets the job running; the second within the guard should
	// return the same id rather than start a new job.
	id1, _ := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerLogin)
	id2, _ := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerLogin)
	if id1 != id2 {
		t.Errorf("guard didn't coalesce: id1=%s id2=%s", id1, id2)
	}
}

func TestSyncUser_ShutdownWaitsForRun(t *testing.T) {
	store := newFakeStore()
	// Slow lister so the run goroutine is still in flight when Shutdown fires.
	lister := &fakeLister{delay: 200 * time.Millisecond, subs: []PDSSubscription{
		{URI: "at://x/a/k1", Rkey: "k1", FeedURL: "https://example.com/feed"},
	}}
	fetcher := &countingFetcher{}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, fetcher, &nopResumer{})
	orch := New(tracker, fetcher, eng)

	id, err := orch.StartLoginRefresh(context.Background(), mustDID("did:plc:alice"), "sid-1")
	if err != nil {
		t.Fatalf("StartLoginRefresh: %v", err)
	}

	t0 := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := orch.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown returned %v, want nil", err)
	}
	elapsed := time.Since(t0)

	if elapsed < lister.delay {
		t.Errorf("Shutdown returned in %v; expected to block at least %v", elapsed, lister.delay)
	}
	// After Shutdown returns, the job must be in a terminal state — the run
	// goroutine has had its chance to update the tracker.
	j, err := tracker.Get(id, mustDID("did:plc:alice"))
	if err != nil {
		t.Fatalf("tracker.Get: %v", err)
	}
	if j.Status != jobs.StatusDone && j.Status != jobs.StatusFailed {
		t.Errorf("job status = %v; want done or failed", j.Status)
	}
}

func TestOrchestrator_ShutdownDeadlineExceeded(t *testing.T) {
	store := newFakeStore()
	// Lister that blocks longer than the Shutdown deadline.
	lister := &fakeLister{delay: 500 * time.Millisecond, subs: []PDSSubscription{
		{URI: "at://x/a/k1", Rkey: "k1", FeedURL: "https://example.com/feed"},
	}}
	tracker := jobs.New()
	fetcher := &countingFetcher{}
	eng := NewEngine(tracker, store, lister, fetcher, &nopResumer{})
	orch := New(tracker, fetcher, eng)

	if _, err := orch.StartLoginRefresh(context.Background(), mustDID("did:plc:alice"), "sid-1"); err != nil {
		t.Fatalf("StartLoginRefresh: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := orch.Shutdown(shutdownCtx); err == nil {
		t.Fatal("Shutdown returned nil; want ctx error")
	}
}

type nopResumer struct{}

func (nopResumer) ResumeSession(_ context.Context, did syntax.DID, sid string) (*oauth.ClientSession, error) {
	return &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: did, SessionID: sid}}, nil
}

func mustDID(s string) syntax.DID {
	d, err := syntax.ParseDID(s)
	if err != nil {
		panic(err)
	}
	return d
}

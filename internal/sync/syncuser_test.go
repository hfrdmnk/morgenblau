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
	mu               sync.Mutex
	rows             map[string]map[string]db.UserSubscription // did -> rkey -> row
	deletes          []string
	upserts          int
	upsertParams     map[string]db.UpsertUserSubscriptionParams // rkey -> last params
	feedUps          int
	feedParams       []db.UpsertFeedParams
	feedErr          func(feedURL string) error
	subUpsertErr     error
	subDeleteErr     error
	saveUpsertErr    error
	saveDeleteErr    error
	saves            map[string]map[string]db.UserSave // did -> rkey -> row
	saveDeletes      []string
	saveUpserts      int
	saveUpsertParams map[string]db.UpsertUserSaveParams // rkey -> last params
	// ops is a single ordered log across deletes and upserts so tests can assert delete-before-rekeyed-upsert ordering, which the maps above lose.
	ops []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		rows:             map[string]map[string]db.UserSubscription{},
		upsertParams:     map[string]db.UpsertUserSubscriptionParams{},
		saves:            map[string]map[string]db.UserSave{},
		saveUpsertParams: map[string]db.UpsertUserSaveParams{},
	}
}

func (s *fakeStore) assertDeleteBeforeUpsert(t *testing.T, delRkey, upRkey string) {
	t.Helper()
	delIndex, upsertIndex := -1, -1
	for i, op := range s.ops {
		if delIndex == -1 && op == "delete:"+delRkey {
			delIndex = i
		}
		if upsertIndex == -1 && op == "upsert:"+upRkey {
			upsertIndex = i
		}
	}
	if delIndex == -1 || upsertIndex == -1 || delIndex >= upsertIndex {
		t.Fatalf("operations = %v, want delete:%s before upsert:%s", s.ops, delRkey, upRkey)
	}
}

func (s *fakeStore) ListUserSavesForSync(_ context.Context, did string) ([]db.UserSave, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]db.UserSave, 0, len(s.saves[did]))
	for _, r := range s.saves[did] {
		rows = append(rows, r)
	}
	return rows, nil
}

func (s *fakeStore) UpsertUserSave(_ context.Context, arg db.UpsertUserSaveParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveUpserts++
	s.saveUpsertParams[arg.Rkey] = arg
	if s.saveUpsertErr != nil {
		return s.saveUpsertErr
	}
	if _, ok := s.saves[arg.Did]; !ok {
		s.saves[arg.Did] = map[string]db.UserSave{}
	}
	s.saves[arg.Did][arg.Rkey] = db.UserSave{
		Did:       arg.Did,
		Rkey:      arg.Rkey,
		AtUri:     arg.AtUri,
		ItemUrl:   arg.ItemUrl,
		FeedUrl:   arg.FeedUrl,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
	}
	return s.saveUpsertErr
}

func (s *fakeStore) DeleteUserSave(_ context.Context, arg db.DeleteUserSaveParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveDeletes = append(s.saveDeletes, arg.Rkey)
	if s.saveDeleteErr != nil {
		return s.saveDeleteErr
	}
	if m, ok := s.saves[arg.Did]; ok {
		delete(m, arg.Rkey)
	}
	return s.saveDeleteErr
}

func (s *fakeStore) ListUserSubscriptionsForSync(_ context.Context, did string) ([]db.UserSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]db.UserSubscription, 0, len(s.rows[did]))
	for _, r := range s.rows[did] {
		rows = append(rows, r)
	}
	return rows, nil
}

func (s *fakeStore) UpsertUserSubscription(_ context.Context, arg db.UpsertUserSubscriptionParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "upsert:"+arg.Rkey)
	s.upserts++
	s.upsertParams[arg.Rkey] = arg
	if s.subUpsertErr != nil {
		return s.subUpsertErr
	}
	if _, ok := s.rows[arg.Did]; !ok {
		s.rows[arg.Did] = map[string]db.UserSubscription{}
	}
	kind := "rss"
	if k, ok := arg.Kind.(string); ok && k != "" {
		kind = k
	}
	s.rows[arg.Did][arg.Rkey] = db.UserSubscription{
		Did:         arg.Did,
		Rkey:        arg.Rkey,
		AtUri:       arg.AtUri,
		FeedUrl:     arg.FeedUrl,
		Kind:        kind,
		SidecarRkey: arg.SidecarRkey,
		Title:       arg.Title,
		IsPrimary:   arg.IsPrimary,
		Tags:        arg.Tags,
		CreatedAt:   arg.CreatedAt,
		UpdatedAt:   arg.UpdatedAt,
	}
	return s.subUpsertErr
}

func (s *fakeStore) DeleteUserSubscription(_ context.Context, arg db.DeleteUserSubscriptionParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops = append(s.ops, "delete:"+arg.Rkey)
	s.deletes = append(s.deletes, arg.Rkey)
	if s.subDeleteErr != nil {
		return s.subDeleteErr
	}
	if m, ok := s.rows[arg.Did]; ok {
		delete(m, arg.Rkey)
	}
	return s.subDeleteErr
}

func (s *fakeStore) UpsertFeed(_ context.Context, arg db.UpsertFeedParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feedUps++
	s.feedParams = append(s.feedParams, arg)
	if s.feedErr != nil {
		if err := s.feedErr(arg.FeedUrl); err != nil {
			return err
		}
	}
	return nil
}

type fakeLister struct {
	calls         int32
	delay         time.Duration
	subs          []PDSSubscription
	subsErr       error
	beforeSubs    func()
	saves         []PDSSave
	savesErr      error
	beforeSaves   func()
	standardSubs  []PDSStandardSubscription
	standardErr   error
	savesCalls    atomic.Int32
	standardCalls atomic.Int32
}

func (f *fakeLister) ListSubscriptions(_ context.Context, _ *oauth.ClientSession) ([]PDSSubscription, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.beforeSubs != nil {
		f.beforeSubs()
	}
	return f.subs, f.subsErr
}

func (f *fakeLister) ListSaves(_ context.Context, _ *oauth.ClientSession) ([]PDSSave, error) {
	f.savesCalls.Add(1)
	if f.beforeSaves != nil {
		f.beforeSaves()
	}
	return f.saves, f.savesErr
}

func (f *fakeLister) ListStandardSubscriptions(_ context.Context, _ *oauth.ClientSession) ([]PDSStandardSubscription, error) {
	f.standardCalls.Add(1)
	return f.standardSubs, f.standardErr
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

func TestSyncUser_ReconcilesOnlyRetainedCollections(t *testing.T) {
	lister := &fakeLister{}
	eng := NewEngine(jobs.New(), newFakeStore(), lister, &countingFetcher{}, nil, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&lister.calls); got != 1 {
		t.Errorf("subscription list calls = %d, want 1", got)
	}
	if got := lister.standardCalls.Load(); got != 1 {
		t.Errorf("standard subscription list calls = %d, want 1", got)
	}
	if got := lister.savesCalls.Load(); got != 1 {
		t.Errorf("save list calls = %d, want 1", got)
	}
}

func TestSyncUser_ReconcileApplies_InsertsAndDeletes(t *testing.T) {
	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"oldA": {Did: "did:plc:alice", Rkey: "oldA", AtUri: "at://x/a/oldA", FeedUrl: "https://feed/old"},
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/newB", Kind: "rss", Rkey: "newB", FeedURL: "https://feed/new", Title: "New"},
	}}
	fetcher := &countingFetcher{}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil, nil)
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

func TestSyncUser_ReconcilePreservesPrimaryAndTags(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/withMeta", Kind: "rss", Rkey: "withMeta", FeedURL: "https://feed/meta", Primary: true, Tags: []string{"tech", "design"}},
		{URI: "at://x/a/bare", Kind: "rss", Rkey: "bare", FeedURL: "https://feed/bare"},
	}}
	eng := NewEngine(jobs.New(), store, lister, &countingFetcher{}, nil, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	withMeta := store.upsertParams["withMeta"]
	if withMeta.IsPrimary != 1 {
		t.Errorf("withMeta.IsPrimary = %d, want 1", withMeta.IsPrimary)
	}
	if withMeta.Tags == nil || *withMeta.Tags != `["tech","design"]` {
		t.Errorf("withMeta.Tags = %v, want [\"tech\",\"design\"]", withMeta.Tags)
	}

	bare := store.upsertParams["bare"]
	if bare.IsPrimary != 0 {
		t.Errorf("bare.IsPrimary = %d, want 0", bare.IsPrimary)
	}
	if bare.Tags != nil {
		t.Errorf("bare.Tags = %v, want nil", bare.Tags)
	}
}

func TestSyncUser_DualTrackParallelism(t *testing.T) {
	const delay = 80 * time.Millisecond

	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"k1": {Did: "did:plc:alice", Rkey: "k1", AtUri: "at://x/a/k1", FeedUrl: "https://existing"},
	}
	lister := &fakeLister{delay: delay, subs: []PDSSubscription{
		{URI: "at://x/a/k1", Kind: "rss", Rkey: "k1", FeedURL: "https://existing"},
	}}
	fetcher := &countingFetcher{delay: delay}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil, nil)

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
	store.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"k1": {Did: "did:plc:alice", Rkey: "k1", AtUri: "at://x/a/k1", FeedUrl: "https://old"},
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/k1", Kind: "rss", Rkey: "k1", FeedURL: "https://old"},
		{URI: "at://x/a/new", Kind: "rss", Rkey: "new", FeedURL: "https://new"},
	}}
	fetcher := &countingFetcher{}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err != nil {
		t.Fatal(err)
	}

	seen := fetcher.seen()
	if len(seen) != 2 {
		t.Errorf("fetches = %v, want both URLs", seen)
	}
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

func TestSyncUser_FailsWhenSubscriptionMirrorDoesNotReconcile(t *testing.T) {
	store := newFakeStore()
	store.subUpsertErr = errors.New("subscription write failed")
	lister := &fakeLister{subs: []PDSSubscription{{URI: "at://did:plc:alice/blue.morgen.feed.subscription/3sub", Kind: "rss", Rkey: "3sub", FeedURL: "https://example.com/feed"}}}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, &countingFetcher{}, &nopResumer{}, nil)
	did := mustDID("did:plc:alice")
	id, err := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	job := waitForTerminalJob(t, tracker, id, did)
	if job.Status != jobs.StatusFailed {
		t.Fatalf("status = %q, want failed after a subscription write error", job.Status)
	}
}

func TestSyncUser_FailsWhenSaveMirrorDoesNotReconcile(t *testing.T) {
	store := newFakeStore()
	store.saveUpsertErr = errors.New("save write failed")
	lister := &fakeLister{saves: []PDSSave{{URI: "at://did:plc:alice/blue.morgen.feed.save/3save", Rkey: "3save", ItemURL: "https://example.com/post", CreatedAt: "2026-07-20T12:00:00Z"}}}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, &countingFetcher{}, &nopResumer{}, nil)
	did := mustDID("did:plc:alice")
	id, err := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	job := waitForTerminalJob(t, tracker, id, did)
	if job.Status != jobs.StatusFailed {
		t.Fatalf("status = %q, want failed after a save write error", job.Status)
	}
}

type failingFetcher struct{ err error }

func (f failingFetcher) FetchAndStore(context.Context, string) error { return f.err }

func TestSyncUser_FetchFailureDoesNotFailReconciliation(t *testing.T) {
	store := newFakeStore()
	store.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"3sub": {Did: "did:plc:alice", Rkey: "3sub", AtUri: "at://did:plc:alice/blue.morgen.feed.subscription/3sub", FeedUrl: "https://example.com/feed", Kind: "rss"},
	}
	lister := &fakeLister{subs: []PDSSubscription{{URI: "at://did:plc:alice/blue.morgen.feed.subscription/3sub", Kind: "rss", Rkey: "3sub", FeedURL: "https://example.com/feed"}}}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, failingFetcher{err: errors.New("upstream unavailable")}, &nopResumer{}, nil)
	did := mustDID("did:plc:alice")
	id, err := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	job := waitForTerminalJob(t, tracker, id, did)
	if job.Status != jobs.StatusDone {
		t.Fatalf("status = %q, want done when only fetch failed", job.Status)
	}
}

func waitForTerminalJob(t *testing.T, tracker *jobs.Tracker, id string, did syntax.DID) *jobs.Job {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		job, err := tracker.Get(id, did)
		if err != nil {
			t.Fatalf("load job: %v", err)
		}
		if job.Status == jobs.StatusDone || job.Status == jobs.StatusFailed {
			return job
		}
		select {
		case <-deadline:
			t.Fatalf("job did not finish: %+v", job)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestSyncUser_FK_NotCalledOnTier2Failure(t *testing.T) {
	// A fetch on Tier-2 UpsertFeed failure would silently violate the feed_entries.feed_url FK.
	store := newFakeStore()
	store.feedErr = func(url string) error {
		if url == "https://broken/feed" {
			return errors.New("tier-2 upsert failed")
		}
		return nil
	}
	lister := &fakeLister{subs: []PDSSubscription{
		{URI: "at://x/a/broken", Kind: "rss", Rkey: "broken", FeedURL: "https://broken/feed"},
	}}
	fetcher := &countingFetcher{}
	eng := NewEngine(jobs.New(), store, lister, fetcher, nil, nil)
	if err := eng.runDualTrack(context.Background(), mustDID("did:plc:alice"), newSession("did:plc:alice")); err == nil {
		t.Fatal("expected Tier-2 failure to fail reconciliation")
	}
	if store.upserts != 0 {
		t.Errorf("Tier-1 upserts = %d, want none after Tier-2 failure", store.upserts)
	}

	for _, u := range fetcher.seen() {
		if u == "https://broken/feed" {
			t.Errorf("broken URL was fetched: would have hit FK violation; fetched = %v", fetcher.seen())
		}
	}
}

func TestSyncUser_InFlightGuard_Coalesces(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, &countingFetcher{}, &nopResumer{}, nil)

	did := mustDID("did:plc:alice")

	id1, _ := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerLogin)
	id2, _ := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerLogin)
	if id1 != id2 {
		t.Errorf("guard didn't coalesce: id1=%s id2=%s", id1, id2)
	}
}

type blockingPDSLister struct {
	calls         atomic.Int32
	firstStarted  chan struct{}
	firstContinue chan struct{}
	secondStarted chan struct{}
	secondWait    chan struct{}
}

func (l *blockingPDSLister) ListSubscriptions(context.Context, *oauth.ClientSession) ([]PDSSubscription, error) {
	switch l.calls.Add(1) {
	case 1:
		close(l.firstStarted)
		<-l.firstContinue
	case 2:
		close(l.secondStarted)
		<-l.secondWait
	}
	return nil, nil
}

func (*blockingPDSLister) ListStandardSubscriptions(context.Context, *oauth.ClientSession) ([]PDSStandardSubscription, error) {
	return nil, nil
}

func (*blockingPDSLister) ListSaves(context.Context, *oauth.ClientSession) ([]PDSSave, error) {
	return nil, nil
}

type sessionIDResumer struct{ sessions chan string }

func (r *sessionIDResumer) ResumeSession(_ context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error) {
	r.sessions <- sessionID
	return &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: did, SessionID: sessionID}}, nil
}

func TestSyncUser_ManualRefreshDuringAutomaticRunWaitsForLaterPass(t *testing.T) {
	lister := &blockingPDSLister{
		firstStarted:  make(chan struct{}),
		firstContinue: make(chan struct{}),
		secondStarted: make(chan struct{}),
		secondWait:    make(chan struct{}),
	}
	tracker := jobs.New()
	eng := NewEngine(tracker, newFakeStore(), lister, &countingFetcher{}, &nopResumer{}, nil)
	did := mustDID("did:plc:alice")

	id, err := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerLogin)
	if err != nil {
		t.Fatalf("start login sync: %v", err)
	}
	select {
	case <-lister.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first PDS pass did not start")
	}

	manualID, err := eng.SyncUser(context.Background(), did, "sid-1", jobs.TriggerManual)
	if err != nil {
		t.Fatalf("start manual sync: %v", err)
	}
	if manualID != id {
		t.Fatalf("manual job id = %q, want coalesced id %q", manualID, id)
	}
	close(lister.firstContinue)

	select {
	case <-lister.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("manual request completed without a later PDS pass")
	}
	if job, err := tracker.Get(id, did); err != nil || job.Status == jobs.StatusDone || job.Status == jobs.StatusFailed {
		t.Fatalf("job completed before the later pass: job=%+v err=%v", job, err)
	}
	close(lister.secondWait)

	deadline := time.After(time.Second)
	for {
		job, err := tracker.Get(id, did)
		if err != nil {
			t.Fatalf("load job: %v", err)
		}
		if job.Status == jobs.StatusDone {
			break
		}
		if job.Status == jobs.StatusFailed {
			t.Fatalf("job failed after successful later pass: %+v", job)
		}
		select {
		case <-deadline:
			t.Fatalf("job did not finish: %+v", job)
		case <-time.After(time.Millisecond):
		}
	}
	if got := lister.calls.Load(); got != 2 {
		t.Errorf("PDS passes = %d, want 2", got)
	}
}

func TestSyncUser_NewSessionDuringAutomaticRunUsesLatestSessionOnLaterPass(t *testing.T) {
	lister := &blockingPDSLister{
		firstStarted:  make(chan struct{}),
		firstContinue: make(chan struct{}),
		secondStarted: make(chan struct{}),
		secondWait:    make(chan struct{}),
	}
	resumer := &sessionIDResumer{sessions: make(chan string, 2)}
	tracker := jobs.New()
	eng := NewEngine(tracker, newFakeStore(), lister, &countingFetcher{}, resumer, nil)
	did := mustDID("did:plc:alice")

	id, err := eng.SyncUser(context.Background(), did, "sid-old", jobs.TriggerLogin)
	if err != nil {
		t.Fatalf("start first login sync: %v", err)
	}
	select {
	case <-lister.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first PDS pass did not start")
	}
	if got := <-resumer.sessions; got != "sid-old" {
		t.Fatalf("first session ID = %q, want sid-old", got)
	}

	coalescedID, err := eng.SyncUser(context.Background(), did, "sid-new", jobs.TriggerLogin)
	if err != nil {
		t.Fatalf("start second login sync: %v", err)
	}
	if coalescedID != id {
		t.Fatalf("coalesced job id = %q, want %q", coalescedID, id)
	}
	close(lister.firstContinue)

	select {
	case <-lister.secondStarted:
	case <-time.After(time.Second):
		t.Fatal("new session did not queue a later PDS pass")
	}
	select {
	case got := <-resumer.sessions:
		if got != "sid-new" {
			t.Fatalf("later pass session ID = %q, want sid-new", got)
		}
	case <-time.After(time.Second):
		t.Fatal("later pass did not resume a session")
	}
	close(lister.secondWait)

	job := waitForTerminalJob(t, tracker, id, did)
	if job.Status != jobs.StatusDone {
		t.Fatalf("job status = %q, want done", job.Status)
	}
}

func TestSyncUser_ShutdownWaitsForRun(t *testing.T) {
	store := newFakeStore()
	lister := &fakeLister{delay: 200 * time.Millisecond, subs: []PDSSubscription{
		{URI: "at://x/a/k1", Kind: "rss", Rkey: "k1", FeedURL: "https://example.com/feed"},
	}}
	fetcher := &countingFetcher{}
	tracker := jobs.New()
	eng := NewEngine(tracker, store, lister, fetcher, &nopResumer{}, nil)
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
	// Shutdown returning means the run goroutine already had its chance to update the tracker.
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
		{URI: "at://x/a/k1", Kind: "rss", Rkey: "k1", FeedURL: "https://example.com/feed"},
	}}
	tracker := jobs.New()
	fetcher := &countingFetcher{}
	eng := NewEngine(tracker, store, lister, fetcher, &nopResumer{}, nil)
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

// spyLocker records lock acquisitions and whether the lock is currently held.
type spyLocker struct {
	calls   atomic.Int32
	heldNow atomic.Bool
	lastKey string
}

func (l *spyLocker) LockSession(did syntax.DID, sid string) func() {
	l.calls.Add(1)
	l.lastKey = did.String() + "|" + sid
	l.heldNow.Store(true)
	return func() { l.heldNow.Store(false) }
}

// recordingResumer records whether the session lock is held at resume time, to prove resume and the eager refresh share one continuous lock hold.
type recordingResumer struct {
	locker       *spyLocker
	heldAtResume bool
	err          error
}

func (r *recordingResumer) ResumeSession(_ context.Context, did syntax.DID, sid string) (*oauth.ClientSession, error) {
	if r.locker != nil {
		r.heldAtResume = r.locker.heldNow.Load()
	}
	if r.err != nil {
		return nil, r.err
	}
	return &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: did, SessionID: sid}}, nil
}

// A resume outside the lock lets the request path rotate the refresh token before the engine refreshes, so the eager refresh would no-op on a stale token.
func TestEngine_ResumeAndRefresh_SharesOneLockHold(t *testing.T) {
	locker := &spyLocker{}
	res := &recordingResumer{locker: locker}
	eng := NewEngine(jobs.New(), newFakeStore(), &fakeLister{}, &countingFetcher{}, res, nil).WithLocker(locker)

	var refreshCalls int
	var heldAtRefresh bool
	eng.refreshSession = func(context.Context, *oauth.ClientSession) error {
		refreshCalls++
		heldAtRefresh = locker.heldNow.Load()
		return nil
	}

	sess, err := eng.resumeAndRefresh(context.Background(), mustDID("did:plc:alice"), "sid-1")
	if err != nil {
		t.Fatalf("resumeAndRefresh: %v", err)
	}
	if sess == nil {
		t.Fatal("nil session")
	}
	if !res.heldAtResume {
		t.Error("resume ran outside the session lock — the refresh race is open")
	}
	if refreshCalls != 1 || !heldAtRefresh {
		t.Errorf("refresh calls=%d heldAtRefresh=%v; want one refresh under the held lock", refreshCalls, heldAtRefresh)
	}
	if got := locker.calls.Load(); got != 1 {
		t.Errorf("LockSession calls = %d, want 1 continuous hold across resume+refresh", got)
	}
	if locker.heldNow.Load() {
		t.Error("lock not released after resumeAndRefresh")
	}
	if locker.lastKey != "did:plc:alice|sid-1" {
		t.Errorf("lock key = %q, want did:plc:alice|sid-1", locker.lastKey)
	}
}

func TestEngine_ResumeAndRefresh_NilLockerSkipsRefresh(t *testing.T) {
	res := &recordingResumer{}
	eng := NewEngine(jobs.New(), newFakeStore(), &fakeLister{}, &countingFetcher{}, res, nil)
	refreshed := false
	eng.refreshSession = func(context.Context, *oauth.ClientSession) error {
		refreshed = true
		return nil
	}
	if _, err := eng.resumeAndRefresh(context.Background(), mustDID("did:plc:alice"), "sid-1"); err != nil {
		t.Fatalf("resumeAndRefresh: %v", err)
	}
	if refreshed {
		t.Error("refresh should be skipped when no locker is installed")
	}
}

func TestEngine_ResumeAndRefresh_ResumeErrorPropagatesAndSkipsRefresh(t *testing.T) {
	locker := &spyLocker{}
	res := &recordingResumer{locker: locker, err: errors.New("session not found")}
	eng := NewEngine(jobs.New(), newFakeStore(), &fakeLister{}, &countingFetcher{}, res, nil).WithLocker(locker)
	refreshed := false
	eng.refreshSession = func(context.Context, *oauth.ClientSession) error {
		refreshed = true
		return nil
	}
	if _, err := eng.resumeAndRefresh(context.Background(), mustDID("did:plc:alice"), "sid-1"); err == nil {
		t.Fatal("resumeAndRefresh: want error, got nil")
	}
	if refreshed {
		t.Error("refresh must not run when resume fails")
	}
	if locker.heldNow.Load() {
		t.Error("lock not released on resume error")
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

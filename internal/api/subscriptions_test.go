package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
)

// --- Tier-1 index reader/writer test doubles ---

type fakeIndex struct {
	mu         sync.Mutex
	rows       map[string]map[string]db.UserSubscription // did → feedURL → row
	getFeedErr error
}

func newFakeIndex() *fakeIndex {
	return &fakeIndex{rows: map[string]map[string]db.UserSubscription{}}
}

func (f *fakeIndex) ListUserSubscriptions(_ context.Context, did string) ([]db.UserSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.UserSubscription, 0)
	for _, r := range f.rows[did] {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeIndex) GetUserSubscriptionByFeedURL(_ context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getFeedErr != nil {
		return db.UserSubscription{}, f.getFeedErr
	}
	byFeed, ok := f.rows[arg.Did]
	if !ok {
		return db.UserSubscription{}, sql.ErrNoRows
	}
	row, ok := byFeed[arg.FeedUrl]
	if !ok {
		return db.UserSubscription{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeIndex) UpsertFeed(_ context.Context, _ db.UpsertFeedParams) error { return nil }

func (f *fakeIndex) UpsertUserSubscription(_ context.Context, arg db.UpsertUserSubscriptionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rows[arg.Did]; !ok {
		f.rows[arg.Did] = map[string]db.UserSubscription{}
	}
	f.rows[arg.Did][arg.FeedUrl] = db.UserSubscription{
		Did:         arg.Did,
		Rkey:        arg.Rkey,
		AtUri:       arg.AtUri,
		FeedUrl:     arg.FeedUrl,
		Title:       arg.Title,
		CustomTitle: arg.CustomTitle,
		CreatedAt:   arg.CreatedAt,
		UpdatedAt:   arg.UpdatedAt,
	}
	return nil
}

// --- Finder + PDS writer + fetch dispatcher doubles ---

type fakeFinder struct {
	candidates []feedfinder.Candidate
	err        error
}

func (f *fakeFinder) Resolve(_ context.Context, _ string) ([]feedfinder.Candidate, error) {
	return f.candidates, f.err
}

type fakePDS struct {
	mu      sync.Mutex
	creates int
	lastRec map[string]any
}

func (p *fakePDS) CreateRecord(_ context.Context, sess *oauth.ClientSession, _ syntax.NSID, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	p.lastRec = record
	rkey := "3la" + itoa(p.creates)
	return &atprepo.RecordRef{
		URI: "at://" + sess.Data.AccountDID.String() + "/app.skyreader.feed.subscription/" + rkey,
		CID: "bafyreiabc",
	}, nil
}

func (p *fakePDS) PutRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, rkey string, _ map[string]any) (*atprepo.RecordRef, error) {
	return &atprepo.RecordRef{URI: "at://x/c/" + rkey, CID: "bafy"}, nil
}

func (p *fakePDS) DeleteRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ string) error {
	return nil
}

type fakeDispatcher struct {
	mu         sync.Mutex
	dispatched []string
	manualSync int
	next       int
}

func (d *fakeDispatcher) StartFetchOneFeed(_ syntax.DID, feedURL string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next++
	id := "job-" + itoa(d.next)
	d.dispatched = append(d.dispatched, feedURL)
	return id
}

func (d *fakeDispatcher) StartManualRefresh(_ context.Context, _ syntax.DID, _ string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.manualSync++
	d.next++
	return "sync-" + itoa(d.next), nil
}

// --- List handler ---

func TestSubscriptionsList_FromIndex(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://example.test/feed.xml": {
			Did:     "did:plc:alice",
			Rkey:    "3la",
			AtUri:   "at://did:plc:alice/app.skyreader.feed.subscription/3la",
			FeedUrl: "https://example.test/feed.xml",
		},
	}
	h := SubscriptionsListHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://example.test/feed.xml" {
		t.Errorf("got = %+v", got)
	}
}

// --- Resolve handler ---

func TestSubscriptionsResolve_ReturnsCandidates(t *testing.T) {
	idx := newFakeIndex()
	finder := &fakeFinder{candidates: []feedfinder.Candidate{
		{FeedURL: "https://example.test/feed.xml", Title: "Example"},
	}}
	h := SubscriptionsResolveHandler(idx, finder)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/resolve",
		strings.NewReader(`{"url":"https://example.test"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got resolveResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates) != 1 {
		t.Errorf("candidates = %+v", got.Candidates)
	}
}

func TestSubscriptionsResolve_FlagsExisting(t *testing.T) {
	idx := newFakeIndex()
	feed := "https://example.test/feed.xml"
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		feed: {Did: "did:plc:alice", FeedUrl: feed, Title: ptrString("Saved")},
	}
	finder := &fakeFinder{candidates: []feedfinder.Candidate{
		{FeedURL: feed, Title: "Example"},
	}}
	h := SubscriptionsResolveHandler(idx, finder)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/resolve",
		strings.NewReader(`{"url":"https://example.test"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got resolveResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got.ExistingSubscriptions) != 1 {
		t.Errorf("existing = %+v", got.ExistingSubscriptions)
	}
}

func TestSubscriptionsResolve_Errors(t *testing.T) {
	t.Run("finder error", func(t *testing.T) {
		h := SubscriptionsResolveHandler(newFakeIndex(), &fakeFinder{err: errors.New("upstream down")})
		req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/resolve",
			strings.NewReader(`{"url":"https://example.test"}`)), "did:plc:alice", "sid-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", rr.Code)
		}
	})

	t.Run("empty url", func(t *testing.T) {
		h := SubscriptionsResolveHandler(newFakeIndex(), &fakeFinder{})
		req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/resolve",
			strings.NewReader(`{"url":"   "}`)), "did:plc:alice", "sid-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})
}

// --- Create handler ---

func TestSubscriptionsCreate_HappyPath_FullChoiceA(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example","siteUrl":"https://example.test"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %d", len(got.Records))
	}
	if len(got.JobIDs) != 1 {
		t.Errorf("jobs = %d", len(got.JobIDs))
	}
	if pds.creates != 1 {
		t.Errorf("PDS creates = %d", pds.creates)
	}
	if len(disp.dispatched) != 1 {
		t.Errorf("dispatcher = %v", disp.dispatched)
	}

	// Tier-1 index should now show the new row.
	rows, _ := idx.ListUserSubscriptions(context.Background(), "did:plc:alice")
	if len(rows) != 1 {
		t.Errorf("Tier-1 rows = %d", len(rows))
	}
}

func TestSubscriptionsCreate_DedupeGuard_Idempotent(t *testing.T) {
	idx := newFakeIndex()
	feed := "https://example.test/feed.xml"
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		feed: {
			Did:     "did:plc:alice",
			Rkey:    "3laOLD",
			AtUri:   "at://did:plc:alice/app.skyreader.feed.subscription/3laOLD",
			FeedUrl: feed,
			Title:   ptrString("Existing"),
		},
	}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"NewName"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("PDS create called on dedupe path: %d", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatcher should not fire on dedupe: %v", disp.dispatched)
	}
}

func TestSubscriptionsCreate_PDSFailure_502(t *testing.T) {
	idx := newFakeIndex()
	pds := &failingPDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestSubscriptionsCreate_Validation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "empty list", body: `{"subscriptions":[]}`, want: "no subscriptions submitted"},
		{name: "missing feed URL", body: `{"subscriptions":[{"title":"Example"}]}`, want: "subscriptions.0.feedUrl"},
		{name: "malformed JSON", body: `{"subscriptions":[`, want: "invalid json"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := SubscriptionsCreateHandler(newFakeIndex(), newFakeIndex(), &fakePDS{}, &fakeDispatcher{})
			req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(tt.body)), "did:plc:alice", "sid-1")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.want) {
				t.Errorf("body = %q, want substring %q", rr.Body.String(), tt.want)
			}
		})
	}
}

func TestSubscriptionsCreate_DedupeProbeError_500(t *testing.T) {
	idx := newFakeIndex()
	idx.getFeedErr = errors.New("database unavailable")
	h := SubscriptionsCreateHandler(idx, idx, &fakePDS{}, &fakeDispatcher{})

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// upsertErrIndex is a fakeIndex variant that errors from UpsertUserSubscription.
// Used to exercise the Tier-1-failure → sync_user dispatch recovery path.
type upsertErrIndex struct{ *fakeIndex }

func (e upsertErrIndex) UpsertUserSubscription(_ context.Context, _ db.UpsertUserSubscriptionParams) error {
	return errors.New("tier-1 down")
}

func TestSubscriptionsCreate_Tier1Failure_DispatchesSyncUser(t *testing.T) {
	idx := newFakeIndex()
	writer := upsertErrIndex{idx}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, writer, pds, disp)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (PDS succeeded, tier-1 fallback should still return success): body = %s", rr.Code, rr.Body.String())
	}
	if disp.manualSync != 1 {
		t.Errorf("manual sync dispatches = %d, want 1", disp.manualSync)
	}
}

type failingPDS struct{}

func (failingPDS) CreateRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("pds down")
}

func (failingPDS) PutRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ string, _ map[string]any) (*atprepo.RecordRef, error) {
	return nil, errors.New("pds down")
}

func (failingPDS) DeleteRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, _ string) error {
	return errors.New("pds down")
}

func ptrString(s string) *string { return &s }

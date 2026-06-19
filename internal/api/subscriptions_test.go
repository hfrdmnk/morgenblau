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
	"time"

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

func (f *fakeIndex) ListUserSourcesWithStats(_ context.Context, arg db.ListUserSourcesWithStatsParams) ([]db.ListUserSourcesWithStatsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.ListUserSourcesWithStatsRow, 0)
	for _, r := range f.rows[arg.Did] {
		out = append(out, db.ListUserSourcesWithStatsRow{
			Did:       r.Did,
			Rkey:      r.Rkey,
			AtUri:     r.AtUri,
			FeedUrl:   r.FeedUrl,
			Title:     r.Title,
			IsPrimary: r.IsPrimary,
			Tags:      r.Tags,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

func (f *fakeIndex) ListUserSubscriptionTags(_ context.Context, did string) ([]*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*string, 0)
	for _, r := range f.rows[did] {
		if r.Tags != nil && *r.Tags != "" {
			out = append(out, r.Tags)
		}
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
		Did:       arg.Did,
		Rkey:      arg.Rkey,
		AtUri:     arg.AtUri,
		FeedUrl:   arg.FeedUrl,
		Title:     arg.Title,
		IsPrimary: arg.IsPrimary,
		Tags:      arg.Tags,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
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
	puts    int
	lastRec map[string]any
	lastPut map[string]any
}

func (p *fakePDS) CreateRecord(_ context.Context, sess *oauth.ClientSession, _ syntax.NSID, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	p.lastRec = record
	rkey := "3la" + itoa(p.creates)
	return &atprepo.RecordRef{
		URI: "at://" + sess.Data.AccountDID.String() + "/blue.morgen.feed.subscription/" + rkey,
		CID: "bafyreiabc",
	}, nil
}

func (p *fakePDS) PutRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, rkey string, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.puts++
	p.lastPut = record
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

// --- Frequency bucket ---

func TestFrequencyBucket(t *testing.T) {
	now := mustParseTime(t, "2026-05-21T12:00:00Z")
	long := now.AddDate(-1, 0, 0).Format(time.RFC3339) // first post a year ago — "New" doesn't apply
	young := now.AddDate(0, 0, -10).Format(time.RFC3339)

	cases := []struct {
		name              string
		first             string
		c7, c28, c56, c84 int64
		want              string
	}{
		{"no posts at all", "", 0, 0, 0, 0, "noPosts"},
		{"cadence wins over recency", young, 99, 99, 99, 99, "daily"},
		{"new is fallback when no cadence fires", young, 0, 0, 0, 0, "new"},
		{"new is fallback when below all thresholds", young, 0, 0, 0, 1, "new"},
		{"daily ≥5/7d", long, 5, 5, 5, 5, "daily"},
		{"weekly ≥3/28d", long, 0, 3, 3, 3, "weekly"},
		{"biweekly ≥3/56d", long, 0, 0, 3, 3, "biweekly"},
		{"monthly ≥2/84d", long, 0, 0, 0, 2, "monthly"},
		{"irregular below every threshold", long, 0, 0, 0, 1, "irregular"},
		{"highest-cadence bucket wins", long, 5, 1, 1, 1, "daily"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := frequencyBucket(c.first, c.c7, c.c28, c.c56, c.c84, now)
			if got != c.want {
				t.Errorf("frequencyBucket = %q, want %q", got, c.want)
			}
		})
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// --- List handler ---

func TestSubscriptionsList_FromIndex(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://example.test/feed.xml": {
			Did:     "did:plc:alice",
			Rkey:    "3la",
			AtUri:   "at://did:plc:alice/blue.morgen.feed.subscription/3la",
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
			AtUri:   "at://did:plc:alice/blue.morgen.feed.subscription/3laOLD",
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

// --- primary + tags ---

func TestSubscriptionsCreate_PrimaryAndTags_PersistedEverywhere(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example","primary":true,"tags":["News","Tech"]}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	// PDS record map carries primary + tags.
	if pds.lastRec["primary"] != true {
		t.Errorf("PDS record primary = %v, want true", pds.lastRec["primary"])
	}
	if tags, ok := pds.lastRec["tags"].([]string); !ok || len(tags) != 2 || tags[0] != "News" || tags[1] != "Tech" {
		t.Errorf("PDS record tags = %v (%T), want [News Tech]", pds.lastRec["tags"], pds.lastRec["tags"])
	}

	// Tier-1 upsert persisted them (round-trip via the stored row).
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did:     "did:plc:alice",
		FeedUrl: "https://example.test/feed.xml",
	})
	if err != nil {
		t.Fatalf("GetUserSubscriptionByFeedURL: %v", err)
	}
	if row.IsPrimary != 1 {
		t.Errorf("stored is_primary = %d, want 1", row.IsPrimary)
	}
	if row.Tags == nil || *row.Tags != `["News","Tech"]` {
		t.Errorf("stored tags = %v, want JSON [News Tech]", row.Tags)
	}

	// Response wire echoes them.
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %d", len(got.Records))
	}
	rec := got.Records[0]
	if !rec.Primary {
		t.Errorf("response primary = %v, want true", rec.Primary)
	}
	if len(rec.Tags) != 2 || rec.Tags[0] != "News" || rec.Tags[1] != "Tech" {
		t.Errorf("response tags = %v", rec.Tags)
	}
	if rec.Value["primary"] != true {
		t.Errorf("response value.primary = %v", rec.Value["primary"])
	}
}

func TestSubscriptionsCreate_DefaultsOmitPrimaryAndTags(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	h := SubscriptionsCreateHandler(idx, idx, pds, &fakeDispatcher{})

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if _, ok := pds.lastRec["primary"]; ok {
		t.Errorf("PDS record should omit primary when false: %v", pds.lastRec["primary"])
	}
	if _, ok := pds.lastRec["tags"]; ok {
		t.Errorf("PDS record should omit tags when empty: %v", pds.lastRec["tags"])
	}

	var got addResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	rec := got.Records[0]
	if rec.Primary {
		t.Errorf("response primary = true, want false")
	}
	// tags is `omitempty` so it should be absent from JSON entirely.
	if strings.Contains(rr.Body.String(), `"tags"`) {
		t.Errorf("response should omit tags key when empty: %s", rr.Body.String())
	}
}

func TestSubscriptionsCreate_TagNormalization(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	h := SubscriptionsCreateHandler(idx, idx, pds, &fakeDispatcher{})

	long := strings.Repeat("x", 65) // 65 runes > 64, must be dropped
	tags := []string{
		" News ", // trimmed → "News"
		"news",   // case-dup of News → dropped, first-seen casing kept
		"",       // blank → dropped
		"  ",     // blank → dropped
		"Tech",
		"TECH",                                 // case-dup → dropped
		long,                                   // over 64 graphemes → dropped
		"a", "b", "c", "d", "e", "f", "g", "h", // pushes well past 10 total
	}
	jsonTags, _ := json.Marshal(tags)
	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","tags":` + string(jsonTags) + `}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got addResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	out := got.Records[0].Tags

	if len(out) > 10 {
		t.Errorf("tags not capped at 10: %v", out)
	}
	if out[0] != "News" || out[1] != "Tech" {
		t.Errorf("expected first-seen casing News, Tech; got %v", out)
	}
	for _, tag := range out {
		if tag == "" {
			t.Errorf("blank tag survived: %v", out)
		}
		if len([]rune(tag)) > 64 {
			t.Errorf("over-64-grapheme tag survived: %q", tag)
		}
	}
	// "news"/"TECH" must not appear (case-dedupe).
	seen := map[string]int{}
	for _, tag := range out {
		seen[strings.ToLower(tag)]++
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("case-duplicate %q appears %d times: %v", k, n, out)
		}
	}
}

func TestSubscriptionsList_RoundTripsPrimaryAndTags(t *testing.T) {
	idx := newFakeIndex()
	tags := `["News","Tech"]`
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://example.test/feed.xml": {
			Did:       "did:plc:alice",
			Rkey:      "3la",
			AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
			FeedUrl:   "https://example.test/feed.xml",
			IsPrimary: 1,
			Tags:      &tags,
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
	if len(got) != 1 {
		t.Fatalf("rows = %d", len(got))
	}
	if !got[0].Primary {
		t.Errorf("primary = false, want true")
	}
	if len(got[0].Tags) != 2 || got[0].Tags[0] != "News" {
		t.Errorf("tags = %v", got[0].Tags)
	}
}

func TestSubscriptionsTags_DistinctSortedUnion(t *testing.T) {
	idx := newFakeIndex()
	aliceA := `["News","Tech"]`
	aliceB := `["news","Design","apple"]` // "news" dups News (case), keep first-seen "News"
	bobTags := `["BobOnly"]`
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		"https://a.test/feed.xml": {Did: "did:plc:alice", Rkey: "1", FeedUrl: "https://a.test/feed.xml", Tags: &aliceA},
		"https://b.test/feed.xml": {Did: "did:plc:alice", Rkey: "2", FeedUrl: "https://b.test/feed.xml", Tags: &aliceB},
	}
	idx.rows["did:plc:bob"] = map[string]db.UserSubscription{
		"https://c.test/feed.xml": {Did: "did:plc:bob", Rkey: "3", FeedUrl: "https://c.test/feed.xml", Tags: &bobTags},
	}

	h := SubscriptionsTagsHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/tags", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	// Case-insensitive sort ascending, first-seen casing, bob excluded.
	want := []string{"apple", "Design", "News", "Tech"}
	if len(got.Tags) != len(want) {
		t.Fatalf("tags = %v, want %v", got.Tags, want)
	}
	for i := range want {
		if got.Tags[i] != want[i] {
			t.Errorf("tags[%d] = %q, want %q (full: %v)", i, got.Tags[i], want[i], got.Tags)
		}
	}
}

// Guards the route-precedence concern: Go's ServeMux must treat the literal
// /api/subscriptions/tags as more specific than /api/subscriptions/{rkey} so
// the tags endpoint isn't swallowed by the wildcard. Mirrors routes.go order.
func TestSubscriptionsTags_RouteWinsOverRkeyWildcard(t *testing.T) {
	idx := newFakeIndex()
	fakeDetail := newFakeSourceDetail()
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/tags", SubscriptionsTagsHandler(idx))
	mux.Handle("GET /api/subscriptions/{rkey}", SubscriptionGetHandler(fakeDetail))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/tags", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	// The tags handler emits {"tags":...}; the rkey handler would 404 (no row)
	// or emit a detail wire. Assert we got the tags shape.
	if body := strings.TrimSpace(rr.Body.String()); body != `{"tags":[]}` {
		t.Errorf("wildcard route swallowed /tags: body = %q", body)
	}
}

func TestSubscriptionsTags_EmptyIsArrayNotNull(t *testing.T) {
	idx := newFakeIndex()
	h := SubscriptionsTagsHandler(idx)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/tags", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != `{"tags":[]}` {
		t.Errorf("body = %q, want {\"tags\":[]}", body)
	}
}

func TestNormalizeTags(t *testing.T) {
	long := strings.Repeat("é", 65) // 65 graphemes/runes, must drop
	in := []string{" a ", "A", "", "  ", "b", "B", long, "c"}
	got := normalizeTags(in)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("normalizeTags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Cap at 10.
	many := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		many = append(many, string(rune('a'+i)))
	}
	if n := len(normalizeTags(many)); n != 10 {
		t.Errorf("cap: len = %d, want 10", n)
	}

	// Empty in → nil/empty out.
	if got := normalizeTags(nil); len(got) != 0 {
		t.Errorf("nil input → %v", got)
	}
}

func TestMarshalUnmarshalTags(t *testing.T) {
	if marshalTags(nil) != nil {
		t.Errorf("marshalTags(nil) should be nil")
	}
	if marshalTags([]string{}) != nil {
		t.Errorf("marshalTags(empty) should be nil")
	}
	p := marshalTags([]string{"a", "b"})
	if p == nil || *p != `["a","b"]` {
		t.Errorf("marshalTags = %v", p)
	}
	if got := unmarshalTags(p); len(got) != 2 || got[0] != "a" {
		t.Errorf("unmarshalTags round-trip = %v", got)
	}
	if got := unmarshalTags(nil); len(got) != 0 {
		t.Errorf("unmarshalTags(nil) = %v", got)
	}
	bad := "not json"
	if got := unmarshalTags(&bad); len(got) != 0 {
		t.Errorf("unmarshalTags(bad) = %v", got)
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

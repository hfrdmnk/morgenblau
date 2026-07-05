package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
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
	mu            sync.Mutex
	rows          map[string]map[string]db.UserSubscription // did → feedURL → row
	getFeedErr    error
	upsertedFeeds []string              // feed URLs passed to UpsertFeed, in call order
	feedParams    []db.UpsertFeedParams // full UpsertFeed args, in call order
	catalogTitles map[string]*string    // feedURL → feeds.title for the stats join
	siteURLs      map[string]*string    // feedURL → feeds.site_url for the sibling join
}

func newFakeIndex() *fakeIndex {
	return &fakeIndex{
		rows:          map[string]map[string]db.UserSubscription{},
		catalogTitles: map[string]*string{},
		siteURLs:      map[string]*string{},
	}
}

func (f *fakeIndex) ListUserSubscriptionsWithSiteURL(_ context.Context, did string) ([]db.ListUserSubscriptionsWithSiteURLRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.ListUserSubscriptionsWithSiteURLRow, 0)
	for _, r := range f.rows[did] {
		out = append(out, db.ListUserSubscriptionsWithSiteURLRow{
			Rkey:         r.Rkey,
			FeedUrl:      r.FeedUrl,
			Kind:         r.Kind,
			Title:        r.Title,
			SiteUrl:      f.siteURLs[r.FeedUrl],
			CatalogTitle: f.catalogTitles[r.FeedUrl],
		})
	}
	return out, nil
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
			Did:          r.Did,
			Rkey:         r.Rkey,
			AtUri:        r.AtUri,
			FeedUrl:      r.FeedUrl,
			Kind:         r.Kind,
			SidecarRkey:  r.SidecarRkey,
			Title:        r.Title,
			IsPrimary:    r.IsPrimary,
			Tags:         r.Tags,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
			CatalogTitle: f.catalogTitles[r.FeedUrl],
		})
	}
	return out, nil
}

func (f *fakeIndex) ListUserSubscriptionTags(_ context.Context, did string) ([]*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirror the query's ORDER BY rkey so "first-seen casing wins" is
	// deterministic — ranging the map directly would randomize the order.
	rows := make([]db.UserSubscription, 0, len(f.rows[did]))
	for _, r := range f.rows[did] {
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Rkey < rows[j].Rkey })
	out := make([]*string, 0, len(rows))
	for _, r := range rows {
		if r.Tags != nil && *r.Tags != "" {
			out = append(out, r.Tags)
		}
	}
	return out, nil
}

func (f *fakeIndex) GetFeed(_ context.Context, feedURL string) (db.Feed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return db.Feed{FeedUrl: feedURL, SiteUrl: f.siteURLs[feedURL]}, nil
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

func (f *fakeIndex) UpsertFeed(_ context.Context, arg db.UpsertFeedParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertedFeeds = append(f.upsertedFeeds, arg.FeedUrl)
	f.feedParams = append(f.feedParams, arg)
	return nil
}

func (f *fakeIndex) UpsertUserSubscription(_ context.Context, arg db.UpsertUserSubscriptionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rows[arg.Did]; !ok {
		f.rows[arg.Did] = map[string]db.UserSubscription{}
	}
	// Mirror the query's NULLIF trick: empty/zero kind persists as rss.
	kind, _ := arg.Kind.(string)
	if kind == "" {
		kind = "rss"
	}
	f.rows[arg.Did][arg.FeedUrl] = db.UserSubscription{
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

// pdsWrite captures one CreateRecord call: which collection got which record.
type pdsWrite struct {
	collection string
	record     map[string]any
}

type fakePDS struct {
	mu          sync.Mutex
	creates     int
	puts        int
	lastRec     map[string]any
	lastPut     map[string]any
	lastPutRkey string
	created     []pdsWrite
	deleted     []string                            // "collection/rkey", in call order
	listed      map[string][]atprepo.ListedRecord   // canned ListRecords result per collection
	listErr     error
	listCalls   int
	createErr   map[string]error // per-collection CreateRecord failure
}

func (p *fakePDS) CreateRecord(_ context.Context, sess *oauth.ClientSession, collection syntax.NSID, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.createErr[collection.String()]; err != nil {
		return nil, err
	}
	p.creates++
	p.lastRec = record
	p.created = append(p.created, pdsWrite{collection: collection.String(), record: record})
	rkey := "3la" + itoa(p.creates)
	return &atprepo.RecordRef{
		URI: "at://" + sess.Data.AccountDID.String() + "/" + collection.String() + "/" + rkey,
		CID: "bafyreiabc",
	}, nil
}

func (p *fakePDS) PutRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, rkey string, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.puts++
	p.lastPut = record
	p.lastPutRkey = rkey
	return &atprepo.RecordRef{URI: "at://x/c/" + rkey, CID: "bafy"}, nil
}

func (p *fakePDS) DeleteRecord(_ context.Context, _ *oauth.ClientSession, collection syntax.NSID, rkey string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, collection.String()+"/"+rkey)
	return nil
}

func (p *fakePDS) ListRecords(_ context.Context, _ *oauth.ClientSession, collection syntax.NSID) ([]atprepo.ListedRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listCalls++
	if p.listErr != nil {
		return nil, p.listErr
	}
	return p.listed[collection.String()], nil
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

func TestSubscriptionsResolve_SiblingMatrix(t *testing.T) {
	const site = "https://blog.example.test"
	const rssFeed = "https://blog.example.test/feed.xml"

	cases := []struct {
		name      string
		sub       db.UserSubscription
		subSite   string
		subCat    *string
		candidate feedfinder.Candidate
		wantVia   *subscribedVia
	}{
		{
			name:      "rss sub flags publication candidate",
			sub:       db.UserSubscription{Did: "did:plc:alice", Rkey: "1", FeedUrl: rssFeed, Kind: "rss", Title: ptrString("My Blog")},
			subSite:   site,
			candidate: feedfinder.Candidate{Kind: "standardfeed", Publication: testPublication, SiteURL: site},
			wantVia:   &subscribedVia{Kind: "rss", Title: "My Blog"},
		},
		{
			name:      "standardfeed sub flags rss candidate with catalog title",
			sub:       db.UserSubscription{Did: "did:plc:alice", Rkey: "1", FeedUrl: testPublication, Kind: "standardfeed"},
			subSite:   site,
			subCat:    ptrString("Pub Name"),
			candidate: feedfinder.Candidate{FeedURL: rssFeed, SiteURL: site},
			wantVia:   &subscribedVia{Kind: "standardfeed", Title: "Pub Name"},
		},
		{
			name:      "same kind same site not flagged",
			sub:       db.UserSubscription{Did: "did:plc:alice", Rkey: "1", FeedUrl: rssFeed, Kind: "rss"},
			subSite:   site,
			candidate: feedfinder.Candidate{FeedURL: "https://blog.example.test/comments.atom", SiteURL: site},
			wantVia:   nil,
		},
		{
			name:      "shared host different path not siblings",
			sub:       db.UserSubscription{Did: "did:plc:alice", Rkey: "1", FeedUrl: testPublication, Kind: "standardfeed"},
			subSite:   "https://leaflet.pub/one",
			candidate: feedfinder.Candidate{FeedURL: "https://leaflet.pub/two/feed.xml", SiteURL: "https://leaflet.pub/two"},
			wantVia:   nil,
		},
		{
			name:      "www and trailing slash normalize equal",
			sub:       db.UserSubscription{Did: "did:plc:alice", Rkey: "1", FeedUrl: rssFeed, Kind: "rss", Title: ptrString("My Blog")},
			subSite:   "https://www.blog.example.test/",
			candidate: feedfinder.Candidate{Kind: "standardfeed", Publication: testPublication, SiteURL: site},
			wantVia:   &subscribedVia{Kind: "rss", Title: "My Blog"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			idx := newFakeIndex()
			idx.rows["did:plc:alice"] = map[string]db.UserSubscription{tt.sub.FeedUrl: tt.sub}
			idx.siteURLs[tt.sub.FeedUrl] = ptrString(tt.subSite)
			if tt.subCat != nil {
				idx.catalogTitles[tt.sub.FeedUrl] = tt.subCat
			}
			finder := &fakeFinder{candidates: []feedfinder.Candidate{tt.candidate}}
			h := SubscriptionsResolveHandler(idx, finder)
			req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/resolve",
				strings.NewReader(`{"url":"https://blog.example.test"}`)), "did:plc:alice", "sid-1")
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
				t.Fatalf("candidates = %+v", got.Candidates)
			}
			via := got.Candidates[0].SubscribedVia
			if tt.wantVia == nil {
				if via != nil {
					t.Errorf("subscribedVia = %+v, want nil", via)
				}
				return
			}
			if via == nil || via.Kind != tt.wantVia.Kind || via.Title != tt.wantVia.Title {
				t.Errorf("subscribedVia = %+v, want %+v", via, tt.wantVia)
			}
		})
	}
}

func TestSubscriptionsResolve_ExistingProbeUsesPublication(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		testPublication: {Did: "did:plc:alice", Rkey: "3std", FeedUrl: testPublication, Kind: "standardfeed"},
	}
	finder := &fakeFinder{candidates: []feedfinder.Candidate{
		{Kind: "standardfeed", Publication: testPublication, SiteURL: "https://blog.example.test"},
	}}
	h := SubscriptionsResolveHandler(idx, finder)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/resolve",
		strings.NewReader(`{"url":"https://blog.example.test"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got resolveResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ExistingSubscriptions) != 1 || got.ExistingSubscriptions[0].FeedURL != testPublication {
		t.Errorf("existing = %+v, want probe keyed on publication", got.ExistingSubscriptions)
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
		{name: "both feedUrl and publication", body: `{"subscriptions":[{"feedUrl":"https://x/feed.xml","publication":"at://did:plc:p/site.standard.publication/3p"}]}`, want: "subscriptions.0.publication"},
		{name: "publication not an at-uri", body: `{"subscriptions":[{"publication":"https://not-an-at-uri.example"}]}`, want: "subscriptions.0.publication"},
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

// --- standardfeed create ---

const testPublication = "at://did:plc:publisher/site.standard.publication/3pub"
const standardSubCollection = "site.standard.graph.subscription"

func TestSubscriptionsCreate_Standardfeed_DefaultsCreateOnlyStandardRecord(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"publication":"` + testPublication + `","siteUrl":"https://blog.example"}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	// Exactly ONE PDS write: the portable standard record. No sidecar when
	// the picker didn't customize anything.
	if pds.creates != 1 {
		t.Fatalf("PDS creates = %d, want 1 (got %+v)", pds.creates, pds.created)
	}
	if pds.created[0].collection != standardSubCollection {
		t.Errorf("collection = %q, want %s", pds.created[0].collection, standardSubCollection)
	}
	rec := pds.created[0].record
	if rec["publication"] != testPublication {
		t.Errorf("record.publication = %v", rec["publication"])
	}
	if _, ok := rec["createdAt"].(string); !ok {
		t.Errorf("record.createdAt missing: %v", rec)
	}
	if _, ok := rec["source"]; ok {
		t.Errorf("standard record must not carry a blue.morgen source union: %v", rec)
	}

	// Tier-2 catalog row keyed by the publication at-uri, kind standardfeed.
	if len(idx.feedParams) != 1 || idx.feedParams[0].FeedUrl != testPublication {
		t.Fatalf("UpsertFeed params = %+v", idx.feedParams)
	}
	if kind, _ := idx.feedParams[0].Kind.(string); kind != "standardfeed" {
		t.Errorf("UpsertFeed kind = %v", idx.feedParams[0].Kind)
	}
	if idx.feedParams[0].SiteUrl == nil || *idx.feedParams[0].SiteUrl != "https://blog.example" {
		t.Errorf("UpsertFeed siteUrl = %v", idx.feedParams[0].SiteUrl)
	}

	// Tier-1 row: existence rkey is the STANDARD record's rkey, no sidecar.
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did: "did:plc:alice", FeedUrl: testPublication,
	})
	if err != nil {
		t.Fatalf("row lookup: %v", err)
	}
	if row.Kind != "standardfeed" {
		t.Errorf("row kind = %q", row.Kind)
	}
	if row.Rkey != "3la1" {
		t.Errorf("row rkey = %q, want the standard record rkey 3la1", row.Rkey)
	}
	if row.SidecarRkey != nil {
		t.Errorf("row sidecar_rkey = %v, want nil", *row.SidecarRkey)
	}

	// Fetch dispatched for the publication key.
	if len(disp.dispatched) != 1 || disp.dispatched[0] != testPublication {
		t.Errorf("dispatched = %v", disp.dispatched)
	}

	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %d", len(got.Records))
	}
	wire := got.Records[0]
	if wire.Kind != "standardfeed" || wire.Publication != testPublication || wire.FeedURL != testPublication {
		t.Errorf("wire = %+v", wire)
	}
	if wire.URI != "at://did:plc:alice/"+standardSubCollection+"/3la1" {
		t.Errorf("wire uri = %q", wire.URI)
	}
}

func TestSubscriptionsCreate_Standardfeed_CustomMetadata_SidecarSecond(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	h := SubscriptionsCreateHandler(idx, idx, pds, &fakeDispatcher{})

	body := `{"subscriptions":[{"publication":"` + testPublication + `","title":"My Name","primary":true,"tags":["News"]}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 2 {
		t.Fatalf("PDS creates = %d, want 2 (standard + sidecar)", pds.creates)
	}
	// Order pins the failure contract: the portable record lands first so a
	// sidecar failure still leaves an adoptable subscription on the PDS.
	if pds.created[0].collection != standardSubCollection {
		t.Errorf("first write collection = %q, want standard record first", pds.created[0].collection)
	}
	if pds.created[1].collection != "blue.morgen.feed.subscription" {
		t.Errorf("second write collection = %q, want blue.morgen sidecar", pds.created[1].collection)
	}
	sidecar := pds.created[1].record
	source, ok := sidecar["source"].(map[string]any)
	if !ok || source["$type"] != "blue.morgen.feed.subscription#standardPublication" || source["publication"] != testPublication {
		t.Errorf("sidecar source = %v", sidecar["source"])
	}
	if sidecar["title"] != "My Name" || sidecar["primary"] != true {
		t.Errorf("sidecar metadata = %v", sidecar)
	}

	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did: "did:plc:alice", FeedUrl: testPublication,
	})
	if err != nil {
		t.Fatalf("row lookup: %v", err)
	}
	if row.Rkey != "3la1" {
		t.Errorf("row rkey = %q, want standard rkey 3la1", row.Rkey)
	}
	if row.SidecarRkey == nil || *row.SidecarRkey != "3la2" {
		t.Errorf("row sidecar_rkey = %v, want 3la2", row.SidecarRkey)
	}
	if row.Title == nil || *row.Title != "My Name" {
		t.Errorf("row title = %v", row.Title)
	}
}

func TestSubscriptionsCreate_Standardfeed_StaleScope_403(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	// withSession carries NO scopes — a pre-change grant.
	body := `{"subscriptions":[{"publication":"` + testPublication + `"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "reauth_required") {
		t.Errorf("body = %q, want reauth_required code", rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0 on stale scope", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatched = %v, want none", disp.dispatched)
	}
}

func TestSubscriptionsCreate_Standardfeed_Dedupe_Idempotent(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		testPublication: {
			Did:     "did:plc:alice",
			Rkey:    "3laOLD",
			AtUri:   "at://did:plc:alice/" + standardSubCollection + "/3laOLD",
			FeedUrl: testPublication,
			Kind:    "standardfeed",
		},
	}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"publication":"` + testPublication + `"}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates on dedupe path: %d", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatch on dedupe path: %v", disp.dispatched)
	}
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 1 || got.Records[0].Kind != "standardfeed" || got.Records[0].Publication != testPublication {
		t.Errorf("records = %+v", got.Records)
	}
}

func TestSubscriptionsCreate_MixedBatch_BothKinds(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"RSS"},{"publication":"` + testPublication + `"}]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 2 {
		t.Fatalf("PDS creates = %d, want 2", pds.creates)
	}
	if pds.created[0].collection != "blue.morgen.feed.subscription" || pds.created[1].collection != standardSubCollection {
		t.Errorf("collections = %q, %q", pds.created[0].collection, pds.created[1].collection)
	}
	var got addResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 2 || got.Records[0].Kind != "rss" || got.Records[1].Kind != "standardfeed" {
		t.Errorf("records = %+v", got.Records)
	}
	if len(disp.dispatched) != 2 {
		t.Errorf("dispatched = %v", disp.dispatched)
	}
}

func TestSubscriptionsCreate_SiblingPairInBatch_409(t *testing.T) {
	idx := newFakeIndex()
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	h := SubscriptionsCreateHandler(idx, idx, pds, disp)

	// An rss feed and a publication for the SAME site in one batch.
	body := `{"subscriptions":[
		{"feedUrl":"https://blog.example.test/feed.xml","siteUrl":"https://blog.example.test"},
		{"publication":"` + testPublication + `","siteUrl":"https://blog.example.test"}
	]}`
	req := withStandardWriteSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0", pds.creates)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatched = %v", disp.dispatched)
	}
}

func TestSubscriptionsList_Standardfeed_CatalogTitleFallback(t *testing.T) {
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		testPublication: {
			Did:     "did:plc:alice",
			Rkey:    "3std",
			AtUri:   "at://did:plc:alice/" + standardSubCollection + "/3std",
			FeedUrl: testPublication,
			Kind:    "standardfeed",
			// No user title: the wire falls back to the cached catalog title.
		},
	}
	idx.catalogTitles[testPublication] = ptrString("Publication Name")

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
	if got[0].Kind != "standardfeed" || got[0].Publication != testPublication {
		t.Errorf("wire kind/publication = %q/%q", got[0].Kind, got[0].Publication)
	}
	if got[0].Title != "Publication Name" {
		t.Errorf("wire title = %q, want catalog fallback", got[0].Title)
	}
	// The value map mirrors the PDS record — no user title there.
	if _, ok := got[0].Value["title"]; ok {
		t.Errorf("value.title should stay absent without a user title: %v", got[0].Value)
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
	// Rev-2 shape: identity lives in the required source union, not flat.
	recSource, ok := pds.lastRec["source"].(map[string]any)
	if !ok || recSource["$type"] != "blue.morgen.feed.subscription#rssFeed" || recSource["feedUrl"] != "https://example.test/feed.xml" {
		t.Errorf("PDS record source = %v, want rssFeed variant", pds.lastRec["source"])
	}
	if _, flat := pds.lastRec["feedUrl"]; flat {
		t.Errorf("PDS record must not carry flat feedUrl: %v", pds.lastRec["feedUrl"])
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

func (failingPDS) ListRecords(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID) ([]atprepo.ListedRecord, error) {
	return nil, errors.New("pds down")
}

func ptrString(s string) *string { return &s }

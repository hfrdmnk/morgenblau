package api

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"sync"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
)

// testPublication is the canonical standardfeed publication at-uri shared across resolve, create, and list tests.
const testPublication = "at://did:plc:publisher/site.standard.publication/3pub"

// --- Tier-1 index reader/writer test doubles ---

type fakeIndex struct {
	mu                  sync.Mutex
	rows                map[string]map[string]db.UserSubscription // did → feedURL → row
	getFeedErr          error
	upsertedFeeds       []string              // feed URLs passed to UpsertFeed, in call order
	feedParams          []db.UpsertFeedParams // full UpsertFeed args, in call order
	catalogTitles       map[string]*string    // feedURL → feeds.title for the stats join
	siteURLs            map[string]*string    // feedURL → feeds.site_url for the sibling join
	lastFetchedAt       map[string]*string    // feedURL → feeds.last_fetched_at for the stats join
	consecutiveFailures map[string]int64      // feedURL → feeds.consecutive_failures for the stats join
}

func newFakeIndex() *fakeIndex {
	return &fakeIndex{
		rows:                map[string]map[string]db.UserSubscription{},
		catalogTitles:       map[string]*string{},
		siteURLs:            map[string]*string{},
		lastFetchedAt:       map[string]*string{},
		consecutiveFailures: map[string]int64{},
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
			Did:                 r.Did,
			Rkey:                r.Rkey,
			AtUri:               r.AtUri,
			FeedUrl:             r.FeedUrl,
			Kind:                r.Kind,
			SidecarRkey:         r.SidecarRkey,
			Title:               r.Title,
			IsPrimary:           r.IsPrimary,
			Tags:                r.Tags,
			CreatedAt:           r.CreatedAt,
			UpdatedAt:           r.UpdatedAt,
			CatalogTitle:        f.catalogTitles[r.FeedUrl],
			LastFetchedAt:       f.lastFetchedAt[r.FeedUrl],
			ConsecutiveFailures: f.consecutiveFailures[r.FeedUrl],
		})
	}
	return out, nil
}

func (f *fakeIndex) ListUserSubscriptionTags(_ context.Context, did string) ([]*string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirror the query's ORDER BY rkey so "first-seen casing wins" is deterministic; ranging the map directly would randomize order.
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
	deleted     []string                          // "collection/rkey", in call order
	listed      map[string][]atprepo.ListedRecord // canned ListRecords result per collection
	listErr     error
	listCalls   int
	createErr   map[string]error // per-collection CreateRecord failure
	deleteErr   error            // DeleteRecord failure, applied to every call
	putErr      error            // PutRecord failure, applied to every call
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
	rkey := "3la" + strconv.Itoa(p.creates)
	return &atprepo.RecordRef{
		URI: "at://" + sess.Data.AccountDID.String() + "/" + collection.String() + "/" + rkey,
		CID: "bafyreiabc",
	}, nil
}

func (p *fakePDS) PutRecord(_ context.Context, _ *oauth.ClientSession, _ syntax.NSID, rkey string, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.putErr != nil {
		return nil, p.putErr
	}
	p.puts++
	p.lastPut = record
	p.lastPutRkey = rkey
	return &atprepo.RecordRef{URI: "at://x/c/" + rkey, CID: "bafy"}, nil
}

func (p *fakePDS) DeleteRecord(_ context.Context, _ *oauth.ClientSession, collection syntax.NSID, rkey string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.deleteErr != nil {
		return p.deleteErr
	}
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
	id := "job-" + strconv.Itoa(d.next)
	d.dispatched = append(d.dispatched, feedURL)
	return id
}

func (d *fakeDispatcher) StartManualRefresh(_ context.Context, _ syntax.DID, _ string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.manualSync++
	d.next++
	return "sync-" + strconv.Itoa(d.next), nil
}

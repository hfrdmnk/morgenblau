package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

type discoverSourceWires []DiscoverSourceWire

func (wires *discoverSourceWires) UnmarshalJSON(data []byte) error {
	var page discoverPageWire[DiscoverSourceWire]
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}
	*wires = page.Items
	return nil
}

func TestDiscoverSourcesHandler_PaginatesBalancedPoolsWithStableCursor(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	personal := make([]discovercrawl.Subscription, 6)
	trendingRows := make([]db.DiscoverTrendingSignal, 0, 18)
	for i := range 6 {
		personal[i] = discovercrawl.Subscription{
			Key:  fmt.Sprintf("https://personal-%d.example/feed", i),
			Kind: "rss",
		}
		trendingRows = append(trendingRows, threeRepoSignal(fmt.Sprintf("https://trending-%d.example/feed", i))...)
	}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:friend": personal,
	}}
	h := DiscoverSourcesHandler(
		follows,
		noAdjacentFollows(),
		noOwnForeignSubscriptions(),
		&fakeDiscoverSubsReader{},
		crawler,
		noAuthoredPublications(),
		noPersonalShares(),
		noEntryResolver(),
		newFakeDiscoverHiddenReader(),
		&fakeDiscoverTrendingSignalsReader{rows: trendingRows},
		noFeedLanguages(),
		noTitleBackfill(),
	)

	firstReq := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	firstRR := httptest.NewRecorder()
	h.ServeHTTP(firstRR, firstReq)

	if firstRR.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", firstRR.Code)
	}
	if got := firstRR.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var first discoverPageWire[DiscoverSourceWire]
	if err := json.Unmarshal(firstRR.Body.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first page: %v", err)
	}
	if len(first.Items) != 8 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want eight items and a cursor", first)
	}
	for i, item := range first.Items {
		if i < 4 && item.Reason.StrongCount == 0 {
			t.Errorf("item %d = %+v, want personal suggestion", i, item)
		}
		if i >= 4 && (!item.Reason.Trending || item.Reason.StrongCount != 0) {
			t.Errorf("item %d = %+v, want trending-only suggestion", i, item)
		}
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil)
	query := secondReq.URL.Query()
	query.Set("cursor", first.NextCursor)
	secondReq.URL.RawQuery = query.Encode()
	secondReq = withSession(secondReq, "did:plc:me", "sess1")
	secondRR := httptest.NewRecorder()
	h.ServeHTTP(secondRR, secondReq)

	var second discoverPageWire[DiscoverSourceWire]
	if err := json.Unmarshal(secondRR.Body.Bytes(), &second); err != nil {
		t.Fatalf("unmarshal second page: %v", err)
	}
	if len(second.Items) != 4 || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want four final items", second)
	}
	seen := map[string]struct{}{}
	for _, item := range append(first.Items, second.Items...) {
		if _, duplicate := seen[item.Key]; duplicate {
			t.Fatalf("duplicate source %q across pages", item.Key)
		}
		seen[item.Key] = struct{}{}
	}
}

func TestDiscoverSourcesHandler_RejectsPeopleCursor(t *testing.T) {
	cursor, err := encodeDiscoverCursor(discoverCursor{
		Version:  1,
		Kind:     "people",
		Seed:     1,
		RankedAt: time.Now().UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := newDiscoverSourcesHandler(
		&fakeDiscoverFollowsReader{},
		noAdjacentFollows(),
		noOwnForeignSubscriptions(),
		&fakeDiscoverSubsReader{},
		&fakeSubscriptionCrawler{},
		newFakeDiscoverHiddenReader(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/discover/sources?cursor="+cursor, nil)
	req = withSession(req, "did:plc:me", "sess1")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	var body errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != codeInvalidRequest {
		t.Errorf("code = %q, want %q", body.Code, codeInvalidRequest)
	}
}

type fakeDiscoverFollowsReader struct {
	rows []db.UserFollow
	err  error
}

func (f *fakeDiscoverFollowsReader) ListUserFollows(context.Context, string) ([]db.UserFollow, error) {
	return f.rows, f.err
}

type fakeAdjacentFollowCrawler struct {
	follows []discovercrawl.AdjacentFollow
	err     error
	calls   int
}

func (f *fakeAdjacentFollowCrawler) CrawlAdjacentFollows(context.Context, syntax.DID) ([]discovercrawl.AdjacentFollow, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.follows, nil
}

type fakeDiscoverSubsReader struct {
	rows []db.UserSubscription
	err  error
}

func (f *fakeDiscoverSubsReader) ListUserSubscriptions(context.Context, string) ([]db.UserSubscription, error) {
	return f.rows, f.err
}

type fakeDiscoverHiddenReader struct {
	hidden map[string][]string // did|kind -> hidden keys
	err    error
	calls  []db.ListActiveDiscoverHidesParams
}

func newFakeDiscoverHiddenReader() *fakeDiscoverHiddenReader {
	return &fakeDiscoverHiddenReader{hidden: map[string][]string{}}
}

func (f *fakeDiscoverHiddenReader) hide(did, kind, key string) {
	k := did + "|" + kind
	f.hidden[k] = append(f.hidden[k], key)
}

func (f *fakeDiscoverHiddenReader) ListActiveDiscoverHides(_ context.Context, arg db.ListActiveDiscoverHidesParams) ([]string, error) {
	f.calls = append(f.calls, arg)
	if f.err != nil {
		return nil, f.err
	}
	return f.hidden[arg.Did+"|"+arg.TargetKind], nil
}

type fakeSubscriptionCrawler struct {
	mu    sync.Mutex
	byDID map[string][]discovercrawl.Subscription
	err   map[string]error
	calls []string
}

// FetchSubscriptions runs concurrently under the handler's fan-out, so calls must be appended under lock.
func (f *fakeSubscriptionCrawler) FetchSubscriptions(_ context.Context, did syntax.DID) ([]discovercrawl.Subscription, error) {
	f.mu.Lock()
	f.calls = append(f.calls, did.String())
	f.mu.Unlock()
	if err, ok := f.err[did.String()]; ok {
		return nil, err
	}
	return f.byDID[did.String()], nil
}

type fakeAuthoredPublicationCrawler struct {
	mu    sync.Mutex
	byDID map[string][]discovercrawl.AuthoredPublication
	err   map[string]error
	calls []string
}

// FetchAuthoredPublications runs concurrently under the handler's fan-out, so calls must be appended under lock.
func (f *fakeAuthoredPublicationCrawler) FetchAuthoredPublications(_ context.Context, did syntax.DID) ([]discovercrawl.AuthoredPublication, error) {
	f.mu.Lock()
	f.calls = append(f.calls, did.String())
	f.mu.Unlock()
	if err, ok := f.err[did.String()]; ok {
		return nil, err
	}
	return f.byDID[did.String()], nil
}

func noAuthoredPublications() *fakeAuthoredPublicationCrawler {
	return &fakeAuthoredPublicationCrawler{byDID: map[string][]discovercrawl.AuthoredPublication{}}
}

type fakePersonalShareCrawler struct {
	mu    sync.Mutex
	byDID map[string][]discovercrawl.Share
	err   map[string]error
	calls []string
}

// FetchShares runs concurrently under the handler's fan-out, so calls must be appended under lock.
func (f *fakePersonalShareCrawler) FetchShares(_ context.Context, did syntax.DID) ([]discovercrawl.Share, error) {
	f.mu.Lock()
	f.calls = append(f.calls, did.String())
	f.mu.Unlock()
	if err, ok := f.err[did.String()]; ok {
		return nil, err
	}
	return f.byDID[did.String()], nil
}

func noPersonalShares() *fakePersonalShareCrawler {
	return &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{}}
}

type fakeDiscoverEntryResolver struct {
	byGuid    map[string]string
	byItemURL map[string]string
}

func noEntryResolver() *fakeDiscoverEntryResolver {
	return &fakeDiscoverEntryResolver{byGuid: map[string]string{}, byItemURL: map[string]string{}}
}

func (f *fakeDiscoverEntryResolver) GetFeedURLByGuid(_ context.Context, guid string) (string, error) {
	if fu, ok := f.byGuid[guid]; ok {
		return fu, nil
	}
	return "", sql.ErrNoRows
}

func (f *fakeDiscoverEntryResolver) GetFeedURLByItemURL(_ context.Context, url string) (string, error) {
	if fu, ok := f.byItemURL[url]; ok {
		return fu, nil
	}
	return "", sql.ErrNoRows
}

func discoverFollow(subjectDID string) db.UserFollow {
	return db.UserFollow{Did: "did:plc:me", SubjectDid: subjectDID}
}

func noAdjacentFollows() *fakeAdjacentFollowCrawler {
	return &fakeAdjacentFollowCrawler{}
}

type fakeOwnForeignSubscriptionCrawler struct {
	byDID map[string][]discovercrawl.ForeignSubscription
	err   map[string]error
	calls []string
}

func (f *fakeOwnForeignSubscriptionCrawler) CrawlOwnForeignSubscriptions(_ context.Context, did syntax.DID) ([]discovercrawl.ForeignSubscription, error) {
	f.calls = append(f.calls, did.String())
	if err, ok := f.err[did.String()]; ok {
		return nil, err
	}
	return f.byDID[did.String()], nil
}

func noOwnForeignSubscriptions() *fakeOwnForeignSubscriptionCrawler {
	return &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{}}
}

type fakeDiscoverTrendingSignalsReader struct {
	rows []db.DiscoverTrendingSignal
	err  error

	// gotMinDistinctRepos lets tests assert the handler threads discoverrank.MinDistinctRepos through rather than a duplicated literal.
	gotMinDistinctRepos int64
}

func (f *fakeDiscoverTrendingSignalsReader) ListDiscoverTrendingSignalsAboveBar(_ context.Context, minDistinctRepos int64) ([]db.DiscoverTrendingSignal, error) {
	f.gotMinDistinctRepos = minDistinctRepos
	return f.rows, f.err
}

func (f *fakeDiscoverTrendingSignalsReader) ListDiscoverTrendingSignalsForEligibleSubjects(_ context.Context, minDistinctRepos int64) ([]db.DiscoverTrendingSignal, error) {
	f.gotMinDistinctRepos = minDistinctRepos
	return f.rows, f.err
}

type fakeDiscoverFeedLanguageReader struct {
	rows []db.ListFeedLanguagesRow
	err  error
}

func (f *fakeDiscoverFeedLanguageReader) ListFeedLanguages(context.Context) ([]db.ListFeedLanguagesRow, error) {
	return f.rows, f.err
}

func noFeedLanguages() *fakeDiscoverFeedLanguageReader { return &fakeDiscoverFeedLanguageReader{} }

func feedLanguage(feedURL, language string) db.ListFeedLanguagesRow {
	return db.ListFeedLanguagesRow{FeedUrl: feedURL, Language: &language}
}

type fakeDiscoverSourceTitleBackfillReader struct {
	rows  map[string]db.GetDiscoverTrendingSignalTitleRow
	err   error
	calls []string
}

func (f *fakeDiscoverSourceTitleBackfillReader) GetDiscoverTrendingSignalTitle(_ context.Context, sourceKey string) (db.GetDiscoverTrendingSignalTitleRow, error) {
	f.calls = append(f.calls, sourceKey)
	if f.err != nil {
		return db.GetDiscoverTrendingSignalTitleRow{}, f.err
	}
	row, ok := f.rows[sourceKey]
	if !ok {
		return db.GetDiscoverTrendingSignalTitleRow{}, sql.ErrNoRows
	}
	return row, nil
}

// noTitleBackfill is the mechanical default for tests that don't exercise the backfill: every lookup misses (sql.ErrNoRows), same as an empty table.
func noTitleBackfill() *fakeDiscoverSourceTitleBackfillReader {
	return &fakeDiscoverSourceTitleBackfillReader{rows: map[string]db.GetDiscoverTrendingSignalTitleRow{}}
}

func titleBackfillRow(title, siteURL string) db.GetDiscoverTrendingSignalTitleRow {
	return db.GetDiscoverTrendingSignalTitleRow{Title: &title, SiteUrl: &siteURL}
}

func trendingSignal(repoDID, sourceKey, kind, signalKind string) db.DiscoverTrendingSignal {
	return db.DiscoverTrendingSignal{
		RepoDid:    repoDID,
		SourceKey:  sourceKey,
		Kind:       kind,
		SignalKind: signalKind,
		FetchedAt:  "2026-07-09T00:00:00Z",
	}
}

func threeRepoSignal(sourceKey string) []db.DiscoverTrendingSignal {
	return []db.DiscoverTrendingSignal{
		trendingSignal("did:plc:repo1", sourceKey, "rss", "subscribe"),
		trendingSignal("did:plc:repo2", sourceKey, "rss", "subscribe"),
		trendingSignal("did:plc:repo3", sourceKey, "rss", "subscribe"),
	}
}

// inFlightTracker records running and peak concurrent calls so a fan-out test can prove both overlap and a bound; reached fires once, the first time current hits target.
type inFlightTracker struct {
	mu      sync.Mutex
	current int
	max     int
	target  int
	reached chan struct{}
	once    sync.Once
}

func newInFlightTracker(target int) *inFlightTracker {
	return &inFlightTracker{target: target, reached: make(chan struct{})}
}

func (t *inFlightTracker) enter() {
	t.mu.Lock()
	t.current++
	if t.current > t.max {
		t.max = t.current
	}
	hitTarget := t.current == t.target
	t.mu.Unlock()
	if hitTarget {
		t.once.Do(func() { close(t.reached) })
	}
}

func (t *inFlightTracker) leave() {
	t.mu.Lock()
	t.current--
	t.mu.Unlock()
}

func (t *inFlightTracker) maxObserved() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.max
}

// gatedSubscriptionCrawler blocks every call until release closes, so a test can pin the fan-out bound's in-flight count before letting them all complete.
type gatedSubscriptionCrawler struct {
	tracker *inFlightTracker
	release chan struct{}
	byDID   map[string][]discovercrawl.Subscription
}

func (f *gatedSubscriptionCrawler) FetchSubscriptions(ctx context.Context, did syntax.DID) ([]discovercrawl.Subscription, error) {
	f.tracker.enter()
	defer f.tracker.leave()
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return f.byDID[did.String()], nil
}

// newDiscoverSourcesHandler defaults every non-subscription crawler to "nothing found".
func newDiscoverSourcesHandler(
	follows DiscoverFollowsReader,
	adjacent AdjacentFollowCrawler,
	ownForeign OwnForeignSubscriptionCrawler,
	subs DiscoverSubscriptionsReader,
	crawler SubscriptionCrawler,
	hides DiscoverHiddenReader,
) http.Handler {
	return DiscoverSourcesHandler(follows, adjacent, ownForeign, subs, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), hides, &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())
}

func TestDiscoverSourcesHandler_NoFollowsAtAll_ReturnsEmptyWithoutCrawlingSubscriptions(t *testing.T) {
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	adjacent := noAdjacentFollows()
	h := newDiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, adjacent, noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
	if len(crawler.calls) != 0 {
		t.Errorf("crawler.calls = %v, want none (cold start must not crawl)", crawler.calls)
	}
}

func TestDiscoverSourcesHandler_RanksByDistinctFollowerCountWithReason(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:alice"),
		discoverFollow("did:plc:bob"),
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://shared.example/feed", Kind: "rss", Title: "Shared"}},
		"did:plc:bob":   {{Key: "https://shared.example/feed", Kind: "rss", Title: "Shared"}},
	}}
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 suggestion", got)
	}
	if got[0].FeedURL != "https://shared.example/feed" || got[0].Kind != "rss" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].Reason.StrongCount != 2 {
		t.Errorf("Reason.StrongCount = %d, want 2", got[0].Reason.StrongCount)
	}
	if got[0].Reason.TopSignal != "subscribe" {
		t.Errorf("Reason.TopSignal = %q, want subscribe", got[0].Reason.TopSignal)
	}
}

func TestDiscoverSourcesHandler_ExcludesAlreadySubscribed(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://already.example/feed", Kind: "rss"}},
	}}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{{FeedUrl: "https://already.example/feed"}}}
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), subs, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want already-subscribed source excluded", got)
	}
}

// Tier-2 stores feed_url exactly as subscribed; the exclusion set must compare canonical forms (see internal/feedkey) or a URL variant leaks its own source back as a suggestion.
func TestDiscoverSourcesHandler_ExcludesAlreadySubscribed_URLVariant(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://already.example/feed", Kind: "rss"}},
	}}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{{FeedUrl: "https://ALREADY.example:443/feed/"}}}
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), subs, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want URL-variant subscription to exclude the suggestion", got)
	}
}

func TestDiscoverSourcesHandler_StandardfeedCandidateCarriesPublicationField(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: pubURI, Kind: "standardfeed", Title: "Zine"}},
	}}
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Publication != pubURI || got[0].FeedURL != "" {
		t.Fatalf("got = %+v", got)
	}
}

func TestDiscoverSourcesHandler_OneBrokenRepoDoesNotFailWholeRequest(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:broken"),
		discoverFollow("did:plc:alice"),
	}}
	crawler := &fakeSubscriptionCrawler{
		byDID: map[string][]discovercrawl.Subscription{
			"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
		},
		err: map[string]error{"did:plc:broken": errors.New("pds unreachable")},
	}
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite one broken repo", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://good.example/feed" {
		t.Fatalf("got = %+v, want the good repo's suggestion despite the broken one", got)
	}
}

func TestDiscoverSourcesHandler_CapsAtEight(t *testing.T) {
	var rows []db.UserFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < 10; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		rows = append(rows, discoverFollow(did))
		byDID[did] = []discovercrawl.Subscription{{Key: "https://feed" + string(rune('a'+i)) + ".example/f", Kind: "rss"}}
	}
	follows := &fakeDiscoverFollowsReader{rows: rows}
	crawler := &fakeSubscriptionCrawler{byDID: byDID}
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestDiscoverSourcesHandler_ExcludesHiddenSources(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://hidden.example/feed", Kind: "rss"}},
	}}
	hides := newFakeDiscoverHiddenReader()
	hides.hide("did:plc:me", "source", "https://hidden.example/feed")
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, hides)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want hidden source excluded", got)
	}
	if len(hides.calls) != 1 || hides.calls[0].Did != "did:plc:me" || hides.calls[0].TargetKind != "source" {
		t.Errorf("hides lookup = %+v, want one call scoped to the session did and 'source' kind", hides.calls)
	}
}

func TestDiscoverSourcesHandler_HiddenSourceForAnotherUserStillShows(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://shared.example/feed", Kind: "rss"}},
	}}
	hides := newFakeDiscoverHiddenReader()
	hides.hide("did:plc:someoneelse", "source", "https://shared.example/feed")
	h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, hides)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got = %+v, want another user's hide to have no effect", got)
	}
}

func TestDiscoverSourcesHandler_SameCursor_IdenticalOrder(t *testing.T) {
	var rows []db.UserFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < 6; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		rows = append(rows, discoverFollow(did))
		byDID[did] = []discovercrawl.Subscription{{Key: "https://feed" + string(rune('a'+i)) + ".example/f", Kind: "rss"}}
	}
	follows := &fakeDiscoverFollowsReader{rows: rows}
	cursor, err := encodeDiscoverCursor(discoverCursor{
		Version:  1,
		Kind:     "sources",
		Seed:     1234,
		RankedAt: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	run := func() []DiscoverSourceWire {
		crawler := &fakeSubscriptionCrawler{byDID: byDID}
		h := newDiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())
		req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
		query := req.URL.Query()
		query.Set("cursor", cursor)
		req.URL.RawQuery = query.Encode()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var got discoverSourceWires
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return got
	}

	first := run()
	second := run()
	if len(first) != len(second) {
		t.Fatalf("len(first) = %d, len(second) = %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Key != second[i].Key {
			t.Errorf("order differs at [%d]: %q != %q (same cursor must be stable)", i, first[i].Key, second[i].Key)
		}
	}
}

// --- Adjacent graphs / weak tier (SPEC <discovery> Trust tiers, Weak-tier cap) ---

func TestDiscoverSourcesHandler_BlueskyOnlyFollow_SurfacesSuggestionWithWeakReason(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:bsky-alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:bsky-alice": {{Key: "https://bsky-source.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, adjacent, noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://bsky-source.example/feed" {
		t.Fatalf("got = %+v, want the bluesky-only follow's source to surface", got)
	}
	if got[0].Reason.StrongCount != 0 || got[0].Reason.WeakCount != 1 {
		t.Errorf("Reason = %+v, want StrongCount=0 WeakCount=1", got[0].Reason)
	}
	if got[0].Reason.TopFollowerNetwork != "bluesky" {
		t.Errorf("Reason.TopFollowerNetwork = %q, want bluesky", got[0].Reason.TopFollowerNetwork)
	}
}

func TestDiscoverSourcesHandler_PersonInBothTiers_CountsOnceAtStrongTier(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://shared.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverSourcesHandler(follows, adjacent, noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 suggestion", got)
	}
	if got[0].Reason.StrongCount != 1 || got[0].Reason.WeakCount != 0 {
		t.Errorf("Reason = %+v, want StrongCount=1 WeakCount=0 (alice counted once, at the strong tier)", got[0].Reason)
	}
	// The crawler must be asked about alice once, not once per tier.
	if len(crawler.calls) != 1 {
		t.Errorf("crawler.calls = %v, want exactly one call for alice", crawler.calls)
	}
}

func TestDiscoverSourcesHandler_AdjacentCrawlFailureDegradesGracefully(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	adjacent := &fakeAdjacentFollowCrawler{err: errors.New("own pds unreachable")}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverSourcesHandler(follows, adjacent, noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the adjacent-graph crawl failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://good.example/feed" {
		t.Fatalf("got = %+v, want the strong-tier suggestion despite the adjacent-graph failure", got)
	}
}

func TestDiscoverSourcesHandler_WeakTierCap_ManyBlueskyFollowsCannotOutrankMultipleStrongFollows(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:s1"), discoverFollow("did:plc:s2"), discoverFollow("did:plc:s3"),
	}}
	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{
		"did:plc:s1": {{Key: "https://strong-favorite.example/feed", Kind: "rss"}},
		"did:plc:s2": {{Key: "https://strong-favorite.example/feed", Kind: "rss"}},
		"did:plc:s3": {{Key: "https://strong-favorite.example/feed", Kind: "rss"}},
	}
	for i := 0; i < 50; i++ {
		did := "did:plc:w" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{{Key: "https://viral-on-bluesky.example/feed", Kind: "rss"}}
	}
	adjacent := &fakeAdjacentFollowCrawler{follows: adjacentFollows}
	crawler := &fakeSubscriptionCrawler{byDID: byDID}
	h := newDiscoverSourcesHandler(follows, adjacent, noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) == 0 || got[0].FeedURL != "https://strong-favorite.example/feed" {
		t.Fatalf("got = %+v, want the multi-strong-follow source ranked first despite 50 bluesky endorsements on the other", got)
	}
}

// --- Full signal model: authors, shares, saves. SPEC <discovery> Signal ordering. ---

func TestDiscoverSourcesHandler_AuthoredPublicationRanksTopWithWritesThisReason(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:author"),
		discoverFollow("did:plc:subscriber"),
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:subscriber": {{Key: "https://ordinary.example/feed", Kind: "rss"}},
	}}
	pubURI := "at://did:plc:author/site.standard.publication/3p"
	authored := &fakeAuthoredPublicationCrawler{byDID: map[string][]discovercrawl.AuthoredPublication{
		"did:plc:author": {{Key: pubURI, Kind: "standardfeed", Title: "My Zine", SiteURL: "https://myzine.example"}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, authored, noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Publication != pubURI || got[0].Title != "My Zine" {
		t.Fatalf("got[0] = %+v, want the authored publication ranked first", got[0])
	}
	if got[0].Reason.TopSignal != "author" {
		t.Errorf("Reason.TopSignal = %q, want author", got[0].Reason.TopSignal)
	}
}

func TestDiscoverSourcesHandler_ShareResolvesViaFeedURLProvenance(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://blog.example/post", FeedURL: "https://blog.example/feed", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://blog.example/feed" {
		t.Fatalf("got = %+v, want the share's feedUrl provenance resolved directly", got)
	}
	if got[0].Reason.TopSignal != "share" {
		t.Errorf("Reason.TopSignal = %q, want share", got[0].Reason.TopSignal)
	}
}

func TestDiscoverSourcesHandler_ShareResolvesViaDocumentTier2Lookup(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	document := "at://did:plc:pub/site.standard.document/9z"
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "standardfeed", Document: document, CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	resolver := noEntryResolver()
	resolver.byGuid[document] = pubURI
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), shares, resolver, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Publication != pubURI {
		t.Fatalf("got = %+v, want the recommend's document resolved via Tier-2 guid lookup", got)
	}
}

func TestDiscoverSourcesHandler_ShareWithOnlyItemURLResolvesViaTier2Fallback(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	itemURL := "https://blog.example/some-post"
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: itemURL, CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	resolver := noEntryResolver()
	resolver.byItemURL[itemURL] = "https://blog.example/feed"
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), shares, resolver, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://blog.example/feed" {
		t.Fatalf("got = %+v, want the itemUrl-only share resolved via the Tier-2 fallback", got)
	}
}

// A Tier-2 itemUrl lookup returns feed_url verbatim, which may be a variant of another person's normalized subscribe key; both must land on one candidate.
func TestDiscoverSourcesHandler_ShareResolvedViaTier2MergesWithSubscribeKeyVariant(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:alice"),
		discoverFollow("did:plc:bob"),
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://blog.example/feed", Kind: "rss", Title: "Blog"}},
	}}
	itemURL := "https://blog.example/some-post"
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:bob": {{Kind: "rss", ItemURL: itemURL, CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	resolver := noEntryResolver()
	resolver.byItemURL[itemURL] = "https://blog.example/feed/"
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), shares, resolver, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want the variant-keyed share merged into one candidate", got)
	}
	// Alice subscribes, bob only shares; subscribe outranks share, so bob doesn't carry the top signal and mustn't inflate the count.
	if got[0].Reason.StrongCount != 1 {
		t.Errorf("Reason.StrongCount = %d, want 1 (bob's share doesn't carry the top signal subscribe)", got[0].Reason.StrongCount)
	}
	if got[0].Reason.TopSignal != "subscribe" {
		t.Errorf("Reason.TopSignal = %q, want subscribe", got[0].Reason.TopSignal)
	}
}

func TestDiscoverSourcesHandler_UnresolvableShareProducesNoCandidateNoError(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://unresolvable.example/post", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unresolvable reaction must not error)", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want no candidate for an unresolvable share", got)
	}
}

func TestDiscoverSourcesHandler_FollowedPersonWithOnlySaveRecordsProducesNoCandidate(t *testing.T) {
	// Inverted from the old SaveResolvesAndCarriesSaveReason: SPEC <saving-sharing> confines saves to the anonymous trending batch, never personal source ranking. There is no PersonalSaveCrawler to wire in any more, so a followed person whose only real-world record is a save has nothing left to surface: every other crawl reports empty.
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want no candidate for a followed person whose only reader-network record is a save", got)
	}
	for _, w := range got {
		if w.Reason.TopSignal == "save" {
			t.Errorf("got = %+v, want the response to never carry topSignal:save", got)
		}
	}
}

func TestDiscoverSourcesHandler_SubscribeSignalOutranksShareSignalFromSamePerson(t *testing.T) {
	// SPEC <discovery> Signal ordering.
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://a.example/feed", Kind: "rss"}},
	}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://b.example/post", FeedURL: "https://b.example/feed", CreatedAt: "2026-07-09T00:00:00Z"}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0].FeedURL != "https://a.example/feed" {
		t.Fatalf("got = %+v, want the subscribed source ranked first", got)
	}
}

func TestDiscoverSourcesHandler_ShareCrawlFailureDegradesGracefully(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	shares := &fakePersonalShareCrawler{err: map[string]error{"did:plc:alice": errors.New("pds unreachable")}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the share crawl failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://good.example/feed" {
		t.Fatalf("got = %+v, want the subscribe signal to survive a share-crawl failure", got)
	}
}

func TestDiscoverSourcesHandler_AuthoredCrawlFailureDegradesGracefully(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	authored := &fakeAuthoredPublicationCrawler{err: map[string]error{"did:plc:alice": errors.New("pds unreachable")}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, authored, noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the authored-publication crawl failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://good.example/feed" {
		t.Fatalf("got = %+v, want the subscribe signal to survive an authored-publication crawl failure", got)
	}
}

// --- The user's own foreign records. SPEC <discovery>. ---

func TestDiscoverSourcesHandler_OwnForeignSkyreaderSubscription_SurfacesWithSelfReason(t *testing.T) {
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://sky.example/feed", Kind: "rss", Title: "Sky Blog"},
			App:          discovercrawl.ForeignAppSkyreader,
		}},
	}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://sky.example/feed" {
		t.Fatalf("got = %+v, want the own-Skyreader subscription to surface", got)
	}
	if got[0].Reason.SelfSourceApp != "skyreader" {
		t.Errorf("Reason.SelfSourceApp = %q, want skyreader", got[0].Reason.SelfSourceApp)
	}
	if len(ownForeign.calls) != 1 || ownForeign.calls[0] != "did:plc:me" {
		t.Errorf("ownForeign.calls = %v, want one call for the session did", ownForeign.calls)
	}
}

func TestDiscoverSourcesHandler_OwnForeignGleanSubscription_SurfacesWithSelfReason(t *testing.T) {
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://glean.example/feed", Kind: "rss"},
			App:          discovercrawl.ForeignAppGlean,
		}},
	}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Reason.SelfSourceApp != "glean" {
		t.Fatalf("got = %+v, want the own-Glean subscription with a glean self reason", got)
	}
}

func TestDiscoverSourcesHandler_OwnForeignSubscription_AlreadySubscribedExcluded(t *testing.T) {
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://already.example/feed", Kind: "rss"},
			App:          discovercrawl.ForeignAppSkyreader,
		}},
	}}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{{FeedUrl: "https://already.example/feed"}}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), ownForeign, subs, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the already-subscribed foreign record excluded", got)
	}
}

func TestDiscoverSourcesHandler_OwnForeignSubscription_HiddenExcluded(t *testing.T) {
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://hidden.example/feed", Kind: "rss"},
			App:          discovercrawl.ForeignAppSkyreader,
		}},
	}}
	hides := newFakeDiscoverHiddenReader()
	hides.hide("did:plc:me", "source", "https://hidden.example/feed")
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), hides, &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the hidden foreign record excluded", got)
	}
}

func TestDiscoverSourcesHandler_OwnForeignSubscription_SurfacesWithNoFollowsAtAll(t *testing.T) {
	// Self-import is independent of the follow graph, unlike the cold-start rule for the followed-people path. SPEC <discovery>.
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://sky.example/feed", Kind: "rss"},
			App:          discovercrawl.ForeignAppSkyreader,
		}},
	}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want the self-tier suggestion despite zero follows", got)
	}
}

func TestDiscoverSourcesHandler_OwnForeignSubscription_EmptyHistoryProducesNoArtifacts(t *testing.T) {
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty (no reasons/groups leak from an empty foreign history)", got)
	}
}

func TestDiscoverSourcesHandler_OwnForeignCrawlFailureDegradesGracefully(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	ownForeign := &fakeOwnForeignSubscriptionCrawler{err: map[string]error{"did:plc:me": errors.New("pds unreachable")}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the own-foreign crawl failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://good.example/feed" {
		t.Fatalf("got = %+v, want the followed-person suggestion to survive an own-foreign crawl failure", got)
	}
}

func TestDiscoverSourcesHandler_OwnForeignSubscription_SelfOutranksStrongFollowerSuggestion(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://strong.example/feed", Kind: "rss"}},
	}}
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://self.example/feed", Kind: "rss"},
			App:          discovercrawl.ForeignAppSkyreader,
		}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0].FeedURL != "https://self.example/feed" {
		t.Fatalf("got = %+v, want the self-tier suggestion ranked first (SPEC: self > reader-network follow)", got)
	}
}

// --- Cold-cache fan-out concurrency bound ---

func TestDiscoverSourcesHandler_CrawlFanOutIsBoundedAndOverlaps(t *testing.T) {
	const numCandidates = 20 // must exceed discoverCrawlFanoutLimit to observe saturation
	var rows []db.UserFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < numCandidates; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		rows = append(rows, discoverFollow(did))
		byDID[did] = []discovercrawl.Subscription{{Key: "https://feed" + string(rune('a'+i)) + ".example/f", Kind: "rss"}}
	}
	follows := &fakeDiscoverFollowsReader{rows: rows}

	tracker := newInFlightTracker(discoverCrawlFanoutLimit)
	release := make(chan struct{})
	crawler := &gatedSubscriptionCrawler{tracker: tracker, release: release, byDID: byDID}

	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-tracker.reached:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for crawls to reach the fan-out bound; handler may not be running crawls concurrently")
	}

	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not complete after release")
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if max := tracker.maxObserved(); max <= 1 {
		t.Errorf("maxObserved in-flight = %d, want > 1 (proves crawls overlap)", max)
	}
	if max := tracker.maxObserved(); max > discoverCrawlFanoutLimit {
		t.Errorf("maxObserved in-flight = %d, want <= %d (fan-out bound)", max, discoverCrawlFanoutLimit)
	}
}

// --- Trending merge: per-card trending flag, trending-only cards, unified endpoint. SPEC <discovery>. ---

func TestDiscoverSourcesHandler_TrendingFlagTrue_WhenPersonalCandidateAboveBar(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://both.example/feed", Kind: "rss"}},
	}}
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://both.example/feed")}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || !got[0].Reason.Trending {
		t.Fatalf("got = %+v, want the personal card flagged trending", got)
	}
	if got[0].Reason.StrongCount != 1 {
		t.Errorf("Reason.StrongCount = %d, want 1 (the personal signal must survive alongside the trending flag)", got[0].Reason.StrongCount)
	}
}

func TestDiscoverSourcesHandler_TrendingFlagFalse_WhenBelowBarOrAbsent(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://onlypersonal.example/feed", Kind: "rss"}},
	}}
	// Only 2 distinct repos: below MinDistinctRepos, so the flag must not fire even though the fake bypasses the SQL-level bar.
	belowBar := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{
		trendingSignal("did:plc:repo1", "https://onlypersonal.example/feed", "rss", "subscribe"),
		trendingSignal("did:plc:repo2", "https://onlypersonal.example/feed", "rss", "subscribe"),
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), belowBar, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Reason.Trending {
		t.Fatalf("got = %+v, want the personal card not flagged trending (only 2 distinct repos)", got)
	}

	// Absent from the aggregate entirely.
	h2 := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())
	req2 := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr2 := httptest.NewRecorder()
	h2.ServeHTTP(rr2, req2)
	var got2 discoverSourceWires
	if err := json.Unmarshal(rr2.Body.Bytes(), &got2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got2) != 1 || got2[0].Reason.Trending {
		t.Fatalf("got = %+v, want the personal card not flagged trending (absent from the aggregate)", got2)
	}
}

func TestDiscoverSourcesHandler_TrendingOnlyCardsAppendedAfterPersonalCards(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://personal.example/feed", Kind: "rss"}},
	}}
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://trending-only.example/feed")}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v, want 1 personal card + 1 trending-only card", got)
	}
	if got[0].FeedURL != "https://personal.example/feed" {
		t.Errorf("got[0] = %+v, want the personal card first", got[0])
	}
	if got[1].FeedURL != "https://trending-only.example/feed" || !got[1].Reason.Trending {
		t.Errorf("got[1] = %+v, want the trending-only card appended last", got[1])
	}
}

func TestDiscoverSourcesHandler_TrendingOnlyReasonIsEmptyExceptTheFlag(t *testing.T) {
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://trending-only.example/feed")}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 trending-only card", got)
	}
	r := got[0].Reason
	if !r.Trending {
		t.Errorf("Reason.Trending = false, want true")
	}
	if r.StrongCount != 0 || r.WeakCount != 0 || r.TopFollowerDID != "" || r.TopFollowerNetwork != "" || r.TopSignal != "" || r.SelfSourceApp != "" || r.AuthorDID != "" || len(r.FollowerDIDs) != 0 {
		t.Errorf("Reason = %+v, want everything but Trending zero-valued (no counts/DIDs leak from contributing repos)", r)
	}
}

func TestDiscoverSourcesHandler_SourceWithBothPersonalSignalAndTrendingAggregate_AppearsExactlyOnce(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://both.example/feed", Kind: "rss"}},
	}}
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://both.example/feed")}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want exactly one card (personal, not duplicated as trending-only)", got)
	}
	if got[0].Reason.StrongCount != 1 || !got[0].Reason.Trending {
		t.Errorf("got[0].Reason = %+v, want the personal reason preserved plus the trending flag", got[0].Reason)
	}
}

func TestDiscoverSourcesHandler_TrendingOnlyExcludesAlreadySubscribed(t *testing.T) {
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://already.example/feed")}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{{FeedUrl: "https://already.example/feed"}}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), subs, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the already-subscribed source excluded from trending-only", got)
	}
}

func TestDiscoverSourcesHandler_TrendingOnlyExcludesHidden(t *testing.T) {
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://hidden.example/feed")}
	hides := newFakeDiscoverHiddenReader()
	hides.hide("did:plc:me", "source", "https://hidden.example/feed")
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), hides, trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the hidden source excluded from trending-only", got)
	}
}

func TestDiscoverSourcesHandler_TrendingOnlyRespectsLanguageFilter(t *testing.T) {
	var rows []db.DiscoverTrendingSignal
	rows = append(rows, threeRepoSignal("https://ja.example/feed")...)
	rows = append(rows, threeRepoSignal("https://en.example/feed")...)
	trending := &fakeDiscoverTrendingSignalsReader{rows: rows}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{{FeedUrl: "https://myenglish.example/feed"}}}
	languages := &fakeDiscoverFeedLanguageReader{rows: []db.ListFeedLanguagesRow{
		feedLanguage("https://ja.example/feed", "ja"),
		feedLanguage("https://en.example/feed", "en"),
		feedLanguage("https://myenglish.example/feed", "en"),
	}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), subs, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, languages, noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://en.example/feed" {
		t.Fatalf("got = %+v, want only the English trending-only source (Japanese-detected source must be filtered)", got)
	}
}

func TestDiscoverSourcesHandler_TrendingOnlyCapsAtEight(t *testing.T) {
	var rows []db.DiscoverTrendingSignal
	for i := 0; i < 10; i++ {
		key := "https://" + string(rune('a'+i)) + ".trending-only.example/feed"
		rows = append(rows, threeRepoSignal(key)...)
	}
	trending := &fakeDiscoverTrendingSignalsReader{rows: rows}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestDiscoverSourcesHandler_ColdStartZeroFollows_StillReturnsTrendingOnlyCards(t *testing.T) {
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://trending.example/feed")}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:new-user", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://trending.example/feed" {
		t.Fatalf("got = %+v, want the trending-only card for a zero-follow account (cold start must not early-return empty)", got)
	}
}

func TestDiscoverSourcesHandler_TrendingSignalsReadFailure_DegradesToPersonalOnly(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	trending := &fakeDiscoverTrendingSignalsReader{err: errors.New("db down")}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the trending-signals read failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://good.example/feed" || got[0].Reason.Trending {
		t.Fatalf("got = %+v, want only the personal card, untrended", got)
	}
}

func TestDiscoverSourcesHandler_FeedLanguagesReadFailure_DegradesToUnfilteredTrendingOnly(t *testing.T) {
	trending := &fakeDiscoverTrendingSignalsReader{rows: threeRepoSignal("https://a.example/feed")}
	languages := &fakeDiscoverFeedLanguageReader{err: errors.New("db down")}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), trending, languages, noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the feed-languages read failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://a.example/feed" {
		t.Fatalf("got = %+v, want the trending-only card unfiltered", got)
	}
}

func TestDiscoverSourcesHandler_ReasonCarriesFollowerDIDsOnPersonalCards(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:alice"),
		discoverFollow("did:plc:bob"),
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://shared.example/feed", Kind: "rss"}},
		"did:plc:bob":   {{Key: "https://shared.example/feed", Kind: "rss"}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 suggestion", got)
	}
	if len(got[0].Reason.FollowerDIDs) != 2 {
		t.Fatalf("Reason.FollowerDIDs = %v, want 2 DIDs (alice and bob)", got[0].Reason.FollowerDIDs)
	}
	if got[0].Reason.FollowerDIDs[0] != got[0].Reason.TopFollowerDID {
		t.Errorf("FollowerDIDs[0] = %q, want it to match TopFollowerDID %q", got[0].Reason.FollowerDIDs[0], got[0].Reason.TopFollowerDID)
	}
}

func TestDiscoverSourcesHandler_AuthorDIDSetOnlyWhenTopSignalIsAuthor(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:author"),
		discoverFollow("did:plc:subscriber"),
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:subscriber": {{Key: "https://ordinary.example/feed", Kind: "rss"}},
	}}
	pubURI := "at://did:plc:author/site.standard.publication/3p"
	authored := &fakeAuthoredPublicationCrawler{byDID: map[string][]discovercrawl.AuthoredPublication{
		"did:plc:author": {{Key: pubURI, Kind: "standardfeed", Title: "My Zine"}},
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, authored, noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	authorCard, subscribeCard := got[0], got[1]
	if authorCard.Publication != pubURI {
		t.Fatalf("got[0] = %+v, want the authored publication ranked first", authorCard)
	}
	if authorCard.Reason.AuthorDID != "did:plc:author" {
		t.Errorf("authorCard.Reason.AuthorDID = %q, want did:plc:author", authorCard.Reason.AuthorDID)
	}
	if subscribeCard.Reason.TopSignal != "subscribe" || subscribeCard.Reason.AuthorDID != "" {
		t.Errorf("subscribeCard.Reason = %+v, want AuthorDID empty for a non-author top signal", subscribeCard.Reason)
	}
}

func TestDiscoverSourcesHandler_AuthorDIDAndFollowerDIDsEmpty_OnSelfCreditedCard(t *testing.T) {
	ownForeign := &fakeOwnForeignSubscriptionCrawler{byDID: map[string][]discovercrawl.ForeignSubscription{
		"did:plc:me": {{
			Subscription: discovercrawl.Subscription{Key: "https://sky.example/feed", Kind: "rss"},
			App:          discovercrawl.ForeignAppSkyreader,
		}},
	}}
	h := DiscoverSourcesHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), ownForeign, &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), noTitleBackfill())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Reason.SelfSourceApp != "skyreader" {
		t.Fatalf("got = %+v, want the self-credited card", got)
	}
	if got[0].Reason.AuthorDID != "" {
		t.Errorf("Reason.AuthorDID = %q, want empty on a self-credited card", got[0].Reason.AuthorDID)
	}
	if len(got[0].Reason.FollowerDIDs) != 0 {
		t.Errorf("Reason.FollowerDIDs = %v, want empty on a self-credited card", got[0].Reason.FollowerDIDs)
	}
}

// --- Title/siteUrl backfill from network trending signals. SPEC <discovery>. ---

func TestDiscoverSourcesHandler_ShareOnlyCandidateGetsTitleBackfilled(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://blog.example/post", FeedURL: "https://blog.example/feed", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	backfill := &fakeDiscoverSourceTitleBackfillReader{rows: map[string]db.GetDiscoverTrendingSignalTitleRow{
		"https://blog.example/feed": titleBackfillRow("Blog Title", "https://blog.example"),
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), backfill)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Blog Title" || got[0].SiteURL != "https://blog.example" {
		t.Fatalf("got = %+v, want the share-only candidate backfilled with title and siteUrl", got)
	}
	if len(backfill.calls) != 1 || backfill.calls[0] != "https://blog.example/feed" {
		t.Errorf("backfill.calls = %v, want one lookup for the reaction-only candidate's key", backfill.calls)
	}
}

func TestDiscoverSourcesHandler_AlreadyTitledCandidateIsNeverQueriedOrOverridden(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://titled.example/feed", Kind: "rss", Title: "Original Title"}},
	}}
	backfill := &fakeDiscoverSourceTitleBackfillReader{rows: map[string]db.GetDiscoverTrendingSignalTitleRow{
		"https://titled.example/feed": titleBackfillRow("Trending Title", "https://trending.example"),
	}}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), backfill)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Original Title" {
		t.Fatalf("got = %+v, want the subscribe signal's own title, never overridden", got)
	}
	if len(backfill.calls) != 0 {
		t.Errorf("backfill.calls = %v, want zero lookups for an already-titled candidate", backfill.calls)
	}
}

func TestDiscoverSourcesHandler_TitleBackfillErrorDegradesGracefully(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://err.example/post", FeedURL: "https://err.example/feed", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	backfill := &fakeDiscoverSourceTitleBackfillReader{err: errors.New("db unreachable")}
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), backfill)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the backfill reader failing", rr.Code)
	}
	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].FeedURL != "https://err.example/feed" || got[0].Title != "" {
		t.Fatalf("got = %+v, want the candidate present but untitled after a backfill error", got)
	}
}

func TestDiscoverSourcesHandler_OnlyRssTitlelessCandidatesAreQueriedForBackfill(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: pubURI, Kind: "standardfeed"}}, // title-less, but not rss: must never be queried.
	}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://blog.example/post", FeedURL: "https://blog.example/feed", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	backfill := noTitleBackfill()
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), shares, noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), backfill)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v, want both candidates to surface", got)
	}
	if len(backfill.calls) != 1 || backfill.calls[0] != "https://blog.example/feed" {
		t.Errorf("backfill.calls = %v, want only the rss candidate's key queried, never the standardfeed one", backfill.calls)
	}
}

func TestDiscoverSourcesHandler_NoMissingTitles_BackfillReaderNeverCalled(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://has-title.example/feed", Kind: "rss", Title: "Has Title"}},
	}}
	backfill := noTitleBackfill()
	h := DiscoverSourcesHandler(follows, noAdjacentFollows(), noOwnForeignSubscriptions(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), noEntryResolver(), newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, noFeedLanguages(), backfill)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverSourceWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 suggestion", got)
	}
	if len(backfill.calls) != 0 {
		t.Errorf("backfill.calls = %v, want zero calls when no candidate is missing a title", backfill.calls)
	}
}

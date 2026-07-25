package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discovermemo"
)

// recordingDiscoverInvalidator captures which DIDs a mutation handler staled.
type recordingDiscoverInvalidator struct {
	mu   sync.Mutex
	dids []string
}

func (r *recordingDiscoverInvalidator) Invalidate(did string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dids = append(r.dids, did)
}

func (r *recordingDiscoverInvalidator) invalidated() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.dids...)
}

func serveDiscover(t *testing.T, h http.Handler, path, cursor string) discoverPageWire[json.RawMessage] {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cursor != "" {
		query := req.URL.Query()
		query.Set("cursor", cursor)
		req.URL.RawQuery = query.Encode()
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withSession(req, "did:plc:me", "sess1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var page discoverPageWire[json.RawMessage]
	if err := json.Unmarshal(rr.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal page: %v", err)
	}
	return page
}

func discoverItemKeys(t *testing.T, page discoverPageWire[json.RawMessage]) []string {
	t.Helper()
	out := make([]string, 0, len(page.Items))
	for _, raw := range page.Items {
		var item struct {
			Key string `json:"key"`
			DID string `json:"did"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("unmarshal item: %v", err)
		}
		out = append(out, item.Key+item.DID)
	}
	return out
}

// --- Sources ---

func memoSourcesHandler(
	follows DiscoverFollowsReader,
	crawler SubscriptionCrawler,
	hides DiscoverHiddenReader,
	trending DiscoverTrendingSignalsReader,
	memo DiscoverMemo[DiscoverSourcesPayload],
) http.Handler {
	return DiscoverSourcesHandler(
		follows,
		noAdjacentFollows(),
		noOwnForeignSubscriptions(),
		&fakeDiscoverSubsReader{},
		crawler,
		noAuthoredPublications(),
		noPersonalShares(),
		noEntryResolver(),
		hides,
		trending,
		noFeedLanguages(),
		noTitleBackfill(),
		memo,
	)
}

func TestDiscoverSourcesHandler_SecondRequestServesTheMemoWithoutRecrawling(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:friend": {{Key: "https://one.example/feed", Kind: "rss", Title: "Example Publication"}},
	}}
	h := memoSourcesHandler(follows, crawler, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, discovermemo.New[DiscoverSourcesPayload](discovermemo.DefaultTTL))

	first := serveDiscover(t, h, "/api/discover/sources", "")
	crawled := len(crawler.calls)
	if crawled == 0 {
		t.Fatal("first request crawled nothing; the test can't observe a memo hit")
	}

	second := serveDiscover(t, h, "/api/discover/sources", "")

	if len(crawler.calls) != crawled {
		t.Errorf("crawler calls after the second request = %d, want %d (memo hit must not recrawl)", len(crawler.calls), crawled)
	}
	if got, want := discoverItemKeys(t, second), discoverItemKeys(t, first); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("memoized page = %v, want %v", got, want)
	}
}

func TestDiscoverSourcesHandler_CursorPageIsSlicedFromTheMemoWithoutRecrawling(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	personal := make([]discovercrawl.Subscription, 6)
	trendingRows := make([]db.DiscoverTrendingSignal, 0, 18)
	for i := range 6 {
		personal[i] = discovercrawl.Subscription{Key: fmt.Sprintf("https://personal-%d.example/feed", i), Kind: "rss"}
		trendingRows = append(trendingRows, threeRepoSignal(fmt.Sprintf("https://trending-%d.example/feed", i))...)
	}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{"did:plc:friend": personal}}
	h := memoSourcesHandler(follows, crawler, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{rows: trendingRows}, discovermemo.New[DiscoverSourcesPayload](discovermemo.DefaultTTL))

	first := serveDiscover(t, h, "/api/discover/sources", "")
	if len(first.Items) != 8 || first.NextCursor == "" {
		t.Fatalf("first page = %d items, cursor %q, want 8 and a cursor", len(first.Items), first.NextCursor)
	}
	crawled := len(crawler.calls)

	second := serveDiscover(t, h, "/api/discover/sources", first.NextCursor)

	if len(crawler.calls) != crawled {
		t.Errorf("crawler calls after the cursor page = %d, want %d (page 2 must come off the memo)", len(crawler.calls), crawled)
	}
	if len(second.Items) != 4 || second.NextCursor != "" {
		t.Fatalf("second page = %d items, cursor %q, want 4 and no cursor", len(second.Items), second.NextCursor)
	}
	seen := map[string]struct{}{}
	for _, key := range append(discoverItemKeys(t, first), discoverItemKeys(t, second)...) {
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate item %q across memoized pages", key)
		}
		seen[key] = struct{}{}
	}
}

func TestDiscoverSourcesHandler_NilMemoRebuildsEveryRequest(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:friend": {{Key: "https://one.example/feed", Kind: "rss"}},
	}}
	h := memoSourcesHandler(follows, crawler, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, nil)

	first := serveDiscover(t, h, "/api/discover/sources", "")
	crawled := len(crawler.calls)
	second := serveDiscover(t, h, "/api/discover/sources", "")

	if len(crawler.calls) != 2*crawled {
		t.Errorf("crawler calls with a nil memo = %d, want %d (memoization off)", len(crawler.calls), 2*crawled)
	}
	if got, want := discoverItemKeys(t, second), discoverItemKeys(t, first); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("second page = %v, want %v", got, want)
	}
}

func TestDiscoverSourcesHandler_InvalidatedMemoPicksUpANewHide(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:friend": {{Key: "https://one.example/feed", Kind: "rss"}},
	}}
	hides := newFakeDiscoverHiddenReader()
	memo := discovermemo.New[DiscoverSourcesPayload](discovermemo.DefaultTTL)
	h := memoSourcesHandler(follows, crawler, hides, &fakeDiscoverTrendingSignalsReader{}, memo)

	if got := serveDiscover(t, h, "/api/discover/sources", ""); len(got.Items) != 1 {
		t.Fatalf("first page = %d items, want 1", len(got.Items))
	}

	hides.hide("did:plc:me", "source", "https://one.example/feed")
	if got := serveDiscover(t, h, "/api/discover/sources", ""); len(got.Items) != 1 {
		t.Fatalf("memoized page = %d items, want the stale 1 until the memo is invalidated", len(got.Items))
	}

	memo.Invalidate("did:plc:me")
	if got := serveDiscover(t, h, "/api/discover/sources", ""); len(got.Items) != 0 {
		t.Fatalf("rebuilt page = %d items, want 0 (the hidden source must drop out)", len(got.Items))
	}
}

func TestDiscoverSourcesHandler_MemoIsPerUser(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:friend": {{Key: "https://one.example/feed", Kind: "rss"}},
	}}
	h := memoSourcesHandler(follows, crawler, newFakeDiscoverHiddenReader(), &fakeDiscoverTrendingSignalsReader{}, discovermemo.New[DiscoverSourcesPayload](discovermemo.DefaultTTL))

	serveDiscover(t, h, "/api/discover/sources", "")
	crawled := len(crawler.calls)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources", nil), "did:plc:other", "sess2")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	if len(crawler.calls) == crawled {
		t.Error("a second user was served the first user's memo")
	}
}

// --- People ---

func memoPeopleHandler(
	adjacent AdjacentFollowCrawler,
	crawler SubscriptionCrawler,
	hides DiscoverHiddenReader,
	trendingFollows DiscoverTrendingFollowsReader,
	signals DiscoverTrendingEligibilityReader,
	memo DiscoverMemo[DiscoverPeoplePayload],
) http.Handler {
	return DiscoverPeopleHandler(
		&fakeDiscoverFollowsReader{},
		adjacent,
		noReaderNetworkFollows(),
		&fakeDiscoverSubsReader{},
		crawler,
		noAuthoredPublications(),
		noPersonalShares(),
		hides,
		trendingFollows,
		signals,
		memo,
	)
}

func TestDiscoverPeopleHandler_SecondRequestServesTheMemoWithoutRecrawling(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{{DID: "did:plc:carol", Network: "bluesky"}}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
	}}
	h := memoPeopleHandler(adjacent, crawler, newFakeDiscoverHiddenReader(), noTrendingFollows(), noTrendingEligibility(), discovermemo.New[DiscoverPeoplePayload](discovermemo.DefaultTTL))

	first := serveDiscover(t, h, "/api/discover/people", "")
	crawled := len(crawler.calls)
	if crawled == 0 {
		t.Fatal("first request crawled nothing; the test can't observe a memo hit")
	}

	second := serveDiscover(t, h, "/api/discover/people", "")

	if len(crawler.calls) != crawled {
		t.Errorf("crawler calls after the second request = %d, want %d (memo hit must not recrawl)", len(crawler.calls), crawled)
	}
	if got, want := discoverItemKeys(t, second), discoverItemKeys(t, first); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("memoized page = %v, want %v", got, want)
	}
}

func TestDiscoverPeopleHandler_CursorPageIsSlicedFromTheMemoWithoutRecrawling(t *testing.T) {
	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{}
	trendingFollowRows := make([]db.DiscoverTrendingFollow, 0, 18)
	signalRows := make([]db.DiscoverTrendingSignal, 0, 6)
	for i := range 6 {
		did := fmt.Sprintf("did:plc:personal%d", i)
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{{Key: fmt.Sprintf("https://personal-%d.example/feed", i), Kind: "rss"}}

		trendingDID := fmt.Sprintf("did:plc:trending%d", i)
		trendingFollowRows = append(trendingFollowRows, threeFollowers(trendingDID)...)
		signalRows = append(signalRows, eligibilitySignal(trendingDID))
	}
	adjacent := &fakeAdjacentFollowCrawler{follows: adjacentFollows}
	crawler := &fakeSubscriptionCrawler{byDID: byDID}
	h := memoPeopleHandler(
		adjacent,
		crawler,
		newFakeDiscoverHiddenReader(),
		&fakeDiscoverTrendingFollowsReader{rows: trendingFollowRows},
		&fakeDiscoverTrendingSignalsReader{rows: signalRows},
		discovermemo.New[DiscoverPeoplePayload](discovermemo.DefaultTTL),
	)

	first := serveDiscover(t, h, "/api/discover/people", "")
	if len(first.Items) != 8 || first.NextCursor == "" {
		t.Fatalf("first page = %d items, cursor %q, want 8 and a cursor", len(first.Items), first.NextCursor)
	}
	crawled := len(crawler.calls)

	second := serveDiscover(t, h, "/api/discover/people", first.NextCursor)

	if len(crawler.calls) != crawled {
		t.Errorf("crawler calls after the cursor page = %d, want %d (page 2 must come off the memo)", len(crawler.calls), crawled)
	}
	if len(second.Items) != 4 || second.NextCursor != "" {
		t.Fatalf("second page = %d items, cursor %q, want 4 and no cursor", len(second.Items), second.NextCursor)
	}
	seen := map[string]struct{}{}
	for _, key := range append(discoverItemKeys(t, first), discoverItemKeys(t, second)...) {
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate item %q across memoized pages", key)
		}
		seen[key] = struct{}{}
	}
}

func TestDiscoverPeopleHandler_NilMemoRebuildsEveryRequest(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{{DID: "did:plc:carol", Network: "bluesky"}}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
	}}
	h := memoPeopleHandler(adjacent, crawler, newFakeDiscoverHiddenReader(), noTrendingFollows(), noTrendingEligibility(), nil)

	first := serveDiscover(t, h, "/api/discover/people", "")
	crawled := len(crawler.calls)
	second := serveDiscover(t, h, "/api/discover/people", "")

	if len(crawler.calls) != 2*crawled {
		t.Errorf("crawler calls with a nil memo = %d, want %d (memoization off)", len(crawler.calls), 2*crawled)
	}
	if got, want := discoverItemKeys(t, second), discoverItemKeys(t, first); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("second page = %v, want %v", got, want)
	}
}

func TestDiscoverPeopleHandler_InvalidatedMemoPicksUpANewHide(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{{DID: "did:plc:carol", Network: "bluesky"}}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
	}}
	hides := newFakeDiscoverHiddenReader()
	memo := discovermemo.New[DiscoverPeoplePayload](discovermemo.DefaultTTL)
	h := memoPeopleHandler(adjacent, crawler, hides, noTrendingFollows(), noTrendingEligibility(), memo)

	if got := serveDiscover(t, h, "/api/discover/people", ""); len(got.Items) != 1 {
		t.Fatalf("first page = %d items, want 1", len(got.Items))
	}

	hides.hide("did:plc:me", "person", "did:plc:carol")
	if got := serveDiscover(t, h, "/api/discover/people", ""); len(got.Items) != 1 {
		t.Fatalf("memoized page = %d items, want the stale 1 until the memo is invalidated", len(got.Items))
	}

	memo.Invalidate("did:plc:me")
	if got := serveDiscover(t, h, "/api/discover/people", ""); len(got.Items) != 0 {
		t.Fatalf("rebuilt page = %d items, want 0 (the hidden person must drop out)", len(got.Items))
	}
}

// --- Invalidation from the mutation handlers ---

func wantInvalidated(t *testing.T, memo *recordingDiscoverInvalidator, did string) {
	t.Helper()
	if got := memo.invalidated(); len(got) != 1 || got[0] != did {
		t.Errorf("invalidated = %v, want exactly [%s]", got, did)
	}
}

func wantNotInvalidated(t *testing.T, memo *recordingDiscoverInvalidator) {
	t.Helper()
	if got := memo.invalidated(); len(got) != 0 {
		t.Errorf("invalidated = %v, want none (nothing the suggestion pool reads changed)", got)
	}
}

func TestDiscoverHidesCreate_StalesTheUsersDiscoverMemo(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	memo := &recordingDiscoverInvalidator{}
	h := DiscoverHidesCreateHandler(idx, idx, memo)

	body := `{"targetKind":"source","targetKey":"https://example.com/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantInvalidated(t, memo, "did:plc:me")
}

func TestDiscoverHidesCreate_RejectedRequestLeavesTheMemoAlone(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	memo := &recordingDiscoverInvalidator{}
	h := DiscoverHidesCreateHandler(idx, idx, memo)

	body := `{"targetKind":"nonsense","targetKey":"https://example.com/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	wantNotInvalidated(t, memo)
}

func TestFollowsCreate_StalesTheUsersDiscoverMemo(t *testing.T) {
	idx := newFakeFollowsIndex()
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"alice.example": mustDID2(t, "did:plc:alice"),
	}}
	memo := &recordingDiscoverInvalidator{}
	h := FollowsCreateHandler(idx, idx, &fakePDS{}, resolver, &recordingRepair{}, memo)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"alice.example"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantInvalidated(t, memo, "did:plc:me")
}

func TestFollowsDelete_StalesTheUsersDiscoverMemo(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:me", Rkey: "3fa", SubjectDid: "did:plc:alice"})
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{
		"blue.morgen.graph.follow": {
			{URI: "at://did:plc:me/blue.morgen.graph.follow/3fa", Value: map[string]any{"subject": "did:plc:alice"}},
		},
	}}
	memo := &recordingDiscoverInvalidator{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/follows/{rkey}", FollowsDeleteHandler(idx, idx, pds, &recordingRepair{}, memo))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/follows/3fa", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	wantInvalidated(t, memo, "did:plc:me")
}

func TestSubscriptionsCreate_StalesTheUsersDiscoverMemo(t *testing.T) {
	idx := newFakeIndex()
	memo := &recordingDiscoverInvalidator{}
	h := SubscriptionsCreateHandler(idx, idx, &fakePDS{}, &fakeDispatcher{}, memo)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml","title":"Example Publication"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantInvalidated(t, memo, "did:plc:alice")
}

func TestSubscriptionsCreate_DedupeOnlyBatchLeavesTheMemoAlone(t *testing.T) {
	idx := newFakeIndex()
	feed := "https://example.test/feed.xml"
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		feed: {Did: "did:plc:alice", Rkey: "3laOLD", FeedUrl: feed},
	}
	memo := &recordingDiscoverInvalidator{}
	h := SubscriptionsCreateHandler(idx, idx, &fakePDS{}, &fakeDispatcher{}, memo)

	body := `{"subscriptions":[{"feedUrl":"https://example.test/feed.xml"}]}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantNotInvalidated(t, memo)
}

func TestSubscriptionsPatch_StalesTheUsersDiscoverMemo(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	memo := &recordingDiscoverInvalidator{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, &fakePDS{}, &fakeDispatcher{}, memo))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"Example Publication"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantInvalidated(t, memo, "did:plc:alice")
}

func TestSubscriptionsPatch_NoDiffLeavesTheMemoAlone(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	memo := &recordingDiscoverInvalidator{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, &fakePDS{}, &fakeDispatcher{}, memo))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la", strings.NewReader(`{}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	wantNotInvalidated(t, memo)
}

func TestSubscriptionsDelete_StalesTheUsersDiscoverMemo(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	memo := &recordingDiscoverInvalidator{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, &fakePDS{}, &recordingRepair{}, memo))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	wantInvalidated(t, memo, "did:plc:alice")
}

// One warm memo entry is rendered by every concurrent request for that user, so ranking must treat the payload's slices and maps as read-only. The memo is warmed first on purpose: the handler fakes are single-request test doubles, and a cold-start stampede would only race them, not the payload.
func TestDiscoverHandlers_ConcurrentRendersOfOneMemoEntryAreRaceFree(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	personal := make([]discovercrawl.Subscription, 6)
	trendingRows := make([]db.DiscoverTrendingSignal, 0, 18)
	for i := range 6 {
		personal[i] = discovercrawl.Subscription{Key: fmt.Sprintf("https://personal-%d.example/feed", i), Kind: "rss"}
		trendingRows = append(trendingRows, threeRepoSignal(fmt.Sprintf("https://trending-%d.example/feed", i))...)
	}
	sources := memoSourcesHandler(
		follows,
		&fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{"did:plc:friend": personal}},
		newFakeDiscoverHiddenReader(),
		&fakeDiscoverTrendingSignalsReader{rows: trendingRows},
		discovermemo.New[DiscoverSourcesPayload](discovermemo.DefaultTTL),
	)

	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{}
	trendingFollowRows := make([]db.DiscoverTrendingFollow, 0, 18)
	signalRows := make([]db.DiscoverTrendingSignal, 0, 6)
	for i := range 6 {
		did := fmt.Sprintf("did:plc:personal%d", i)
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{{Key: fmt.Sprintf("https://personal-%d.example/feed", i), Kind: "rss"}}

		trendingDID := fmt.Sprintf("did:plc:trending%d", i)
		trendingFollowRows = append(trendingFollowRows, threeFollowers(trendingDID)...)
		signalRows = append(signalRows, eligibilitySignal(trendingDID))
	}
	people := memoPeopleHandler(
		&fakeAdjacentFollowCrawler{follows: adjacentFollows},
		&fakeSubscriptionCrawler{byDID: byDID},
		newFakeDiscoverHiddenReader(),
		&fakeDiscoverTrendingFollowsReader{rows: trendingFollowRows},
		&fakeDiscoverTrendingSignalsReader{rows: signalRows},
		discovermemo.New[DiscoverPeoplePayload](discovermemo.DefaultTTL),
	)

	sourcesCursor := serveDiscover(t, sources, "/api/discover/sources", "").NextCursor
	peopleCursor := serveDiscover(t, people, "/api/discover/people", "").NextCursor
	if sourcesCursor == "" || peopleCursor == "" {
		t.Fatal("warm-up produced no cursor; the concurrent phase would not exercise page-2 slicing")
	}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(4)
		go func() { defer wg.Done(); serveDiscover(t, sources, "/api/discover/sources", "") }()
		go func() { defer wg.Done(); serveDiscover(t, sources, "/api/discover/sources", sourcesCursor) }()
		go func() { defer wg.Done(); serveDiscover(t, people, "/api/discover/people", "") }()
		go func() { defer wg.Done(); serveDiscover(t, people, "/api/discover/people", peopleCursor) }()
	}
	wg.Wait()
}

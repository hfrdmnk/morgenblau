package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

type discoverPersonWires []DiscoverPersonWire

func (wires *discoverPersonWires) UnmarshalJSON(data []byte) error {
	var page discoverPageWire[DiscoverPersonWire]
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}
	*wires = page.Items
	return nil
}

func TestDiscoverPeopleHandler_PaginatesBalancedPoolsWithStableCursor(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{}
	trendingSignals := &fakeDiscoverTrendingSignalsReader{}
	for i := range 6 {
		did := fmt.Sprintf("did:plc:personal%d", i)
		adjacent.follows = append(adjacent.follows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		crawler.byDID[did] = []discovercrawl.Subscription{{
			Key:  fmt.Sprintf("https://personal-%d.example/feed", i),
			Kind: "rss",
		}}

		trendingDID := fmt.Sprintf("did:plc:trending%d", i)
		trendingFollows.rows = append(trendingFollows.rows, threeFollowers(trendingDID)...)
		trendingSignals.rows = append(trendingSignals.rows, eligibilitySignal(trendingDID))
	}
	h := DiscoverPeopleHandler(
		&fakeDiscoverFollowsReader{},
		adjacent,
		noReaderNetworkFollows(),
		&fakeDiscoverSubsReader{},
		crawler,
		noAuthoredPublications(),
		noPersonalShares(),
		newFakeDiscoverHiddenReader(),
		trendingFollows,
		trendingSignals,
		nil,
	)

	firstReq := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	firstRR := httptest.NewRecorder()
	h.ServeHTTP(firstRR, firstReq)

	if firstRR.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", firstRR.Code)
	}
	if got := firstRR.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var first discoverPageWire[DiscoverPersonWire]
	if err := json.Unmarshal(firstRR.Body.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first page: %v", err)
	}
	if len(first.Items) != 8 || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want eight items and a cursor", first)
	}
	for i, item := range first.Items {
		if i < 4 && !item.Reason.BlueskyFollow {
			t.Errorf("item %d = %+v, want personal suggestion", i, item)
		}
		if i >= 4 && !item.Reason.Trending {
			t.Errorf("item %d = %+v, want trending-only suggestion", i, item)
		}
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/discover/people", nil)
	query := secondReq.URL.Query()
	query.Set("cursor", first.NextCursor)
	secondReq.URL.RawQuery = query.Encode()
	secondReq = withSession(secondReq, "did:plc:me", "sess1")
	secondRR := httptest.NewRecorder()
	h.ServeHTTP(secondRR, secondReq)

	var second discoverPageWire[DiscoverPersonWire]
	if err := json.Unmarshal(secondRR.Body.Bytes(), &second); err != nil {
		t.Fatalf("unmarshal second page: %v", err)
	}
	if len(second.Items) != 4 || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want four final items", second)
	}
	seen := map[string]struct{}{}
	for _, item := range append(first.Items, second.Items...) {
		if _, duplicate := seen[item.DID]; duplicate {
			t.Fatalf("duplicate person %q across pages", item.DID)
		}
		seen[item.DID] = struct{}{}
	}
}

func TestDiscoverPeopleHandler_RejectsMalformedCursor(t *testing.T) {
	h := newDiscoverPeopleHandler(
		&fakeDiscoverFollowsReader{},
		noAdjacentFollows(),
		noReaderNetworkFollows(),
		&fakeDiscoverSubsReader{},
		&fakeSubscriptionCrawler{},
		newFakeDiscoverHiddenReader(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/discover/people?cursor=not-a-cursor", nil)
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

type fakeReaderNetworkFollowCrawler struct {
	byDID map[string][]discovercrawl.ReaderNetworkFollow
	err   map[string]error
	calls []string
}

// FetchReaderNetworkFollowsBatch mirrors the cached crawler's contract: every requested did gets an entry, a failed did gets nil.
func (f *fakeReaderNetworkFollowCrawler) FetchReaderNetworkFollowsBatch(_ context.Context, dids []string) map[string][]discovercrawl.ReaderNetworkFollow {
	out := make(map[string][]discovercrawl.ReaderNetworkFollow, len(dids))
	for _, did := range dids {
		f.calls = append(f.calls, did)
		if _, failed := f.err[did]; failed {
			out[did] = nil
			continue
		}
		out[did] = f.byDID[did]
	}
	return out
}

func noReaderNetworkFollows() *fakeReaderNetworkFollowCrawler {
	return &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{}}
}

type fakeDiscoverTrendingFollowsReader struct {
	rows []db.DiscoverTrendingFollow
	err  error

	// gotMinDistinctRepos lets tests assert it threads discoverrank.MinDistinctRepos through rather than a duplicated literal.
	gotMinDistinctRepos int64
}

func (f *fakeDiscoverTrendingFollowsReader) ListDiscoverTrendingFollowsAboveBar(_ context.Context, minDistinctRepos int64) ([]db.DiscoverTrendingFollow, error) {
	f.gotMinDistinctRepos = minDistinctRepos
	return f.rows, f.err
}

func noTrendingFollows() *fakeDiscoverTrendingFollowsReader {
	return &fakeDiscoverTrendingFollowsReader{}
}

func noTrendingEligibility() *fakeDiscoverTrendingSignalsReader {
	return &fakeDiscoverTrendingSignalsReader{}
}

func trendingFollow(repoDID, subjectDID string) db.DiscoverTrendingFollow {
	return db.DiscoverTrendingFollow{RepoDid: repoDID, SubjectDid: subjectDID, FetchedAt: "2026-07-09T00:00:00Z"}
}

// threeFollowers gives subjectDID exactly the MinDistinctRepos follower bar.
func threeFollowers(subjectDID string) []db.DiscoverTrendingFollow {
	return []db.DiscoverTrendingFollow{
		trendingFollow("did:plc:repo1", subjectDID),
		trendingFollow("did:plc:repo2", subjectDID),
		trendingFollow("did:plc:repo3", subjectDID),
	}
}

// eligibilitySignal gives subjectDID reader-network presence under its own DID so it clears the trending eligibility bar.
func eligibilitySignal(subjectDID string) db.DiscoverTrendingSignal {
	return trendingSignal(subjectDID, "https://"+subjectDID+".example/feed", "rss", "subscribe")
}

// newDiscoverPeopleHandler builds the handler with every crawler defaulted to "nothing found" except the ones the test explicitly wires; trending inputs default to empty, so callers that don't care about trending see the pre-B6 shape.
func newDiscoverPeopleHandler(
	follows DiscoverFollowsReader,
	adjacent AdjacentFollowCrawler,
	readerFollows ReaderNetworkFollowCrawler,
	subs DiscoverSubscriptionsReader,
	crawler SubscriptionCrawler,
	hides DiscoverHiddenReader,
) http.Handler {
	return DiscoverPeopleHandler(follows, adjacent, readerFollows, subs, crawler, noAuthoredPublications(), noPersonalShares(), hides, noTrendingFollows(), noTrendingEligibility(), nil)
}

func TestDiscoverPeopleHandler_NoCandidatesAtAll_ReturnsEmptyWithoutCrawling(t *testing.T) {
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got discoverPersonWires
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

func TestDiscoverPeopleHandler_EligibilityRequiresReaderNetworkPresence(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:ghost", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want a person with zero reader-network records excluded despite graph proximity", got)
	}
}

func TestDiscoverPeopleHandler_BlueskyFollowProducesBlueskyReason(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://alice.example/feed", Kind: "rss", Title: "Alice's Blog"}},
	}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" {
		t.Fatalf("got = %+v, want alice", got)
	}
	if !got[0].Reason.BlueskyFollow || got[0].Reason.TangledFollow || got[0].Reason.FollowedByDID != "" {
		t.Errorf("Reason = %+v, want only BlueskyFollow set", got[0].Reason)
	}
}

func TestDiscoverPeopleHandler_TangledFollowProducesTangledReason(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:dev", Network: "tangled"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:dev": {{Key: "https://dev.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || !got[0].Reason.TangledFollow || got[0].Reason.BlueskyFollow {
		t.Fatalf("got = %+v, want only TangledFollow set", got)
	}
}

func TestDiscoverPeopleHandler_OneHopCandidateProducesFollowedByReason(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	readerFollows := &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{
		"did:plc:alice": {{DID: "did:plc:carol"}},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverPeopleHandler(follows, noAdjacentFollows(), readerFollows, &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:carol" {
		t.Fatalf("got = %+v, want carol", got)
	}
	if got[0].Reason.FollowedByDID != "did:plc:alice" {
		t.Errorf("Reason.FollowedByDID = %q, want did:plc:alice", got[0].Reason.FollowedByDID)
	}
}

func TestDiscoverPeopleHandler_TasteOverlapRanksAboveStrangerAtEqualActivity(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:stranger", Network: "bluesky"},
		{DID: "did:plc:overlaps", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:stranger": {{Key: "https://stranger-only.example/feed", Kind: "rss"}},
		"did:plc:overlaps": {
			{Key: "https://shared1.example/feed", Kind: "rss"},
			{Key: "https://shared2.example/feed", Kind: "rss"},
			{Key: "https://shared3.example/feed", Kind: "rss"},
			{Key: "https://shared4.example/feed", Kind: "rss"},
		},
	}}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{
		{FeedUrl: "https://shared1.example/feed"},
		{FeedUrl: "https://shared2.example/feed"},
		{FeedUrl: "https://shared3.example/feed"},
		{FeedUrl: "https://shared4.example/feed"},
	}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), subs, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0].DID != "did:plc:overlaps" {
		t.Fatalf("got = %+v, want the 4-shared-source person ranked first", got)
	}
	if got[0].Reason.SharedSourceCount != 4 {
		t.Errorf("SharedSourceCount = %d, want 4", got[0].Reason.SharedSourceCount)
	}
}

// Subscription feed_urls are stored verbatim while candidate keys arrive normalized; overlap must compare canonical forms or a variant subscription undercounts.
func TestDiscoverPeopleHandler_TasteOverlapCountsURLVariantSubscription(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:overlaps", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:overlaps": {{Key: "https://shared.example/feed", Kind: "rss"}},
	}}
	subs := &fakeDiscoverSubsReader{rows: []db.UserSubscription{
		{FeedUrl: "https://SHARED.example:443/feed/"},
	}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), subs, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want one candidate", got)
	}
	if got[0].Reason.SharedSourceCount != 1 {
		t.Errorf("SharedSourceCount = %d, want 1 (URL-variant subscription must count)", got[0].Reason.SharedSourceCount)
	}
}

func TestDiscoverPeopleHandler_AlreadyFollowedExcludedAndNeverCrawled(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://alice.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverPeopleHandler(follows, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want alice excluded (already followed)", got)
	}
	if len(crawler.calls) != 0 {
		t.Errorf("crawler.calls = %v, want alice never crawled once already-followed", crawler.calls)
	}
}

func TestDiscoverPeopleHandler_HiddenExcluded(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://alice.example/feed", Kind: "rss"}},
	}}
	hides := newFakeDiscoverHiddenReader()
	hides.hide("did:plc:me", "person", "did:plc:alice")
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, hides)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want alice excluded (hidden)", got)
	}
	if len(hides.calls) != 1 || hides.calls[0].TargetKind != "person" {
		t.Errorf("hides lookup = %+v, want one call scoped to targetKind person", hides.calls)
	}
}

func TestDiscoverPeopleHandler_CapsAtEight(t *testing.T) {
	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < 10; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{{Key: "https://feed" + string(rune('a'+i)) + ".example/f", Kind: "rss"}}
	}
	adjacent := &fakeAdjacentFollowCrawler{follows: adjacentFollows}
	crawler := &fakeSubscriptionCrawler{byDID: byDID}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

func TestDiscoverPeopleHandler_OneBrokenRepoDoesNotFailWholeRequest(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:broken", Network: "bluesky"},
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{
		byDID: map[string][]discovercrawl.Subscription{
			"did:plc:alice": {{Key: "https://good.example/feed", Kind: "rss"}},
		},
		err: map[string]error{"did:plc:broken": errors.New("pds unreachable")},
	}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite one broken repo", rr.Code)
	}
	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:alice" {
		t.Fatalf("got = %+v, want only alice despite broken's crawl failing", got)
	}
}

func TestDiscoverPeopleHandler_TastePreviewFromSubscriptions(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:alice": {{Key: "https://alice.example/feed", Kind: "rss", Title: "Alice's Blog", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].TastePreview == nil {
		t.Fatalf("got = %+v, want a taste preview", got)
	}
	if len(got[0].TastePreview.Titles) != 1 || got[0].TastePreview.Titles[0] != "Alice's Blog" {
		t.Errorf("TastePreview.Titles = %+v, want [Alice's Blog]", got[0].TastePreview.Titles)
	}
}

func TestDiscoverPeopleHandler_TastePreviewFallsBackToLatestShare(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://blog.example/post", Comment: "great read", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), shares, newFakeDiscoverHiddenReader(), noTrendingFollows(), noTrendingEligibility(), nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].TastePreview == nil {
		t.Fatalf("got = %+v, want a taste preview from the latest share", got)
	}
	if got[0].TastePreview.LatestShareItemURL != "https://blog.example/post" {
		t.Errorf("TastePreview.LatestShareItemURL = %q, want https://blog.example/post", got[0].TastePreview.LatestShareItemURL)
	}
}

func TestDiscoverPeopleHandler_SelfNeverSuggested(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	readerFollows := &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{
		"did:plc:friend": {{DID: "did:plc:me"}, {DID: "did:plc:carol"}},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
	}}
	h := newDiscoverPeopleHandler(follows, noAdjacentFollows(), readerFollows, &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:carol" {
		t.Fatalf("got = %+v, want only carol (self excluded)", got)
	}
}

func TestDiscoverPeopleHandler_SameCursor_IdenticalOrder(t *testing.T) {
	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < 6; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{{Key: "https://feed" + string(rune('a'+i)) + ".example/f", Kind: "rss"}}
	}
	cursor, err := encodeDiscoverCursor(discoverCursor{
		Version:  1,
		Kind:     "people",
		Seed:     1234,
		RankedAt: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}

	run := func() []DiscoverPersonWire {
		adjacent := &fakeAdjacentFollowCrawler{follows: adjacentFollows}
		crawler := &fakeSubscriptionCrawler{byDID: byDID}
		h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())
		req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
		query := req.URL.Query()
		query.Set("cursor", cursor)
		req.URL.RawQuery = query.Encode()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var got discoverPersonWires
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
		if first[i].DID != second[i].DID {
			t.Errorf("order differs at [%d]: %q != %q (same cursor must be stable)", i, first[i].DID, second[i].DID)
		}
	}
}

func TestDiscoverPeopleHandler_AdjacentCrawlFailureDegradesGracefully(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	readerFollows := &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{
		"did:plc:friend": {{DID: "did:plc:carol"}},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
	}}
	adjacent := &fakeAdjacentFollowCrawler{err: errors.New("own pds unreachable")}
	h := newDiscoverPeopleHandler(follows, adjacent, readerFollows, &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the adjacent-graph crawl failing", rr.Code)
	}
	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:carol" {
		t.Fatalf("got = %+v, want the one-hop suggestion despite the adjacent-graph failure", got)
	}
}

// --- Save-privacy invariant. SPEC <saving-sharing>: saves are anonymous-batch-only, never crawled or scored personally. ---

func TestDiscoverPeopleHandler_CandidateWithOnlySaveRecordsNeverSuggested(t *testing.T) {
	// There is no PersonalSaveCrawler to wire in any more, so a candidate whose only real-world record is a save has nothing left to surface: every other crawl reports empty.
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:alice", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), noTrendingFollows(), noTrendingEligibility(), nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want a candidate whose only reader-network record is a save never suggested", got)
	}
}

// --- Unified delivery: personal cards then trending-only cards. SPEC <discovery> People. ---

func TestDiscoverPeopleHandler_TrendingFlagTrue_WhenPersonalCandidateAboveBar(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	readerFollows := &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{
		"did:plc:friend": {{DID: "did:plc:both"}},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:both": {{Key: "https://both.example/feed", Kind: "rss"}},
	}}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:both")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:both")}}
	h := DiscoverPeopleHandler(follows, noAdjacentFollows(), readerFollows, &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || !got[0].Reason.Trending {
		t.Fatalf("got = %+v, want the personal card flagged trending", got)
	}
	if got[0].Reason.FollowedByDID != "did:plc:friend" {
		t.Errorf("Reason.FollowedByDID = %q, want did:plc:friend (the personal reason must survive alongside the trending flag)", got[0].Reason.FollowedByDID)
	}
}

func TestDiscoverPeopleHandler_TrendingFlagFalse_WhenBelowBarOrAbsent(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:onlypersonal", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:onlypersonal": {{Key: "https://onlypersonal.example/feed", Kind: "rss"}},
	}}
	// Only 2 distinct followers: below MinDistinctRepos, so the flag must not fire even though the fake bypasses the SQL-level bar.
	belowBar := &fakeDiscoverTrendingFollowsReader{rows: []db.DiscoverTrendingFollow{
		trendingFollow("did:plc:repo1", "did:plc:onlypersonal"),
		trendingFollow("did:plc:repo2", "did:plc:onlypersonal"),
	}}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:onlypersonal")}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), belowBar, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Reason.Trending {
		t.Fatalf("got = %+v, want the personal card not flagged trending (only 2 distinct followers)", got)
	}

	// Absent from the aggregate entirely.
	h2 := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), noTrendingFollows(), noTrendingEligibility(), nil)
	req2 := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr2 := httptest.NewRecorder()
	h2.ServeHTTP(rr2, req2)
	var got2 discoverPersonWires
	if err := json.Unmarshal(rr2.Body.Bytes(), &got2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got2) != 1 || got2[0].Reason.Trending {
		t.Fatalf("got = %+v, want the personal card not flagged trending (absent from the aggregate)", got2)
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyCardsAppendedAfterPersonalCards(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:personal", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:personal": {{Key: "https://personal.example/feed", Kind: "rss"}},
	}}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:trending-only")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:trending-only")}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v, want 1 personal card + 1 trending-only card", got)
	}
	if got[0].DID != "did:plc:personal" {
		t.Errorf("got[0] = %+v, want the personal card first", got[0])
	}
	if got[1].DID != "did:plc:trending-only" || !got[1].Reason.Trending {
		t.Errorf("got[1] = %+v, want the trending-only card appended last", got[1])
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyReasonIsEmptyExceptTheFlag(t *testing.T) {
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:trending-only")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:trending-only")}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
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
	if r.BlueskyFollow || r.TangledFollow || r.FollowedByDID != "" || r.SharedSourceCount != 0 {
		t.Errorf("Reason = %+v, want everything but Trending zero-valued", r)
	}
	if got[0].TastePreview != nil {
		t.Errorf("TastePreview = %+v, want nil (no subscribe/author titles in the fixture)", got[0].TastePreview)
	}
}

func TestDiscoverPeopleHandler_PersonWithBothPersonalAndTrendingAggregate_AppearsExactlyOnce(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:friend")}}
	readerFollows := &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{
		"did:plc:friend": {{DID: "did:plc:both"}},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:both": {{Key: "https://both.example/feed", Kind: "rss"}},
	}}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:both")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:both")}}
	h := DiscoverPeopleHandler(follows, noAdjacentFollows(), readerFollows, &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want exactly one card (personal, not duplicated as trending-only)", got)
	}
	if got[0].Reason.FollowedByDID != "did:plc:friend" || !got[0].Reason.Trending {
		t.Errorf("got[0].Reason = %+v, want the personal reason preserved plus the trending flag", got[0].Reason)
	}
}

func TestDiscoverPeopleHandler_ColdStartZeroCandidates_StillReturnsTrendingOnlyCards(t *testing.T) {
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:trending-only")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:trending-only")}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:new-user", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:trending-only" {
		t.Fatalf("got = %+v, want the trending-only card for a zero-candidate account (cold start must not early-return empty)", got)
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyExcludesAlreadyFollowed(t *testing.T) {
	// Followed people never become personal candidates at all (addCandidate filters them upstream), so they need their own explicit exclusion from trending-only.
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:already-followed")}}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:already-followed")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:already-followed")}}
	h := DiscoverPeopleHandler(follows, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the already-followed person excluded from trending-only", got)
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyExcludesSelf(t *testing.T) {
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:me")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:me")}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the viewer's own DID never suggested as trending-only", got)
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyExcludesHidden(t *testing.T) {
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers("did:plc:hidden-person")}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal("did:plc:hidden-person")}}
	hides := newFakeDiscoverHiddenReader()
	hides.hide("did:plc:me", "person", "did:plc:hidden-person")
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), hides, trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want the hidden person excluded from trending-only", got)
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyExcludesFirstPageCutPersonalCandidate(t *testing.T) {
	// 8 candidates with two subscribe signals each (score ~2.3, high band) versus 1 candidate with a single subscribe signal (score ~1.15, several bands lower): a coarse activity-count gap, not a fine recency gap, so the first-page cut is deterministic (the shuffle only reorders within a band).
	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < 8; i++ {
		did := "did:plc:p" + string(rune('0'+i))
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{
			{Key: "https://feed" + string(rune('0'+i)) + "a.example/f", Kind: "rss"},
			{Key: "https://feed" + string(rune('0'+i)) + "b.example/f", Kind: "rss"},
		}
	}
	cutDID := "did:plc:p8" // single subscribe signal: lowest score, deferred to the next page
	adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: cutDID, Network: "bluesky"})
	byDID[cutDID] = []discovercrawl.Subscription{{Key: "https://feed8.example/f", Kind: "rss"}}
	adjacent := &fakeAdjacentFollowCrawler{follows: adjacentFollows}
	crawler := &fakeSubscriptionCrawler{byDID: byDID}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers(cutDID)}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal(cutDID)}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("len = %d, want an eight-item page", len(got))
	}
	for _, w := range got {
		if w.DID == cutDID {
			t.Fatalf("got = %+v, want %q deferred as a personal card rather than resurfacing as trending-only", got, cutDID)
		}
	}
}

func TestDiscoverPeopleHandler_TrendingFollowsReadFailure_DegradesToPersonalOnly(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:good", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:good": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{err: errors.New("db down")}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, noTrendingEligibility(), nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the trending-follows read failing", rr.Code)
	}
	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:good" || got[0].Reason.Trending {
		t.Fatalf("got = %+v, want only the personal card, untrended", got)
	}
}

func TestDiscoverPeopleHandler_TrendingSignalsReadFailure_DegradesToPersonalOnly(t *testing.T) {
	adjacent := &fakeAdjacentFollowCrawler{follows: []discovercrawl.AdjacentFollow{
		{DID: "did:plc:good", Network: "bluesky"},
	}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
		"did:plc:good": {{Key: "https://good.example/feed", Kind: "rss"}},
	}}
	signals := &fakeDiscoverTrendingSignalsReader{err: errors.New("db down")}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), noTrendingFollows(), signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite the trending-signals read failing", rr.Code)
	}
	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != "did:plc:good" || got[0].Reason.Trending {
		t.Fatalf("got = %+v, want only the personal card, untrended", got)
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyTastePreviewFromEligibilitySignals(t *testing.T) {
	did := "did:plc:trending-only"
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers(did)}
	rows := []db.DiscoverTrendingSignal{
		trendingSignal(did, "https://old-sub.example/feed", "rss", "subscribe"),
		trendingSignal(did, "https://new-author.example/feed", "standardfeed", "author"),
		trendingSignal(did, "https://newest-sub.example/feed", "rss", "subscribe"),
		trendingSignal(did, "https://excluded-share.example/feed", "rss", "share"), // never a taste-preview title
	}
	rows[0].SignalAt = strPtr("2026-06-01T00:00:00Z")
	rows[0].Title = strPtr("Old Sub")
	rows[1].SignalAt = strPtr("2026-06-15T00:00:00Z")
	rows[1].Title = strPtr("New Author")
	rows[2].SignalAt = strPtr("2026-07-01T00:00:00Z")
	rows[2].Title = strPtr("Newest Sub")
	rows[3].SignalAt = strPtr("2026-07-05T00:00:00Z")
	rows[3].Title = strPtr("Excluded Share")
	signals := &fakeDiscoverTrendingSignalsReader{rows: append([]db.DiscoverTrendingSignal{eligibilitySignal(did)}, rows...)}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 trending-only card", got)
	}
	if got[0].TastePreview == nil {
		t.Fatalf("TastePreview = nil, want titles from the eligibility read")
	}
	gotTitles := got[0].TastePreview.Titles
	if len(gotTitles) != 3 {
		t.Fatalf("TastePreview.Titles = %v, want 3 (capped at maxTastePreviewTitles, share excluded)", gotTitles)
	}
	wantOrder := []string{"Newest Sub", "New Author", "Old Sub"}
	for i, title := range wantOrder {
		if gotTitles[i] != title {
			t.Errorf("TastePreview.Titles[%d] = %q, want %q (newest first)", i, gotTitles[i], title)
		}
	}
	for _, title := range gotTitles {
		if title == "Excluded Share" {
			t.Errorf("TastePreview.Titles = %v, want share-signal titles excluded", gotTitles)
		}
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyNeverCrawlsCandidates(t *testing.T) {
	did := "did:plc:trending-only"
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: threeFollowers(did)}
	signals := &fakeDiscoverTrendingSignalsReader{rows: []db.DiscoverTrendingSignal{eligibilitySignal(did)}}
	crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}
	authored := &fakeAuthoredPublicationCrawler{byDID: map[string][]discovercrawl.AuthoredPublication{}}
	shares := &fakePersonalShareCrawler{byDID: map[string][]discovercrawl.Share{}}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, authored, shares, newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].DID != did {
		t.Fatalf("got = %+v, want the trending-only card", got)
	}
	for _, calls := range [][]string{crawler.calls, authored.calls, shares.calls} {
		for _, c := range calls {
			if c == did {
				t.Errorf("crawl calls = %v, want the trending-only candidate never personally crawled", calls)
			}
		}
	}
}

func TestDiscoverPeopleHandler_TrendingOnlyCapsAtEight(t *testing.T) {
	var followRows []db.DiscoverTrendingFollow
	var signalRows []db.DiscoverTrendingSignal
	for i := 0; i < 10; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		followRows = append(followRows, threeFollowers(did)...)
		signalRows = append(signalRows, eligibilitySignal(did))
	}
	trendingFollows := &fakeDiscoverTrendingFollowsReader{rows: followRows}
	signals := &fakeDiscoverTrendingSignalsReader{rows: signalRows}
	h := DiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, noAdjacentFollows(), noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{}}, noAuthoredPublications(), noPersonalShares(), newFakeDiscoverHiddenReader(), trendingFollows, signals, nil)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got discoverPersonWires
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}
}

// --- Cold-cache fan-out concurrency bound ---

func TestDiscoverPeopleHandler_CrawlFanOutIsBoundedAndOverlaps(t *testing.T) {
	const numCandidates = 20 // must exceed discoverCrawlFanoutLimit to observe saturation
	var adjacentFollows []discovercrawl.AdjacentFollow
	byDID := map[string][]discovercrawl.Subscription{}
	for i := 0; i < numCandidates; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		adjacentFollows = append(adjacentFollows, discovercrawl.AdjacentFollow{DID: did, Network: "bluesky"})
		byDID[did] = []discovercrawl.Subscription{{Key: "https://feed" + string(rune('a'+i)) + ".example/f", Kind: "rss"}}
	}
	adjacent := &fakeAdjacentFollowCrawler{follows: adjacentFollows}

	tracker := newInFlightTracker(discoverCrawlFanoutLimit)
	release := make(chan struct{})
	crawler := &gatedSubscriptionCrawler{tracker: tracker, release: release, byDID: byDID}

	h := newDiscoverPeopleHandler(&fakeDiscoverFollowsReader{}, adjacent, noReaderNetworkFollows(), &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
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

// TestDiscoverPeopleHandler_FollowedByTieBreakIsDeterministic pins the batched one-hop read to sorted friend order: two friends follow the same candidate, and the alphabetically-first friend must be credited on every run.
func TestDiscoverPeopleHandler_FollowedByTieBreakIsDeterministic(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:zoe"),
		discoverFollow("did:plc:alice"),
	}}
	readerFollows := &fakeReaderNetworkFollowCrawler{byDID: map[string][]discovercrawl.ReaderNetworkFollow{
		"did:plc:zoe":   {{DID: "did:plc:carol"}},
		"did:plc:alice": {{DID: "did:plc:carol"}},
	}}

	for run := 0; run < 5; run++ {
		crawler := &fakeSubscriptionCrawler{byDID: map[string][]discovercrawl.Subscription{
			"did:plc:carol": {{Key: "https://carol.example/feed", Kind: "rss"}},
		}}
		h := newDiscoverPeopleHandler(follows, noAdjacentFollows(), readerFollows, &fakeDiscoverSubsReader{}, crawler, newFakeDiscoverHiddenReader())

		req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/people", nil), "did:plc:me", "sess1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		var got discoverPersonWires
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("run %d: unmarshal: %v", run, err)
		}
		if len(got) != 1 || got[0].DID != "did:plc:carol" {
			t.Fatalf("run %d: got = %+v, want carol", run, got)
		}
		if got[0].Reason.FollowedByDID != "did:plc:alice" {
			t.Fatalf("run %d: Reason.FollowedByDID = %q, want did:plc:alice on every run", run, got[0].Reason.FollowedByDID)
		}
	}
}

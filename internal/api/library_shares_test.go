package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/sharemeta"
)

type fakeShareCrawlerForAPI struct {
	mu    sync.Mutex
	byDID map[string][]discovercrawl.Share
	err   map[string]error
	calls []string
}

// FetchShares runs concurrently across the fan-out, so calls must be appended under lock.
func (f *fakeShareCrawlerForAPI) FetchShares(_ context.Context, did syntax.DID) ([]discovercrawl.Share, error) {
	f.mu.Lock()
	f.calls = append(f.calls, did.String())
	f.mu.Unlock()
	if err, ok := f.err[did.String()]; ok {
		return nil, err
	}
	return f.byDID[did.String()], nil
}

// gatedShareCrawlerForAPI blocks every FetchShares call until release closes, letting a test pin how many crawls the fan-out bound admits in flight.
type gatedShareCrawlerForAPI struct {
	tracker *inFlightTracker
	release chan struct{}
	byDID   map[string][]discovercrawl.Share
}

func (f *gatedShareCrawlerForAPI) FetchShares(ctx context.Context, did syntax.DID) ([]discovercrawl.Share, error) {
	f.tracker.enter()
	defer f.tracker.leave()
	select {
	case <-f.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return f.byDID[did.String()], nil
}

func TestLibraryNetworkSharesHandler_NoFollows_ReturnsEmptyWithoutCrawling(t *testing.T) {
	crawler := &fakeShareCrawlerForAPI{byDID: map[string][]discovercrawl.Share{}}
	h := LibraryNetworkSharesHandler(&fakeDiscoverFollowsReader{}, crawler, noShareMetadata())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got []NetworkShareWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
	if len(crawler.calls) != 0 {
		t.Errorf("crawler.calls = %v, want none (no follows must not crawl)", crawler.calls)
	}
}

func TestLibraryNetworkSharesHandler_RendersFollowedPersonsShares(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeShareCrawlerForAPI{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://a.example/post", Comment: "read this", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	metadata := noShareMetadata()
	metadata.byKey["https://a.example/post"] = sharemeta.Metadata{
		Title: "Network article", TargetURL: "https://a.example/final", EntrySlug: "network-entry",
	}
	h := LibraryNetworkSharesHandler(follows, crawler, metadata)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got []NetworkShareWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got = %+v, want 1 share", got)
	}
	if got[0].SharerDID != "did:plc:alice" || got[0].ItemURL != "https://a.example/post" || got[0].Comment != "read this" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].Title != "Network article" || got[0].TargetURL != "https://a.example/final" || got[0].EntrySlug != "network-entry" {
		t.Errorf("metadata = %+v", got[0])
	}
}

func TestLibraryNetworkSharesHandler_MergesRecommendPlusSidecarAsOneEntry(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	document := "at://did:plc:pub/site.standard.document/1"
	crawler := &fakeShareCrawlerForAPI{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "standardfeed", Document: document, ItemURL: "https://pub.example/a", Comment: "worth it", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	h := LibraryNetworkSharesHandler(follows, crawler, noShareMetadata())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got []NetworkShareWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Kind != "standardfeed" || got[0].Document != document || got[0].Comment != "worth it" {
		t.Fatalf("got = %+v, want one merged standardfeed entry with the sidecar's comment", got)
	}
}

func TestLibraryNetworkSharesHandler_SkyreaderShareRendersLikeMorgenblauShare(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{discoverFollow("did:plc:alice")}}
	crawler := &fakeShareCrawlerForAPI{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "skyreader", ItemURL: "https://sky.example/post", CreatedAt: "2026-07-01T00:00:00Z"}},
	}}
	h := LibraryNetworkSharesHandler(follows, crawler, noShareMetadata())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got []NetworkShareWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Kind != "skyreader" || got[0].ItemURL != "https://sky.example/post" {
		t.Fatalf("got = %+v, want the skyreader share rendered the same shape", got)
	}
}

func TestLibraryNetworkSharesHandler_SortsNewestFirstAcrossFollowedPeople(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:alice"),
		discoverFollow("did:plc:bob"),
	}}
	crawler := &fakeShareCrawlerForAPI{byDID: map[string][]discovercrawl.Share{
		"did:plc:alice": {{Kind: "rss", ItemURL: "https://old.example/post", CreatedAt: "2026-07-01T00:00:00Z"}},
		"did:plc:bob":   {{Kind: "rss", ItemURL: "https://new.example/post", CreatedAt: "2026-07-05T00:00:00Z"}},
	}}
	h := LibraryNetworkSharesHandler(follows, crawler, noShareMetadata())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got []NetworkShareWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0].ItemURL != "https://new.example/post" || got[1].ItemURL != "https://old.example/post" {
		t.Fatalf("got = %+v, want newest first", got)
	}
}

func TestLibraryNetworkSharesHandler_OneBrokenRepoDoesNotFailWholeRequest(t *testing.T) {
	follows := &fakeDiscoverFollowsReader{rows: []db.UserFollow{
		discoverFollow("did:plc:broken"),
		discoverFollow("did:plc:alice"),
	}}
	crawler := &fakeShareCrawlerForAPI{
		byDID: map[string][]discovercrawl.Share{
			"did:plc:alice": {{Kind: "rss", ItemURL: "https://good.example/post", CreatedAt: "2026-07-01T00:00:00Z"}},
		},
		err: map[string]error{"did:plc:broken": errors.New("pds unreachable")},
	}
	h := LibraryNetworkSharesHandler(follows, crawler, noShareMetadata())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 despite one broken repo", rr.Code)
	}
	var got []NetworkShareWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].ItemURL != "https://good.example/post" {
		t.Fatalf("got = %+v, want the good repo's share despite the broken one", got)
	}
}

// --- Cold-cache fan-out concurrency bound ---

func TestLibraryNetworkSharesHandler_CrawlFanOutIsBoundedAndOverlaps(t *testing.T) {
	const numFollows = 20 // must exceed discoverCrawlFanoutLimit to observe saturation
	var rows []db.UserFollow
	byDID := map[string][]discovercrawl.Share{}
	for i := 0; i < numFollows; i++ {
		did := "did:plc:p" + string(rune('a'+i))
		rows = append(rows, discoverFollow(did))
		byDID[did] = []discovercrawl.Share{{Kind: "rss", ItemURL: "https://feed" + string(rune('a'+i)) + ".example/post", CreatedAt: "2026-07-01T00:00:00Z"}}
	}
	follows := &fakeDiscoverFollowsReader{rows: rows}

	tracker := newInFlightTracker(discoverCrawlFanoutLimit)
	release := make(chan struct{})
	crawler := &gatedShareCrawlerForAPI{tracker: tracker, release: release, byDID: byDID}

	h := LibraryNetworkSharesHandler(follows, crawler, noShareMetadata())

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/library/network-shares", nil), "did:plc:me", "sess1")
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

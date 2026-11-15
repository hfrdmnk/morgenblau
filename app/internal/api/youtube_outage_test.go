package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/websource"
)

const (
	testYouTubeFeed = "https://www.youtube.com/feeds/videos.xml?channel_id=UCexampleexampleexample1"
	testBlogFeed    = "https://publication.example.com/feed.xml"
)

func listOutageFlags(t *testing.T, failing, total int64) map[string]SubscriptionWire {
	t.Helper()
	idx := newFakeIndex()
	idx.youtubeFeeds = db.CountYouTubeFeedFailuresRow{Total: total, Failing: failing}
	healthyYouTube := "https://www.youtube.com/feeds/videos.xml?channel_id=UCexampleexampleexample2"
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{
		testYouTubeFeed: {Did: "did:plc:alice", Rkey: "1", FeedUrl: testYouTubeFeed},
		testBlogFeed:    {Did: "did:plc:alice", Rkey: "2", FeedUrl: testBlogFeed},
		healthyYouTube:  {Did: "did:plc:alice", Rkey: "3", FeedUrl: healthyYouTube},
	}
	idx.consecutiveFailures[testYouTubeFeed] = 2
	idx.consecutiveFailures[testBlogFeed] = 2
	idx.lastFetchedAt[healthyYouTube] = ptrString("2026-11-15T07:00:00Z")

	rr := httptest.NewRecorder()
	SubscriptionsListHandler(idx, websource.FeedPrefixes{}).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	byRkey := map[string]SubscriptionWire{}
	for _, w := range got {
		byRkey[w.Rkey] = w
	}
	return byRkey
}

func TestSubscriptionsList_FlagsFailingYouTubeFeedsDuringAnOutage(t *testing.T) {
	for _, tc := range []struct {
		name           string
		failing, total int64
		outage         bool
	}{
		{"half of the YouTube feeds failing", 3, 6, true},
		{"just under half failing", 3, 7, false},
		{"too few feeds to tell an outage from dead channels", 2, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byRkey := listOutageFlags(t, tc.failing, tc.total)
			if got := byRkey["1"]; got.YouTubeOutage != tc.outage || got.FetchStatus != "unavailable" {
				t.Errorf("failing YouTube feed: outage = %v, status = %q; want outage %v and still unavailable", got.YouTubeOutage, got.FetchStatus, tc.outage)
			}
			if byRkey["2"].YouTubeOutage {
				t.Error("a failing blog is never blamed on YouTube")
			}
			if byRkey["3"].YouTubeOutage {
				t.Error("a YouTube feed that fetches fine is not waiting on YouTube")
			}
		})
	}
}

func TestSubscriptionGet_FlagsAFailingYouTubeFeedDuringAnOutage(t *testing.T) {
	fake := newFakeSourceDetail()
	fake.youtubeFeeds = db.CountYouTubeFeedFailuresRow{Total: 4, Failing: 4}
	fake.rows["did:plc:alice"] = map[string]db.GetUserSourceWithStatsRow{
		"1": {Rkey: "1", FeedUrl: testYouTubeFeed, ConsecutiveFailures: 1},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}", SubscriptionGetHandler(fake, websource.FeedPrefixes{}))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/1", nil), "did:plc:alice", "sid-1"))
	if !strings.Contains(rr.Body.String(), `"youtubeOutage":true`) {
		t.Errorf("status = %d, body = %s; want the outage flag", rr.Code, rr.Body.String())
	}
}

func TestSubscriptionsPreview_NamesTheYouTubeOutage(t *testing.T) {
	outage := newFakeIndex()
	outage.youtubeFeeds = db.CountYouTubeFeedFailuresRow{Total: 5, Failing: 5}
	for _, tc := range []struct {
		name   string
		health YouTubeFeedHealth
		feed   string
		code   string
	}{
		{"YouTube feed during an outage", outage, testYouTubeFeed, codeYouTubeDown},
		{"other feed during an outage", outage, testBlogFeed, codeUpstreamError},
		{"YouTube feed without an outage", newFakeIndex(), testYouTubeFeed, codeUpstreamError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := SubscriptionsPreviewHandler(&fakeFeedFetcher{err: errors.New("404 Not Found")}, fakeDocLister{}, tc.health)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/preview", strings.NewReader(`{"feedUrl":"`+tc.feed+`"}`)), "did:plc:alice", "sid-1"))
			var got errorEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if rr.Code != http.StatusBadGateway || got.Code != tc.code {
				t.Errorf("status = %d, code = %q; want 502 %q", rr.Code, got.Code, tc.code)
			}
		})
	}
}

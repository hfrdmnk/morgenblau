package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
)

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

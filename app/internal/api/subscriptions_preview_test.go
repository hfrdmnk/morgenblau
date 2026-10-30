package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/database/db"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/socialspec"
	"morgenblau/internal/standardfeed"
)

type fakeFeedFetcher struct {
	feed *gofeed.Feed
	err  error
	got  string
}

func (f *fakeFeedFetcher) Fetch(_ context.Context, rawURL string, _ fetcher.FeedState) (*fetcher.Result, error) {
	f.got = rawURL
	return &fetcher.Result{Feed: f.feed}, f.err
}

type fakeDocLister struct {
	docs   []standardfeed.Document
	pubURL string
}

func (f fakeDocLister) ListDocuments(context.Context, string) ([]standardfeed.Document, error) {
	return f.docs, nil
}

func (f fakeDocLister) GetPublication(_ context.Context, uri string) (*standardfeed.Publication, error) {
	return &standardfeed.Publication{URI: uri, URL: f.pubURL}, nil
}

func day(d int) *time.Time {
	t := time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC)
	return &t
}

func preview(t *testing.T, h http.Handler, body string) (int, previewResponse) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/preview", strings.NewReader(body)), "did:plc:alice", "sid-1"))
	var got previewResponse
	if rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
	}
	return rr.Code, got
}

func TestSubscriptionsPreview_NewestThreeFeedItems(t *testing.T) {
	feeds := &fakeFeedFetcher{feed: &gofeed.Feed{Items: []*gofeed.Item{
		{Title: "Undated"},
		{Title: "Second", PublishedParsed: day(20), Link: "https://blog.example.com/second", Description: "Teaser", Content: `<p>Full <script>alert(1)</script>text</p>`},
		{Title: "Oldest", PublishedParsed: day(2)},
		{Title: "Newest", UpdatedParsed: day(27), Description: "<p>Only a summary</p>"},
		{Title: "Third", PublishedParsed: day(9)},
	}}}
	code, got := preview(t, SubscriptionsPreviewHandler(feeds, fakeDocLister{}), `{"feedUrl":"https://blog.example.com/feed.xml"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if feeds.got != "https://blog.example.com/feed.xml" {
		t.Errorf("fetched %q", feeds.got)
	}
	want := []previewItem{
		{Title: "Newest", PublishedAt: "2026-10-27T09:00:00Z", ContentHTML: "<p>Only a summary</p>"},
		{Title: "Second", PublishedAt: "2026-10-20T09:00:00Z", URL: "https://blog.example.com/second", ContentHTML: "<p>Full text</p>"},
		{Title: "Third", PublishedAt: "2026-10-09T09:00:00Z"},
	}
	if !reflect.DeepEqual(got.Items, want) {
		t.Errorf("items = %+v, want %+v", got.Items, want)
	}
}

func TestSubscriptionsPreview_NewestThreePublicationDocuments(t *testing.T) {
	docs := fakeDocLister{pubURL: "https://publication.example.com", docs: []standardfeed.Document{
		{Title: "Old", PublishedAt: "2026-09-01T00:00:00Z", TextContent: "Plain body"},
		{Title: "New", PublishedAt: "2026-10-28T00:00:00Z", Path: "/posts/new", Description: "Summary", TextContent: "Ignored"},
		{Title: "Mid", PublishedAt: "2026-10-01T00:00:00Z"},
		{Title: "Older", PublishedAt: "2026-08-01T00:00:00Z"},
	}}
	code, got := preview(t, SubscriptionsPreviewHandler(&fakeFeedFetcher{}, docs), `{"publication":"at://did:plc:example/site.standard.publication/self"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	var titles []string
	for _, item := range got.Items {
		titles = append(titles, item.Title)
	}
	if !reflect.DeepEqual(titles, []string{"New", "Mid", "Old"}) {
		t.Errorf("titles = %v", titles)
	}
	if got.Items[0].URL != "https://publication.example.com/posts/new" || got.Items[0].ContentHTML != "Summary" {
		t.Errorf("newest = %+v, want the publication URL joined with its path and the description as body", got.Items[0])
	}
	if got.Items[2].URL != "" || got.Items[2].ContentHTML != "Plain body" {
		t.Errorf("oldest = %+v, want no URL without a path and textContent as body", got.Items[2])
	}
}

func TestSubscriptionsPreview_EmptyFeedIsAnEmptyList(t *testing.T) {
	code, got := preview(t, SubscriptionsPreviewHandler(&fakeFeedFetcher{feed: &gofeed.Feed{}}, fakeDocLister{}), `{"feedUrl":"https://quiet.example.com/feed.xml"}`)
	if code != http.StatusOK || got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("status = %d, items = %#v; want 200 with []", code, got.Items)
	}
}

func TestSubscriptionsPreview_Rejects(t *testing.T) {
	h := SubscriptionsPreviewHandler(&fakeFeedFetcher{err: errors.New("down")}, fakeDocLister{})
	for body, want := range map[string]int{
		`{}`: http.StatusBadRequest,
		`{"feedUrl":"https://a.example.com/f","publication":"at://did:plc:x/site.standard.publication/y"}`: http.StatusBadRequest,
		`{"feedUrl":"https://down.example.com/feed.xml"}`:                                                  http.StatusBadGateway,
	} {
		if code, _ := preview(t, h, body); code != want {
			t.Errorf("%s: status = %d, want %d", body, code, want)
		}
	}
}

func TestSubscriptionsProfile_MarksFollowedFeeds(t *testing.T) {
	const followed = "https://avery.example.com/feed.xml"
	idx := newFakeIndex()
	idx.rows["did:plc:alice"] = map[string]db.UserSubscription{followed: {Did: "did:plc:alice", Rkey: "avery-rkey", FeedUrl: followed}}
	var origin string
	fetch := func(_ context.Context, o string) (*socialspec.Profile, error) {
		origin = o
		return &socialspec.Profile{Name: "Jordan Doe", Following: []socialspec.Follow{
			{Title: "Avery", FeedURL: followed},
			{Title: "Example Publication", FeedURL: "https://publication.example.com/rss"},
		}}, nil
	}
	rr := httptest.NewRecorder()
	SubscriptionsProfileHandler(idx, fetch, nil).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/profile",
		strings.NewReader(`{"url":"jordan.example.com/posts/1"}`)), "did:plc:alice", "sid-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if origin != "https://jordan.example.com" {
		t.Errorf("origin = %q", origin)
	}
	var got profileResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Profile == nil || got.Profile.Name != "Jordan Doe" || len(got.Profile.Following) != 2 {
		t.Fatalf("profile = %+v", got.Profile)
	}
	if got.Profile.Following[0].Rkey != "avery-rkey" || got.Profile.Following[1].Rkey != "" {
		t.Errorf("rkeys = %q, %q; want avery-rkey and none", got.Profile.Following[0].Rkey, got.Profile.Following[1].Rkey)
	}
}

func TestSubscriptionsProfile_NoProfileIsNull(t *testing.T) {
	fetch := func(context.Context, string) (*socialspec.Profile, error) { return nil, nil }
	rr := httptest.NewRecorder()
	SubscriptionsProfileHandler(newFakeIndex(), fetch, nil).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/profile",
		strings.NewReader(`{"url":"https://quiet.example.com"}`)), "did:plc:alice", "sid-1"))
	if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != `{"profile":null}` {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestSubscriptionsProfile_RejectsNonWebURLs(t *testing.T) {
	fetch := func(context.Context, string) (*socialspec.Profile, error) {
		t.Fatal("fetched a non-web URL")
		return nil, nil
	}
	for _, raw := range []string{"", "ftp://files.example.com", "at://did:plc:example/site.standard.publication/self"} {
		rr := httptest.NewRecorder()
		SubscriptionsProfileHandler(newFakeIndex(), fetch, nil).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/profile",
			strings.NewReader(`{"url":"`+raw+`"}`)), "did:plc:alice", "sid-1"))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", raw, rr.Code)
		}
	}
}

func TestSubscriptionsPreviewArticle_ExtractsTheSanitizedArticle(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/posts/new" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>New</title></head><body><nav>Menu</nav><article><h1>New</h1>
<p>The first paragraph of the post is long enough for readability to keep it as the main content of this page.</p>
<p>A second paragraph carries on with more words, so the article clearly outweighs the navigation.<script>alert(1)</script></p>
</article></body></html>`))
	}))
	defer page.Close()
	h := SubscriptionsPreviewArticleHandler(page.Client())
	post := func(body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodPost, "/api/subscriptions/preview/article", strings.NewReader(body)), "did:plc:alice", "sid-1"))
		return rr
	}

	rr := post(`{"url":"` + page.URL + `/posts/new"}`)
	var got struct {
		ContentHTML string `json:"contentHtml"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); rr.Code != http.StatusOK || err != nil {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(got.ContentHTML, "The first paragraph") || strings.Contains(got.ContentHTML, "<script") {
		t.Errorf("contentHtml = %q, want the article without scripts", got.ContentHTML)
	}

	for body, want := range map[string]int{
		`{"url":"ftp://files.example.com/a"}`:      http.StatusBadRequest,
		`{"url":"` + page.URL + `/posts/missing"}`: http.StatusBadGateway,
	} {
		if rr := post(body); rr.Code != want {
			t.Errorf("%s: status = %d, want %d", body, rr.Code, want)
		}
	}
}

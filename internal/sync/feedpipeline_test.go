package sync

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
)

type fakePipelineQueries struct {
	existing       db.Feed
	getErr         error
	fetchStates    []db.UpdateFeedFetchStateParams
	feeds          []db.UpsertFeedParams
	entries        []db.UpsertFeedEntryParams
	titleUpdates   []db.UpdateUserSubscriptionsTitleByFeedURLParams
	iconUpdates    []db.SetFeedIconURLParams
	upsertFeedErr  error
	upsertEntryErr error
}

func (f *fakePipelineQueries) GetFeed(_ context.Context, _ string) (db.Feed, error) {
	if f.getErr != nil {
		return db.Feed{}, f.getErr
	}
	return f.existing, nil
}

func (f *fakePipelineQueries) UpdateFeedFetchState(_ context.Context, arg db.UpdateFeedFetchStateParams) error {
	f.fetchStates = append(f.fetchStates, arg)
	return nil
}

func (f *fakePipelineQueries) UpsertFeed(_ context.Context, arg db.UpsertFeedParams) error {
	f.feeds = append(f.feeds, arg)
	return f.upsertFeedErr
}

func (f *fakePipelineQueries) UpsertFeedEntry(_ context.Context, arg db.UpsertFeedEntryParams) error {
	f.entries = append(f.entries, arg)
	return f.upsertEntryErr
}

func (f *fakePipelineQueries) UpdateUserSubscriptionsTitleByFeedURL(_ context.Context, arg db.UpdateUserSubscriptionsTitleByFeedURLParams) error {
	f.titleUpdates = append(f.titleUpdates, arg)
	return nil
}

func (f *fakePipelineQueries) SetFeedIconURL(_ context.Context, arg db.SetFeedIconURLParams) error {
	f.iconUpdates = append(f.iconUpdates, arg)
	return nil
}

type fakeFaviconDiscoverer struct {
	sites []string
	icon  string
	err   error
}

func (f *fakeFaviconDiscoverer) Discover(_ context.Context, siteURL string) (string, error) {
	f.sites = append(f.sites, siteURL)
	if f.err != nil {
		return "", f.err
	}
	return f.icon, nil
}

func newTestPipeline(q *fakePipelineQueries, fav *fakeFaviconDiscoverer, now time.Time) *FeedPipeline {
	p := NewFeedPipeline(fetcher.New(fetcher.WithSafeHTTPOptions(safehttp.WithAllowLoopback())), q)
	p.now = func() time.Time { return now }
	p.WithFaviconDiscoverer(fav)
	return p
}

func TestFeedPipeline_FetchAndStore_NotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	fav := &fakeFaviconDiscoverer{icon: "https://example.com/favicon.ico"}
	p := newTestPipeline(q, fav, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.fetchStates) != 1 {
		t.Fatalf("fetch state updates = %d, want 1", len(q.fetchStates))
	}
	if len(q.feeds) != 0 {
		t.Errorf("UpsertFeed calls = %d, want 0", len(q.feeds))
	}
	if len(q.entries) != 0 {
		t.Errorf("UpsertFeedEntry calls = %d, want 0", len(q.entries))
	}
	if len(fav.sites) != 0 {
		t.Errorf("favicon discoveries = %v, want none", fav.sites)
	}
}

func TestFeedPipeline_FetchAndStore_HappyPath(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>Audio item</title>
<link>https://site.example.com/audio</link>
<guid>guid-audio</guid>
<description><![CDATA[<p>Hello <strong>reader</strong></p><script>alert("x")</script>]]></description>
<enclosure url="https://site.example.com/audio.mp3" type="audio/mpeg" />
</item>
<item>
<title>Post item</title>
<link>https://site.example.com/post</link>
<guid>guid-post</guid>
<description><![CDATA[<p>Plain post</p>]]></description>
</item>`))
	defer closeServer()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	fav := &fakeFaviconDiscoverer{icon: "https://site.example.com/favicon.ico"}
	p := newTestPipeline(q, fav, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.feeds) != 1 {
		t.Fatalf("UpsertFeed calls = %d, want 1", len(q.feeds))
	}
	if q.feeds[0].SiteUrl == nil || *q.feeds[0].SiteUrl != "https://site.example.com/" {
		t.Errorf("SiteUrl = %v", q.feeds[0].SiteUrl)
	}
	if len(q.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(q.entries))
	}
	if q.entries[0].ContentType != "podcast" {
		t.Errorf("first content type = %q, want podcast", q.entries[0].ContentType)
	}
	if q.entries[1].ContentType != "blogpost" {
		t.Errorf("second content type = %q, want blogpost", q.entries[1].ContentType)
	}
	if q.entries[0].ContentHtml == nil || strings.Contains(*q.entries[0].ContentHtml, "<script") {
		t.Errorf("first body was not sanitized: %v", q.entries[0].ContentHtml)
	}
	if len(q.titleUpdates) != 1 || q.titleUpdates[0].Title == nil || *q.titleUpdates[0].Title != "Example Feed" {
		t.Errorf("title updates = %+v", q.titleUpdates)
	}
	if len(fav.sites) != 1 || fav.sites[0] != "https://site.example.com/" {
		t.Errorf("favicon sites = %v", fav.sites)
	}
	if len(q.iconUpdates) != 1 || q.iconUpdates[0].IconUrl == nil || *q.iconUpdates[0].IconUrl != "https://site.example.com/favicon.ico" {
		t.Errorf("icon updates = %+v", q.iconUpdates)
	}
}

func TestFeedPipeline_FetchAndStore_FeedWithoutLinkUsesFeedOriginForFavicon(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("", `<item>
<title>Post item</title>
<link>https://example.com/post</link>
<guid>guid-post</guid>
</item>`))
	defer closeServer()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	fav := &fakeFaviconDiscoverer{icon: "https://example.com/favicon.ico"}
	p := newTestPipeline(q, fav, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	wantOrigin := strings.TrimSuffix(feedURL, "/feed.xml")
	if len(fav.sites) != 1 || fav.sites[0] != wantOrigin {
		t.Errorf("favicon sites = %v, want %s", fav.sites, wantOrigin)
	}
}

func TestFeedPipeline_FetchAndStore_ExistingFreshIconSkipsDiscovery(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>Post item</title>
<link>https://site.example.com/post</link>
<guid>guid-post</guid>
</item>`))
	defer closeServer()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	icon := "https://site.example.com/favicon.ico"
	fetchedAt := now.Add(-time.Hour).Format(time.RFC3339)
	q := &fakePipelineQueries{existing: db.Feed{IconUrl: &icon, IconFetchedAt: &fetchedAt}}
	fav := &fakeFaviconDiscoverer{icon: "https://site.example.com/new.ico"}
	p := newTestPipeline(q, fav, now)

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(fav.sites) != 0 {
		t.Errorf("favicon sites = %v, want none", fav.sites)
	}
	if len(q.iconUpdates) != 0 {
		t.Errorf("icon updates = %+v, want none", q.iconUpdates)
	}
}

func TestFeedPipeline_FetchAndStore_SanitizerStripsScriptTags(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>Post item</title>
<link>https://site.example.com/post</link>
<guid>guid-post</guid>
<description><![CDATA[<p>Safe</p><script>alert("x")</script>]]></description>
</item>`))
	defer closeServer()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	fav := &fakeFaviconDiscoverer{}
	p := newTestPipeline(q, fav, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(q.entries))
	}
	if q.entries[0].ContentHtml == nil {
		t.Fatal("ContentHtml was nil")
	}
	if strings.Contains(*q.entries[0].ContentHtml, "<script") {
		t.Fatalf("body still contains script tag: %s", *q.entries[0].ContentHtml)
	}
}

func serveFeed(t *testing.T, body string) (string, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	return srv.URL + "/feed.xml", srv.Close
}

func rssWithLink(link string, items string) string {
	linkXML := ""
	if link != "" {
		linkXML = "<link>" + link + "</link>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Example Feed</title>` + linkXML + `
<description>Example</description>
` + items + `
</channel></rss>`
}

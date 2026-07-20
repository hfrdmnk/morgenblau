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
	iconUpdates    []db.SetFeedIconURLParams
	failures       []db.UpdateFeedFetchFailureParams
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

func (f *fakePipelineQueries) SetFeedIconURL(_ context.Context, arg db.SetFeedIconURLParams) error {
	f.iconUpdates = append(f.iconUpdates, arg)
	return nil
}

func (f *fakePipelineQueries) UpdateFeedFetchFailure(_ context.Context, arg db.UpdateFeedFetchFailureParams) error {
	f.failures = append(f.failures, arg)
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
	if len(q.failures) != 0 {
		t.Errorf("failure records = %d, want 0 for a 304", len(q.failures))
	}
}

func TestFeedPipeline_FetchAndStore_HappyPath(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", `<item>
<title>Video item</title>
<link>https://site.example.com/video</link>
<guid>guid-video</guid>
<description><![CDATA[<p>Hello <strong>reader</strong></p><script>alert("x")</script>]]></description>
<enclosure url="https://site.example.com/video.mp4" type="video/mp4" />
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
	if q.entries[0].ContentType != "video" {
		t.Errorf("first content type = %q, want video", q.entries[0].ContentType)
	}
	if q.entries[1].ContentType != "blogpost" {
		t.Errorf("second content type = %q, want blogpost", q.entries[1].ContentType)
	}
	if q.entries[0].ContentHtml == nil || strings.Contains(*q.entries[0].ContentHtml, "<script") {
		t.Errorf("first body was not sanitized: %v", q.entries[0].ContentHtml)
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

func TestFeedPipeline_FetchAndStore_DetectsLanguageFromContent(t *testing.T) {
	frenchItem := `<item>
<title>Article</title>
<link>https://site.example.com/a</link>
<guid>guid-a</guid>
<description><![CDATA[Le rapide renard brun saute par-dessus le chien paresseux pendant que le soleil se couche lentement derriere les collines lointaines.]]></description>
</item>`
	feedURL, closeServer := serveFeed(t, rssWithLink("https://site.example.com/", frenchItem))
	defer closeServer()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.feeds) != 1 {
		t.Fatalf("UpsertFeed calls = %d, want 1", len(q.feeds))
	}
	if q.feeds[0].Language == nil || *q.feeds[0].Language != "fr" {
		t.Errorf("Language = %v, want fr (detected from item content)", q.feeds[0].Language)
	}
}

func TestFeedPipeline_FetchAndStore_FallsBackToFeedTagWhenContentTooShort(t *testing.T) {
	feedURL, closeServer := serveFeed(t, rssWithLanguage("https://site.example.com/", "en-US", `<item>
<title>Hi</title>
<link>https://site.example.com/a</link>
<guid>guid-a</guid>
<description>Hi</description>
</item>`))
	defer closeServer()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.feeds) != 1 {
		t.Fatalf("UpsertFeed calls = %d, want 1", len(q.feeds))
	}
	if q.feeds[0].Language == nil || *q.feeds[0].Language != "en" {
		t.Errorf("Language = %v, want en (from the feed's own tag, content too short to detect)", q.feeds[0].Language)
	}
}

func TestFeedPipeline_FetchAndStore_ContentWinsOverDisagreeingFeedTag(t *testing.T) {
	frenchItem := `<item>
<title>Article</title>
<link>https://site.example.com/a</link>
<guid>guid-a</guid>
<description><![CDATA[Le rapide renard brun saute par-dessus le chien paresseux pendant que le soleil se couche lentement derriere les collines lointaines.]]></description>
</item>`
	feedURL, closeServer := serveFeed(t, rssWithLanguage("https://site.example.com/", "en-US", frenchItem))
	defer closeServer()

	q := &fakePipelineQueries{getErr: sql.ErrNoRows}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), feedURL); err != nil {
		t.Fatalf("FetchAndStore: %v", err)
	}
	if len(q.feeds) != 1 {
		t.Fatalf("UpsertFeed calls = %d, want 1", len(q.feeds))
	}
	if q.feeds[0].Language == nil || *q.feeds[0].Language != "fr" {
		t.Errorf("Language = %v, want fr (content detection must win over the en-US tag)", q.feeds[0].Language)
	}
}

func TestFeedPipeline_FetchAndStore_RecordsFailureOnFirstError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	q := &fakePipelineQueries{existing: db.Feed{ConsecutiveFailures: 0}}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err == nil {
		t.Fatal("FetchAndStore: want error for a 500 response")
	}
	if len(q.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(q.failures))
	}
	got := q.failures[0]
	if got.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", got.ConsecutiveFailures)
	}
	wantNext := now.Add(5 * time.Minute).Format(time.RFC3339)
	if got.NextFetchAt == nil || *got.NextFetchAt != wantNext {
		t.Errorf("NextFetchAt = %v, want %s", got.NextFetchAt, wantNext)
	}
	if len(q.fetchStates) != 0 {
		t.Errorf("UpdateFeedFetchState calls = %d, want 0", len(q.fetchStates))
	}
	if len(q.feeds) != 0 {
		t.Errorf("UpsertFeed calls = %d, want 0", len(q.feeds))
	}
}

func TestFeedPipeline_FetchAndStore_RecordsFailureOnSubsequentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	q := &fakePipelineQueries{existing: db.Feed{ConsecutiveFailures: 2}}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err == nil {
		t.Fatal("FetchAndStore: want error for a 500 response")
	}
	if len(q.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(q.failures))
	}
	got := q.failures[0]
	if got.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", got.ConsecutiveFailures)
	}
	wantNext := now.Add(time.Hour).Format(time.RFC3339)
	if got.NextFetchAt == nil || *got.NextFetchAt != wantNext {
		t.Errorf("NextFetchAt = %v, want %s", got.NextFetchAt, wantNext)
	}
}

func TestFeedPipeline_FetchAndStore_RetryAfterBeatsLadder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7200")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	q := &fakePipelineQueries{existing: db.Feed{ConsecutiveFailures: 0}}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err == nil {
		t.Fatal("FetchAndStore: want error for a 429 response")
	}
	wantNext := now.Add(2 * time.Hour).Format(time.RFC3339)
	if got := q.failures[0].NextFetchAt; got == nil || *got != wantNext {
		t.Errorf("NextFetchAt = %v, want %s (Retry-After beats the 5min ladder step)", got, wantNext)
	}
}

func TestFeedPipeline_FetchAndStore_RetryAfterClampedToCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "999999")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	q := &fakePipelineQueries{existing: db.Feed{ConsecutiveFailures: 0}}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err == nil {
		t.Fatal("FetchAndStore: want error for a 429 response")
	}
	wantNext := now.Add(24 * time.Hour).Format(time.RFC3339)
	if got := q.failures[0].NextFetchAt; got == nil || *got != wantNext {
		t.Errorf("NextFetchAt = %v, want %s (clamped to the 24h cap)", got, wantNext)
	}
}

func TestFeedPipeline_FetchAndStore_LadderBeatsShortRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	q := &fakePipelineQueries{existing: db.Feed{ConsecutiveFailures: 3}}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err == nil {
		t.Fatal("FetchAndStore: want error for a 429 response")
	}
	wantNext := now.Add(6 * time.Hour).Format(time.RFC3339)
	if got := q.failures[0].NextFetchAt; got == nil || *got != wantNext {
		t.Errorf("NextFetchAt = %v, want %s (ladder beats a short Retry-After)", got, wantNext)
	}
}

func TestFeedPipeline_FetchAndStore_SkipsWhenInBackoff(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	nextFetch := now.Add(10 * time.Minute).Format(time.RFC3339)
	q := &fakePipelineQueries{existing: db.Feed{NextFetchAt: &nextFetch}}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

	if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err != nil {
		t.Fatalf("FetchAndStore: %v, want nil (skip)", err)
	}
	if hits != 0 {
		t.Errorf("HTTP hits = %d, want 0", hits)
	}
	if len(q.failures) != 0 || len(q.fetchStates) != 0 || len(q.feeds) != 0 {
		t.Errorf("writes recorded during a skip: failures=%d fetchStates=%d feeds=%d", len(q.failures), len(q.fetchStates), len(q.feeds))
	}
}

func TestFeedPipeline_FetchAndStore_ProceedsWhenBackoffExpiredOrUnparseable(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		nextFetchAt string
	}{
		{name: "past stamp", nextFetchAt: now.Add(-time.Minute).Format(time.RFC3339)},
		{name: "unparseable stamp", nextFetchAt: "not-a-time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits++
				w.WriteHeader(http.StatusNotModified)
			}))
			defer srv.Close()

			nextFetch := tc.nextFetchAt
			q := &fakePipelineQueries{existing: db.Feed{NextFetchAt: &nextFetch}}
			p := newTestPipeline(q, &fakeFaviconDiscoverer{}, now)

			if err := p.FetchAndStore(context.Background(), srv.URL+"/feed.xml"); err != nil {
				t.Fatalf("FetchAndStore: %v", err)
			}
			if hits != 1 {
				t.Errorf("HTTP hits = %d, want 1 (fetch should proceed)", hits)
			}
		})
	}
}

func TestFeedPipeline_FetchAndStore_CancelledContextNoFailureRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	q := &fakePipelineQueries{}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := p.FetchAndStore(ctx, srv.URL+"/feed.xml"); err == nil {
		t.Fatal("FetchAndStore: want error for a cancelled context")
	}
	if len(q.failures) != 0 {
		t.Errorf("failure records = %d, want 0 for context.Canceled", len(q.failures))
	}
}

func TestFeedPipeline_FetchAndStore_UnreachableServerRecordsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachableURL := srv.URL + "/feed.xml"
	srv.Close() // closed listener: nothing answers this URL

	q := &fakePipelineQueries{}
	p := newTestPipeline(q, &fakeFaviconDiscoverer{}, time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC))

	if err := p.FetchAndStore(context.Background(), unreachableURL); err == nil {
		t.Fatal("FetchAndStore: want error for an unreachable server")
	}
	if len(q.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(q.failures))
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
	return rssWithLanguage(link, "", items)
}

func rssWithLanguage(link, language, items string) string {
	linkXML := ""
	if link != "" {
		linkXML = "<link>" + link + "</link>"
	}
	languageXML := ""
	if language != "" {
		languageXML = "<language>" + language + "</language>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Example Feed</title>` + linkXML + languageXML + `
<description>Example</description>
` + items + `
</channel></rss>`
}

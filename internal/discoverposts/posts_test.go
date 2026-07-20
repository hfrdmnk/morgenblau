package discoverposts

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/database/db"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/standardfeed"
)

type fakeRSSFetcher struct {
	result *fetcher.Result
	err    error
	calls  int
}

func (f *fakeRSSFetcher) Fetch(_ context.Context, _ string, _ fetcher.FeedState) (*fetcher.Result, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type fakeDocLister struct {
	docs      []standardfeed.Document
	err       error
	calls     int
	gotLimits []int
}

func (f *fakeDocLister) ListRecentDocuments(_ context.Context, _ string, limit int) ([]standardfeed.Document, error) {
	f.calls++
	f.gotLimits = append(f.gotLimits, limit)
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
}

// fakePublicationResolutionReader is the discoverposts-local stand-in for the sqlc-generated resolutions reader.
type fakePublicationResolutionReader struct {
	byKey map[string]db.DiscoverPublicationResolution
	err   error
}

func (f *fakePublicationResolutionReader) GetDiscoverPublicationResolutionByCanonicalKey(_ context.Context, canonicalKey *string) (db.DiscoverPublicationResolution, error) {
	if f.err != nil {
		return db.DiscoverPublicationResolution{}, f.err
	}
	if canonicalKey != nil {
		if row, ok := f.byKey[*canonicalKey]; ok {
			return row, nil
		}
	}
	return db.DiscoverPublicationResolution{}, sql.ErrNoRows
}

type fakeFaviconDiscoverer struct {
	url   string
	err   error
	sites []string
}

func (f *fakeFaviconDiscoverer) Discover(_ context.Context, siteURL string) (string, error) {
	f.sites = append(f.sites, siteURL)
	if f.err != nil {
		return "", f.err
	}
	return f.url, nil
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func TestFetchPosts_RSS_NewestFirstCappedAtThree(t *testing.T) {
	older := mustTime(t, "2026-01-01T00:00:00Z")
	newer := mustTime(t, "2026-06-01T00:00:00Z")
	newest := mustTime(t, "2026-07-01T00:00:00Z")
	oldest := mustTime(t, "2025-01-01T00:00:00Z")
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{
		Link: "https://blog.example",
		Items: []*gofeed.Item{
			{Title: "Old", Link: "https://blog.example/old", PublishedParsed: &older},
			{Title: "Newest", Link: "https://blog.example/newest", PublishedParsed: &newest},
			{Title: "Oldest", Link: "https://blog.example/oldest", PublishedParsed: &oldest},
			{Title: "Newer", Link: "https://blog.example/newer", PublishedParsed: &newer},
		},
	}}}
	fav := &fakeFaviconDiscoverer{}
	f := NewFetcher(rss, &fakeDocLister{}).WithFaviconDiscoverer(fav)

	got, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != PreviewCap {
		t.Fatalf("posts = %+v, want %d", got.Posts, PreviewCap)
	}
	wantOrder := []string{"Newest", "Newer", "Old"}
	for i, title := range wantOrder {
		if got.Posts[i].Title != title {
			t.Errorf("posts[%d].Title = %q, want %q", i, got.Posts[i].Title, title)
		}
	}
}

func TestFetchPosts_RSS_SkipsEmptyTitleItems(t *testing.T) {
	at := mustTime(t, "2026-01-01T00:00:00Z")
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{
		Link: "https://blog.example",
		Items: []*gofeed.Item{
			{Title: "", Link: "https://blog.example/untitled", PublishedParsed: &at},
			{Title: "Titled", Link: "https://blog.example/titled", PublishedParsed: &at},
		},
	}}}
	f := NewFetcher(rss, &fakeDocLister{}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != 1 || got.Posts[0].Title != "Titled" {
		t.Fatalf("posts = %+v, want only the titled item", got.Posts)
	}
}

func TestFetchPosts_RSS_UpdatedParsedFallback(t *testing.T) {
	updated := mustTime(t, "2026-03-01T00:00:00Z")
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{
		Link: "https://blog.example",
		Items: []*gofeed.Item{
			{Title: "No pubdate", Link: "https://blog.example/x", UpdatedParsed: &updated},
		},
	}}}
	f := NewFetcher(rss, &fakeDocLister{}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != 1 {
		t.Fatalf("posts = %+v, want 1", got.Posts)
	}
	want := updated.UTC().Format(time.RFC3339)
	if got.Posts[0].PublishedAt != want {
		t.Errorf("PublishedAt = %q, want %q (UpdatedParsed fallback)", got.Posts[0].PublishedAt, want)
	}
}

func TestFetchPosts_RSS_FetchErrorPropagates(t *testing.T) {
	wantErr := errors.New("upstream unreachable")
	rss := &fakeRSSFetcher{err: wantErr}
	f := NewFetcher(rss, &fakeDocLister{})

	_, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestFetchPosts_Standardfeed_NewestFirstCapped(t *testing.T) {
	docs := []standardfeed.Document{
		{Title: "Old", PublishedAt: "2026-01-01T00:00:00Z"},
		{Title: "Newest", PublishedAt: "2026-07-01T00:00:00Z"},
		{Title: "Oldest", PublishedAt: "2025-01-01T00:00:00Z"},
		{Title: "Newer", PublishedAt: "2026-06-01T00:00:00Z"},
	}
	lister := &fakeDocLister{docs: docs}
	f := NewFetcher(&fakeRSSFetcher{}, lister).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), "at://did:plc:example/site.standard.publication/abc")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != PreviewCap {
		t.Fatalf("posts = %+v, want %d", got.Posts, PreviewCap)
	}
	wantOrder := []string{"Newest", "Newer", "Old"}
	for i, title := range wantOrder {
		if got.Posts[i].Title != title {
			t.Errorf("posts[%d].Title = %q, want %q", i, got.Posts[i].Title, title)
		}
	}
}

func TestFetchPosts_Standardfeed_SkipsEmptyTitleOrPublishedAt(t *testing.T) {
	docs := []standardfeed.Document{
		{Title: "", PublishedAt: "2026-01-01T00:00:00Z"},
		{Title: "No date", PublishedAt: ""},
		{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z"},
	}
	lister := &fakeDocLister{docs: docs}
	f := NewFetcher(&fakeRSSFetcher{}, lister).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), "at://did:plc:example/site.standard.publication/abc")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != 1 || got.Posts[0].Title != "Good" {
		t.Fatalf("posts = %+v, want only the complete doc", got.Posts)
	}
}

func TestFetchPosts_Standardfeed_ListErrorPropagates(t *testing.T) {
	wantErr := errors.New("pds unreachable")
	lister := &fakeDocLister{err: wantErr}
	f := NewFetcher(&fakeRSSFetcher{}, lister)

	_, err := f.FetchPosts(context.Background(), "at://did:plc:example/site.standard.publication/abc")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestFetchPosts_DispatchesByKeyShape(t *testing.T) {
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{}}}
	lister := &fakeDocLister{}
	f := NewFetcher(rss, lister).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	if _, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml"); err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if rss.calls != 1 || lister.calls != 0 {
		t.Fatalf("rss.calls = %d, lister.calls = %d, want a plain URL key dispatched to the rss fetcher", rss.calls, lister.calls)
	}

	if _, err := f.FetchPosts(context.Background(), "at://did:plc:example/site.standard.publication/abc"); err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if rss.calls != 1 || lister.calls != 1 {
		t.Fatalf("rss.calls = %d, lister.calls = %d, want an at:// key dispatched to the standardfeed lister", rss.calls, lister.calls)
	}
}

func TestFetchPosts_RSS_FaviconCaptured(t *testing.T) {
	at := mustTime(t, "2026-01-01T00:00:00Z")
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{
		Link:  "https://blog.example",
		Items: []*gofeed.Item{{Title: "Post", Link: "https://blog.example/post", PublishedParsed: &at}},
	}}}
	fav := &fakeFaviconDiscoverer{url: "https://blog.example/favicon.ico"}
	f := NewFetcher(rss, &fakeDocLister{}).WithFaviconDiscoverer(fav)

	got, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if got.FaviconURL != "https://blog.example/favicon.ico" {
		t.Errorf("FaviconURL = %q, want the discovered icon", got.FaviconURL)
	}
	if len(fav.sites) != 1 || fav.sites[0] != "https://blog.example" {
		t.Errorf("favicon discovery sites = %v, want the feed's Link", fav.sites)
	}
}

func TestFetchPosts_RSS_FaviconFailureNonFatal(t *testing.T) {
	at := mustTime(t, "2026-01-01T00:00:00Z")
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{
		Link:  "https://blog.example",
		Items: []*gofeed.Item{{Title: "Post", Link: "https://blog.example/post", PublishedParsed: &at}},
	}}}
	fav := &fakeFaviconDiscoverer{err: errors.New("no icon found")}
	f := NewFetcher(rss, &fakeDocLister{}).WithFaviconDiscoverer(fav)

	got, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml")
	if err != nil {
		t.Fatalf("FetchPosts: %v, want the posts fetch to succeed despite the favicon failure", err)
	}
	if len(got.Posts) != 1 {
		t.Fatalf("posts = %+v, want 1", got.Posts)
	}
	if got.FaviconURL != "" {
		t.Errorf("FaviconURL = %q, want empty on discovery failure", got.FaviconURL)
	}
}

func TestFetchPosts_RSS_FaviconSiteFallsBackToFeedURLOrigin(t *testing.T) {
	at := mustTime(t, "2026-01-01T00:00:00Z")
	rss := &fakeRSSFetcher{result: &fetcher.Result{Feed: &gofeed.Feed{
		Link:  "", // no <link> in the feed XML
		Items: []*gofeed.Item{{Title: "Post", Link: "https://blog.example/post", PublishedParsed: &at}},
	}}}
	fav := &fakeFaviconDiscoverer{}
	f := NewFetcher(rss, &fakeDocLister{}).WithFaviconDiscoverer(fav)

	if _, err := f.FetchPosts(context.Background(), "https://blog.example/feed.xml"); err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(fav.sites) != 1 || fav.sites[0] != "https://blog.example" {
		t.Errorf("favicon discovery sites = %v, want the feed URL's origin", fav.sites)
	}
}

func TestFetchPosts_Standardfeed_ListsWithPreviewFetchLimit(t *testing.T) {
	lister := &fakeDocLister{docs: []standardfeed.Document{{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z"}}}
	f := NewFetcher(&fakeRSSFetcher{}, lister).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	if _, err := f.FetchPosts(context.Background(), "at://did:plc:example/site.standard.publication/abc"); err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(lister.gotLimits) != 1 || lister.gotLimits[0] != standardfeedPreviewFetchLimit {
		t.Fatalf("gotLimits = %v, want [%d]", lister.gotLimits, standardfeedPreviewFetchLimit)
	}
}

func TestFetchPosts_Standardfeed_FaviconFromResolutionIconURLNoNetworkCall(t *testing.T) {
	pubURI := "at://did:plc:example/site.standard.publication/abc"
	docs := []standardfeed.Document{{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: pubURI, Path: "/posts/good"}}
	iconURL := "https://pds.example/xrpc/com.atproto.sync.getBlob?did=did:plc:example&cid=bafkreiabc"
	resolutions := &fakePublicationResolutionReader{byKey: map[string]db.DiscoverPublicationResolution{pubURI: {IconUrl: &iconURL}}}
	fav := &fakeFaviconDiscoverer{}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(fav).WithPublicationResolutions(resolutions)

	got, err := f.FetchPosts(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if got.FaviconURL != iconURL {
		t.Errorf("FaviconURL = %q, want %q", got.FaviconURL, iconURL)
	}
	if len(fav.sites) != 0 {
		t.Errorf("favicon discovery sites = %v, want none: the resolution cache's icon_url must serve with zero network calls", fav.sites)
	}
}

func TestFetchPosts_Standardfeed_FaviconFallsBackToSiteURLSniffingWhenIconMissing(t *testing.T) {
	pubURI := "at://did:plc:example/site.standard.publication/abc"
	docs := []standardfeed.Document{{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: pubURI, Path: "/posts/good"}}
	siteURL := "https://zine.example"
	resolutions := &fakePublicationResolutionReader{byKey: map[string]db.DiscoverPublicationResolution{pubURI: {SiteUrl: &siteURL}}}
	fav := &fakeFaviconDiscoverer{url: "https://zine.example/icon.png"}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(fav).WithPublicationResolutions(resolutions)

	got, err := f.FetchPosts(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if got.FaviconURL != "https://zine.example/icon.png" {
		t.Errorf("FaviconURL = %q, want the sniffed icon", got.FaviconURL)
	}
	if len(fav.sites) != 1 || fav.sites[0] != siteURL {
		t.Errorf("favicon discovery sites = %v, want [%q]", fav.sites, siteURL)
	}
}

func TestFetchPosts_Standardfeed_FaviconEmptyWhenResolutionLookupErrors(t *testing.T) {
	pubURI := "at://did:plc:example/site.standard.publication/abc"
	docs := []standardfeed.Document{{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: pubURI, Path: "/posts/good"}}
	resolutions := &fakePublicationResolutionReader{err: sql.ErrNoRows}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{}).WithPublicationResolutions(resolutions)

	got, err := f.FetchPosts(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("FetchPosts: %v, want the posts fetch to succeed despite the favicon lookup failure", err)
	}
	if len(got.Posts) != 1 {
		t.Fatalf("posts = %+v, want 1", got.Posts)
	}
	if got.FaviconURL != "" {
		t.Errorf("FaviconURL = %q, want empty on a resolution lookup error", got.FaviconURL)
	}
}

func TestFetchPosts_Standardfeed_FaviconEmptyWithoutResolutionsWired(t *testing.T) {
	pubURI := "at://did:plc:example/site.standard.publication/abc"
	docs := []standardfeed.Document{{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: pubURI, Path: "/posts/good"}}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if got.FaviconURL != "" {
		t.Errorf("FaviconURL = %q, want empty when WithPublicationResolutions was never called", got.FaviconURL)
	}
}

// TestFetchPosts_Standardfeed_DocumentURLJoinsResolutionSiteURLForAtURISite covers the realistic case:
// ListRecentDocuments' own site filter (internal/standardfeed/client.go) only ever returns documents whose
// site matches the publication's at:// uri, so every doc fetchStandardfeed actually sees needs the resolution
// cache's site_url to produce a URL at all.
func TestFetchPosts_Standardfeed_DocumentURLJoinsResolutionSiteURLForAtURISite(t *testing.T) {
	pubURI := "at://did:plc:example/site.standard.publication/abc"
	docs := []standardfeed.Document{
		{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: pubURI, Path: "/posts/good"},
	}
	siteURL := "https://zine.example"
	resolutions := &fakePublicationResolutionReader{byKey: map[string]db.DiscoverPublicationResolution{pubURI: {SiteUrl: &siteURL}}}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{}).WithPublicationResolutions(resolutions)

	got, err := f.FetchPosts(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != 1 || got.Posts[0].URL != "https://zine.example/posts/good" {
		t.Fatalf("posts = %+v, want the resolution's site_url joined with the doc path", got.Posts)
	}
}

// TestFetchPosts_Standardfeed_DocumentURLJoinsSiteAndPath covers documentURL's other branch (an https doc.Site
// wins over the resolved siteURL) for completeness; ListRecentDocuments' site filter never actually produces
// this shape for a publication-bound document (see the test above), so this exercises the fallback logic in
// isolation rather than a real fetchStandardfeed input.
func TestFetchPosts_Standardfeed_DocumentURLJoinsSiteAndPath(t *testing.T) {
	docs := []standardfeed.Document{
		{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: "https://zine.example", Path: "/posts/good"},
	}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), "at://did:plc:example/site.standard.publication/abc")
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != 1 || got.Posts[0].URL != "https://zine.example/posts/good" {
		t.Fatalf("posts = %+v, want the joined site+path URL", got.Posts)
	}
}

func TestFetchPosts_Standardfeed_DocumentURLEmptyWithoutResolvableSite(t *testing.T) {
	pubURI := "at://did:plc:example/site.standard.publication/abc"
	docs := []standardfeed.Document{
		{Title: "Good", PublishedAt: "2026-02-01T00:00:00Z", Site: pubURI, Path: "/posts/good"},
	}
	f := NewFetcher(&fakeRSSFetcher{}, &fakeDocLister{docs: docs}).WithFaviconDiscoverer(&fakeFaviconDiscoverer{})

	got, err := f.FetchPosts(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("FetchPosts: %v", err)
	}
	if len(got.Posts) != 1 || got.Posts[0].URL != "" {
		t.Fatalf("posts = %+v, want an empty URL when no resolution site_url is available", got.Posts)
	}
}

func TestPostKey_DeterministicForSameIdentity(t *testing.T) {
	a := postKey("https://blog.example/feed.xml", "https://blog.example/post-1", "Title", "2026-01-01T00:00:00Z")
	b := postKey("https://blog.example/feed.xml", "https://blog.example/post-1", "Title", "2026-01-01T00:00:00Z")
	if a != b {
		t.Fatalf("postKey not deterministic: %q != %q", a, b)
	}
	if len(a) != 10 {
		t.Fatalf("postKey length = %d, want 10", len(a))
	}
}

func TestPostKey_DistinctForDifferentURL(t *testing.T) {
	a := postKey("https://blog.example/feed.xml", "https://blog.example/post-1", "Title", "2026-01-01T00:00:00Z")
	b := postKey("https://blog.example/feed.xml", "https://blog.example/post-2", "Title", "2026-01-01T00:00:00Z")
	if a == b {
		t.Fatalf("postKey collided for distinct URLs: %q", a)
	}
}

func TestPostKey_EmptyURLFallsBackToTitleAndPublishedAt_Deterministic(t *testing.T) {
	a := postKey("https://blog.example/feed.xml", "", "Title", "2026-01-01T00:00:00Z")
	b := postKey("https://blog.example/feed.xml", "", "Title", "2026-01-01T00:00:00Z")
	if a != b {
		t.Fatalf("postKey not deterministic for the title+publishedAt fallback: %q != %q", a, b)
	}
	c := postKey("https://blog.example/feed.xml", "", "Different Title", "2026-01-01T00:00:00Z")
	if a == c {
		t.Fatalf("postKey collided for distinct titles in the fallback: %q", a)
	}
}

func TestPostKey_UniqueAcrossSources(t *testing.T) {
	a := postKey("https://blog.example/feed-a.xml", "https://blog.example/post-1", "Title", "2026-01-01T00:00:00Z")
	b := postKey("https://blog.example/feed-b.xml", "https://blog.example/post-1", "Title", "2026-01-01T00:00:00Z")
	if a == b {
		t.Fatalf("postKey collided across sources: %q", a)
	}
}

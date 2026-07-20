// Package discoverposts fetches a small newest-posts preview for a discover
// source candidate, plus a best-effort favicon captured from the same fetch.
package discoverposts

import (
	"context"
	"crypto/sha256"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/database/db"
	"morgenblau/internal/favicon"
	"morgenblau/internal/feedkey"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/standardfeed"
)

// PreviewCap bounds how many posts a candidate preview shows. SPEC <discovery> Presentation.
const PreviewCap = 3

// standardfeedPreviewFetchLimit gives headroom over PreviewCap since the publication's site filter runs
// client-side and multi-publication repos share one document collection.
const standardfeedPreviewFetchLimit = 50

// Post is one preview item; PublishedAt is RFC3339, empty when undeterminable.
type Post struct {
	Title       string
	PublishedAt string
	URL         string
	Key         string
}

// FetchResult bundles a fetch's posts with a best-effort favicon URL captured from the same network round trip.
type FetchResult struct {
	Posts      []Post
	FaviconURL string
}

// RSSFetcher is the fetcher.Fetcher slice this package needs.
type RSSFetcher interface {
	Fetch(ctx context.Context, rawURL string, state fetcher.FeedState) (*fetcher.Result, error)
}

// StandardfeedDocumentLister is the standardfeed.Client slice this package needs.
type StandardfeedDocumentLister interface {
	ListRecentDocuments(ctx context.Context, pubURI string, limit int) ([]standardfeed.Document, error)
}

// FaviconDiscoverer lets tests stub favicon discovery.
type FaviconDiscoverer interface {
	Discover(ctx context.Context, siteURL string) (string, error)
}

// PublicationFaviconReader is the resolution-cache slice discoverposts needs for standardfeed favicons; matches
// the sqlc-generated signature exactly (canonical_key is nullable, hence the pointer param).
type PublicationFaviconReader interface {
	GetDiscoverPublicationResolutionByCanonicalKey(ctx context.Context, canonicalKey *string) (db.DiscoverPublicationResolution, error)
}

type faviconHTTPClient struct{ client *http.Client }

func defaultFaviconDiscoverer() FaviconDiscoverer {
	return &faviconHTTPClient{client: safehttp.NewClient(10*time.Second, 5)}
}

func (c *faviconHTTPClient) Discover(ctx context.Context, siteURL string) (string, error) {
	return favicon.Discover(ctx, c.client, siteURL)
}

// Fetcher fetches a candidate source's newest posts plus a best-effort favicon.
type Fetcher struct {
	rss         RSSFetcher
	std         StandardfeedDocumentLister
	favicon     FaviconDiscoverer
	resolutions PublicationFaviconReader
}

func NewFetcher(rss RSSFetcher, std StandardfeedDocumentLister) *Fetcher {
	return &Fetcher{rss: rss, std: std, favicon: defaultFaviconDiscoverer()}
}

// WithFaviconDiscoverer swaps the favicon discoverer for tests.
func (f *Fetcher) WithFaviconDiscoverer(d FaviconDiscoverer) *Fetcher {
	f.favicon = d
	return f
}

// WithPublicationResolutions wires the standardfeed favicon source; without it, standardfeedFavicon always returns "".
func (f *Fetcher) WithPublicationResolutions(r PublicationFaviconReader) *Fetcher {
	f.resolutions = r
	return f
}

// FetchPosts fetches key's newest posts (capped at PreviewCap) and captures a best-effort favicon from the same round trip; the fetch path is derived from key shape (feedkey.Kind) rather than a caller-supplied kind.
func (f *Fetcher) FetchPosts(ctx context.Context, key string) (FetchResult, error) {
	if feedkey.Kind(key) == "standardfeed" {
		return f.fetchStandardfeed(ctx, key)
	}
	return f.fetchRSS(ctx, key)
}

type datedPost struct {
	post Post
	at   time.Time
}

func newestFirstCapped(items []datedPost) []Post {
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	if len(items) > PreviewCap {
		items = items[:PreviewCap]
	}
	posts := make([]Post, 0, len(items))
	for _, d := range items {
		posts = append(posts, d.post)
	}
	return posts
}

func (f *Fetcher) fetchRSS(ctx context.Context, feedURL string) (FetchResult, error) {
	res, err := f.rss.Fetch(ctx, feedURL, fetcher.FeedState{})
	if err != nil {
		return FetchResult{}, err
	}
	if res.Feed == nil {
		return FetchResult{}, nil
	}

	var items []datedPost
	for _, item := range res.Feed.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		post := Post{Title: title, URL: item.Link}
		at, ok := chooseRSSPublished(item)
		if ok {
			post.PublishedAt = at.UTC().Format(time.RFC3339)
		}
		post.Key = postKey(feedURL, post.URL, post.Title, post.PublishedAt)
		items = append(items, datedPost{post: post, at: at})
	}

	site := strings.TrimSpace(res.Feed.Link)
	if site == "" {
		// Falls back to the feed URL's origin so favicon discovery has somewhere to look when the feed XML lacks a <link>.
		if u, err := url.Parse(feedURL); err == nil && u.Scheme != "" && u.Host != "" {
			site = u.Scheme + "://" + u.Host
		}
	}

	return FetchResult{Posts: newestFirstCapped(items), FaviconURL: f.discoverFavicon(ctx, site)}, nil
}

// chooseRSSPublished mirrors the PublishedParsed->UpdatedParsed fallback in internal/sync/feedpipeline.go's chooseTime, without that function's raw-date-format parsing (out of scope for a preview).
func chooseRSSPublished(item *gofeed.Item) (time.Time, bool) {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed, true
	}
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed, true
	}
	return time.Time{}, false
}

// postKey returns a deterministic 10-char base62 key, same algorithm/alphabet as sync.EntrySlug. Identity is the
// post URL when present, else title+publishedAt; salting with sourceKey keeps keys stable across TTL refetches
// and unique across sources sharing the same identity.
func postKey(sourceKey, postURL, title, publishedAt string) string {
	identity := postURL
	if identity == "" {
		identity = title + "|" + publishedAt
	}
	digest := sha256.Sum256([]byte(sourceKey + "|" + identity))
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	out := make([]byte, 10)
	for i := range out {
		out[i] = alphabet[digest[i]%62]
	}
	return string(out)
}

func (f *Fetcher) fetchStandardfeed(ctx context.Context, pubURI string) (FetchResult, error) {
	docs, err := f.std.ListRecentDocuments(ctx, pubURI, standardfeedPreviewFetchLimit)
	if err != nil {
		return FetchResult{}, err
	}

	resolution, haveResolution := f.publicationResolution(ctx, pubURI)
	siteURL := ""
	if haveResolution && resolution.SiteUrl != nil {
		siteURL = *resolution.SiteUrl
	}

	var items []datedPost
	for _, doc := range docs {
		title := strings.TrimSpace(doc.Title)
		if title == "" || doc.PublishedAt == "" {
			continue
		}
		// A malformed date still keeps the doc (only emptiness drops it); it just sorts as oldest.
		at, _ := time.Parse(time.RFC3339, doc.PublishedAt)
		post := Post{
			Title:       title,
			PublishedAt: doc.PublishedAt,
			URL:         documentURL(doc, siteURL),
		}
		post.Key = postKey(pubURI, post.URL, post.Title, post.PublishedAt)
		items = append(items, datedPost{post: post, at: at})
	}

	return FetchResult{Posts: newestFirstCapped(items), FaviconURL: f.standardfeedFavicon(ctx, resolution, haveResolution)}, nil
}

// publicationResolution is a nil-safe single lookup shared by post URLs and the favicon fallback, so fetchStandardfeed never queries the resolution cache twice for the same publication.
func (f *Fetcher) publicationResolution(ctx context.Context, pubURI string) (db.DiscoverPublicationResolution, bool) {
	if f.resolutions == nil {
		return db.DiscoverPublicationResolution{}, false
	}
	row, err := f.resolutions.GetDiscoverPublicationResolutionByCanonicalKey(ctx, &pubURI)
	if err != nil {
		return db.DiscoverPublicationResolution{}, false
	}
	return row, true
}

// standardfeedFavicon prefers the icon captured at resolution time (zero network calls); a missing resolution
// or one with neither icon nor site is never fatal to the fetch.
func (f *Fetcher) standardfeedFavicon(ctx context.Context, row db.DiscoverPublicationResolution, haveResolution bool) string {
	if !haveResolution {
		return ""
	}
	if row.IconUrl != nil && *row.IconUrl != "" {
		return *row.IconUrl
	}
	if row.SiteUrl != nil {
		return f.discoverFavicon(ctx, *row.SiteUrl)
	}
	return ""
}

// documentURL picks a document's canonical join base: its own https site when present (loose documents carry
// their real site), else the publication's resolved siteURL, since standardfeed documents bound to a
// publication store an at:// site with no derivable web URL on its own.
func documentURL(doc standardfeed.Document, siteURL string) string {
	base := doc.Site
	if !strings.HasPrefix(base, "https://") {
		base = siteURL
	}
	return canonicalDocumentURL(base, doc.Path)
}

// canonicalDocumentURL mirrors internal/sync/standardpipeline.go's unexported function of the same name so a
// preview's post URL agrees with what Tier-2 stores for the same document; duplicated rather than imported
// since discoverposts stays leaf-like and sync doesn't export it.
func canonicalDocumentURL(baseURL, path string) string {
	if path == "" || baseURL == "" {
		return ""
	}
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (f *Fetcher) discoverFavicon(ctx context.Context, site string) string {
	if site == "" {
		return ""
	}
	iconURL, err := f.favicon.Discover(ctx, site)
	if err != nil {
		slog.Warn("discoverposts: favicon discovery failed", "site", site, "err", err)
		return ""
	}
	return iconURL
}

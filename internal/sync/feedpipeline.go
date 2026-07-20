package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverlang"
	"morgenblau/internal/favicon"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
)

// iconRefreshAfter: long enough to avoid hammering the site, short enough to catch a redesign within a month.
const iconRefreshAfter = 30 * 24 * time.Hour

// feedBackoff implements SPEC <feed-sources> failure handling; the 24h cap also serves as the muted feeds' once-daily retry.
var feedBackoff = backoff.Policy{Steps: []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}}

// FaviconDiscoverer lets tests stub favicon discovery.
type FaviconDiscoverer interface {
	Discover(ctx context.Context, siteURL string) (string, error)
}

type faviconHTTPClient struct{ client *http.Client }

func defaultFaviconDiscoverer() FaviconDiscoverer {
	return &faviconHTTPClient{client: safehttp.NewClient(10*time.Second, 5)}
}

func (c *faviconHTTPClient) Discover(ctx context.Context, siteURL string) (string, error) {
	return favicon.Discover(ctx, c.client, siteURL)
}

// FeedPipeline implements FeedFetcher for RSS/Atom feeds.
type FeedPipeline struct {
	fetcher   *fetcher.Fetcher
	queries   pipelineQueries
	sanitizer *bluemonday.Policy
	now       func() time.Time
	favicon   FaviconDiscoverer
	detector  discoverlang.Detector
	runTx     func(ctx context.Context, fn func(pipelineQueries) error) error
}

type pipelineQueries interface {
	GetFeed(ctx context.Context, feedURL string) (db.Feed, error)
	UpdateFeedFetchState(ctx context.Context, arg db.UpdateFeedFetchStateParams) error
	UpdateFeedFetchFailure(ctx context.Context, arg db.UpdateFeedFetchFailureParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	UpsertFeedEntry(ctx context.Context, arg db.UpsertFeedEntryParams) error
	SetFeedIconURL(ctx context.Context, arg db.SetFeedIconURLParams) error
}

func NewFeedPipeline(f *fetcher.Fetcher, q pipelineQueries) *FeedPipeline {
	p := &FeedPipeline{
		fetcher:   f,
		queries:   q,
		sanitizer: bluemonday.UGCPolicy(),
		now:       time.Now,
		favicon:   defaultFaviconDiscoverer(),
		detector:  discoverlang.NewDetector(),
	}
	// No transaction by default so fake-based tests work; production installs a real runner via WithTxRunner.
	p.runTx = func(ctx context.Context, fn func(pipelineQueries) error) error {
		return fn(p.queries)
	}
	return p
}

// WithFaviconDiscoverer swaps the favicon discoverer for tests.
func (p *FeedPipeline) WithFaviconDiscoverer(d FaviconDiscoverer) *FeedPipeline {
	p.favicon = d
	return p
}

// WithTxRunner commits each feed's write batch in one transaction on the writer pool.
func (p *FeedPipeline) WithTxRunner(w *sql.DB) *FeedPipeline {
	p.runTx = func(ctx context.Context, fn func(pipelineQueries) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return p
}

func (p *FeedPipeline) FetchAndStore(ctx context.Context, feedURL string) error {
	state := fetcher.FeedState{}
	var existing db.Feed
	if row, err := p.queries.GetFeed(ctx, feedURL); err == nil {
		existing = row
		if row.Etag != nil {
			state.ETag = *row.Etag
		}
		if row.LastModified != nil {
			state.LastModified = *row.LastModified
		}
	}

	if inBackoff(existing, p.now()) {
		slog.Debug("feedpipeline: skipping fetch, feed in backoff", "url", feedURL, "next_fetch_at", existing.NextFetchAt)
		return nil
	}

	res, err := p.fetcher.Fetch(ctx, feedURL, state)
	if err != nil {
		// context.Canceled means the caller gave up, not that the upstream is unhealthy; it must not count against the feed.
		if !errors.Is(err, context.Canceled) {
			p.recordFetchFailure(ctx, feedURL, existing, err)
		}
		return err
	}

	nowStr := p.now().UTC().Format(time.RFC3339)
	fetchState := db.UpdateFeedFetchStateParams{
		Etag:          nilIfEmpty(res.ETag),
		LastModified:  nilIfEmpty(res.LastModified),
		LastFetchedAt: &nowStr,
		UpdatedAt:     nowStr,
		FeedUrl:       feedURL,
	}

	// NotModified only touches fetch state, a single auto-commit write; no transaction needed.
	if res.NotModified || res.Feed == nil {
		if err := p.queries.UpdateFeedFetchState(ctx, fetchState); err != nil {
			slog.Warn("feedpipeline: Tier-2 state update failed", "url", feedURL, "err", err)
		}
		return nil
	}

	// The feed-claimed title (res.Feed.Title) is intentionally never persisted to
	// Tier-1: the subscription title is user-owned and a refresh must not clobber it.
	feedSite := strings.TrimSpace(res.Feed.Link)
	if feedSite == "" {
		// Falls back to the feed URL's origin so favicon discovery has somewhere to look when the feed XML lacks a <link>.
		if u, err := url.Parse(feedURL); err == nil && u.Scheme != "" && u.Host != "" {
			feedSite = u.Scheme + "://" + u.Host
		}
	}

	// Favicon discovery is a network call, hoisted out of the transaction so the write batch never holds the writer connection across a slow fetch.
	var iconURL string
	if feedSite != "" && shouldDiscoverIcon(existing, p.now()) {
		if u, err := p.favicon.Discover(ctx, feedSite); err == nil && u != "" {
			iconURL = u
		} else if err != nil {
			slog.Warn("feedpipeline: favicon discovery failed", "site", feedSite, "err", err)
		}
	}

	// SPEC <discovery> Global/Trending ranking: content-based detection primary, feed tag only a hint; reuses this fetch's content instead of a dedicated call.
	language := languageOrNil(p.detector, languageSample(res.Feed.Items), res.Feed.Language)

	// Per-entry errors are tolerated (SQLite statement errors don't poison the
	// tx) so fn always returns nil; only a Begin/Commit failure rolls the batch back.
	return p.runTx(ctx, func(q pipelineQueries) error {
		if err := q.UpdateFeedFetchState(ctx, fetchState); err != nil {
			slog.Warn("feedpipeline: Tier-2 state update failed", "url", feedURL, "err", err)
		}
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   feedURL,
			SiteUrl:   nilIfEmpty(feedSite),
			Language:  language,
			CreatedAt: nowStr,
			UpdatedAt: nowStr,
		}); err != nil {
			slog.Warn("feedpipeline: feed upsert failed", "url", feedURL, "err", err)
		}
		if iconURL != "" {
			fetchedAt := nowStr
			if err := q.SetFeedIconURL(ctx, db.SetFeedIconURLParams{
				IconUrl:       &iconURL,
				IconFetchedAt: &fetchedAt,
				UpdatedAt:     nowStr,
				FeedUrl:       feedURL,
			}); err != nil {
				slog.Warn("feedpipeline: icon persist failed", "url", feedURL, "err", err)
			}
		}

		for _, item := range res.Feed.Items {
			guid := chooseGUID(item)
			if guid == "" {
				continue
			}
			published := chooseTime(item, p.now())
			body := chooseBody(item)
			sanitized := p.sanitizer.Sanitize(body)
			ct := classifyContentType(feedURL, item)
			meta := buildMetadata(item)

			var metaJSON *string
			if len(meta) > 0 {
				if raw, err := json.Marshal(meta); err == nil {
					s := string(raw)
					metaJSON = &s
				}
			}

			if err := q.UpsertFeedEntry(ctx, db.UpsertFeedEntryParams{
				FeedUrl:     feedURL,
				Guid:        guid,
				EntrySlug:   EntrySlug(feedURL, guid),
				Url:         item.Link,
				Title:       nilIfEmpty(strings.TrimSpace(item.Title)),
				ContentHtml: nilIfEmpty(sanitized),
				ContentType: ct,
				PublishedAt: published.UTC().Format(time.RFC3339),
				FetchedAt:   nowStr,
				Metadata:    metaJSON,
			}); err != nil {
				slog.Warn("feedpipeline: entry upsert failed", "url", feedURL, "guid", guid, "err", err)
			}
		}

		return nil
	})
}

func chooseGUID(item *gofeed.Item) string {
	if g := strings.TrimSpace(item.GUID); g != "" {
		return g
	}
	if l := strings.TrimSpace(item.Link); l != "" {
		return l
	}
	return ""
}

func chooseTime(item *gofeed.Item, fallback time.Time) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed
	}
	if t, ok := parseRawDate(item.Published); ok {
		return t
	}
	if t, ok := parseRawDate(item.Updated); ok {
		return t
	}
	return fallback
}

// parseRawDate covers date shapes gofeed misses: Bluesky's RSS pubDate has no
// weekday prefix, no seconds, and a numeric offset (e.g. "24 May 2026 20:02 +0200").
var extraDateFormats = []string{
	"2 Jan 2006 15:04 -0700",
}

func parseRawDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range extraDateFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func chooseBody(item *gofeed.Item) string {
	if c := strings.TrimSpace(item.Content); c != "" {
		return c
	}
	return strings.TrimSpace(item.Description)
}

// classifyContentType applies SPEC <content-types>; defaults to "blogpost" as the safest UI fallback.
func classifyContentType(feedURL string, item *gofeed.Item) string {
	if u, err := url.Parse(feedURL); err == nil {
		host := strings.ToLower(u.Host)
		if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
			return "video"
		}
	}
	for _, enc := range item.Enclosures {
		t := strings.ToLower(enc.Type)
		if strings.HasPrefix(t, "video/") {
			return "video"
		}
	}
	if strings.TrimSpace(item.Title) == "" {
		return "microblog"
	}
	return "blogpost"
}

// buildMetadata captures type-specific metadata as a JSON blob, promoted to typed columns only once their UI ships.
func buildMetadata(item *gofeed.Item) map[string]any {
	out := map[string]any{}
	if len(item.Authors) > 0 && item.Authors[0] != nil && item.Authors[0].Name != "" {
		out["author"] = item.Authors[0].Name
	}
	if item.Image != nil && item.Image.URL != "" {
		out["image"] = item.Image.URL
	}
	if len(item.Enclosures) > 0 {
		enc := item.Enclosures[0]
		out["enclosure"] = map[string]any{
			"url":  enc.URL,
			"type": enc.Type,
		}
	}
	return out
}

// inBackoff treats a missing or unparseable next_fetch_at as eligible so partial state always re-fetches.
func inBackoff(f db.Feed, now time.Time) bool {
	if f.NextFetchAt == nil {
		return false
	}
	next, err := time.Parse(time.RFC3339, *f.NextFetchAt)
	if err != nil {
		return false
	}
	return now.Before(next)
}

// recordFetchFailure persists SPEC <feed-sources> backoff state; write errors are logged, never returned, so they don't mask the fetch error that triggered them.
func (p *FeedPipeline) recordFetchFailure(ctx context.Context, feedURL string, prior db.Feed, fetchErr error) {
	failures := prior.ConsecutiveFailures + 1
	delay := feedBackoff.Delay(int(failures))

	var httpErr *fetcher.HTTPError
	if errors.As(fetchErr, &httpErr) && httpErr.RetryAfter > 0 {
		serverDelay := min(httpErr.RetryAfter, feedBackoff.Cap())
		delay = max(delay, serverDelay)
	}

	nextFetchAt := p.now().UTC().Add(delay).Format(time.RFC3339)
	if err := p.queries.UpdateFeedFetchFailure(ctx, db.UpdateFeedFetchFailureParams{
		ConsecutiveFailures: failures,
		NextFetchAt:         &nextFetchAt,
		UpdatedAt:           p.now().UTC().Format(time.RFC3339),
		FeedUrl:             feedURL,
	}); err != nil {
		slog.Warn("feedpipeline: failure state update failed", "url", feedURL, "err", err)
	}
}

// shouldDiscoverIcon treats a missing or unparseable icon_fetched_at as stale so partial state always re-resolves.
func shouldDiscoverIcon(f db.Feed, now time.Time) bool {
	if f.IconUrl == nil || *f.IconUrl == "" {
		return true
	}
	if f.IconFetchedAt == nil {
		return true
	}
	fetched, err := time.Parse(time.RFC3339, *f.IconFetchedAt)
	if err != nil {
		return true
	}
	return now.Sub(fetched) > iconRefreshAfter
}

// languageSampleMaxItems/Bytes cap detection cost: enough prose for a confident trigram read, capped so a huge feed can't blow up per-fetch CPU.
const (
	languageSampleMaxItems = 10
	languageSampleMaxBytes = 4000
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// languageSample builds rough plain text for language.Detect; a trigram detector tolerates leftover markup, so this skips full HTML extraction.
func languageSample(items []*gofeed.Item) string {
	var b strings.Builder
	for i, item := range items {
		if i >= languageSampleMaxItems || b.Len() >= languageSampleMaxBytes {
			break
		}
		b.WriteString(item.Title)
		b.WriteString(" ")
		b.WriteString(htmlTagPattern.ReplaceAllString(chooseBody(item), " "))
		b.WriteString(" ")
	}
	sample := b.String()
	if len(sample) > languageSampleMaxBytes {
		sample = sample[:languageSampleMaxBytes]
	}
	return sample
}

// languageOrNil returns nil for undetermined language (SPEC <discovery>: undetermined sources still pass the filter, nil is never a guess).
func languageOrNil(detector discoverlang.Detector, sample, tagHint string) *string {
	lang, ok := discoverlang.SourceLanguage(detector, sample, tagHint)
	if !ok {
		return nil
	}
	s := string(lang)
	return &s
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/favicon"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
)

const iconRefreshAfter = 30 * 24 * time.Hour

var feedBackoff = backoff.Policy{Steps: []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}}

const youtubeRetry = 15 * time.Minute

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

type FeedPipeline struct {
	fetcher   *fetcher.Fetcher
	queries   pipelineQueries
	sanitizer *bluemonday.Policy
	now       func() time.Time
	favicon   FaviconDiscoverer

	youtubeHandle  func(ctx context.Context, channelURL string) (string, error)
	youtubeGone    func(ctx context.Context, channelURL string) (bool, error)
	runTx          func(ctx context.Context, fn func(pipelineQueries) error) error
	undatedHistory func(feedURL string) bool
}

type pipelineQueries interface {
	GetFeed(ctx context.Context, feedURL string) (db.Feed, error)
	UpdateFeedFetchState(ctx context.Context, arg db.UpdateFeedFetchStateParams) error
	UpdateFeedFetchFailure(ctx context.Context, arg db.UpdateFeedFetchFailureParams) error
	CountYouTubeFeedFailures(ctx context.Context, failingSince string) (db.CountYouTubeFeedFailuresRow, error)
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
	}
	handleClient := safehttp.NewClient(10*time.Second, 5)
	p.youtubeHandle = func(ctx context.Context, channelURL string) (string, error) {
		return feedfinder.YouTubeHandleURL(ctx, handleClient, channelURL)
	}
	p.youtubeGone = func(ctx context.Context, channelURL string) (bool, error) {
		return feedfinder.YouTubeChannelGone(ctx, handleClient, channelURL)
	}
	p.runTx = func(ctx context.Context, fn func(pipelineQueries) error) error {
		return fn(p.queries)
	}
	return p
}

func (p *FeedPipeline) WithFaviconDiscoverer(d FaviconDiscoverer) *FeedPipeline {
	p.favicon = d
	return p
}

func (p *FeedPipeline) WithUndatedHistory(isHistory func(feedURL string) bool) *FeedPipeline {
	p.undatedHistory = isHistory
	return p
}

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

	if res.NotModified || res.Feed == nil {
		if err := p.queries.UpdateFeedFetchState(ctx, fetchState); err != nil {
			slog.Warn("feedpipeline: Tier-2 state update failed", "url", feedURL, "err", err)
		}
		return nil
	}

	feedSite := strings.TrimSpace(res.Feed.Link)
	if feedSite == "" {
		feedSite = feedfinder.YouTubeChannelURL(feedURL)
	}
	if feedSite == "" {
		feedSite = origin(feedURL)
	}

	storedSite := feedSite
	channel := feedfinder.YouTubeChannelURL(feedURL)
	if channel == "" {
		channel = feedfinder.YouTubeChannelURL(feedSite)
	}
	if channel != "" {
		storedSite = channel
		if existing.SiteUrl != nil && feedfinder.IsYouTubeHandleURL(*existing.SiteUrl) {
			storedSite = ""
		} else if handle, err := p.youtubeHandle(ctx, channel); err != nil {
			slog.Warn("feedpipeline: YouTube handle lookup failed", "channel", channel, "err", err)
		} else if handle != "" {
			storedSite = handle
		}
	}

	var iconURL string
	if feedSite != "" && shouldDiscoverIcon(existing, p.now()) {
		if u, err := p.favicon.Discover(ctx, feedSite); err == nil && u != "" {
			iconURL = u
		} else if err != nil {
			slog.Warn("feedpipeline: favicon discovery failed", "site", feedSite, "err", err)
		}
	}

	// SQLite backs out only a failing statement and keeps the transaction active, so a failed entry write leaves its siblings in the batch.
	// https://www.sqlite.org/lang_conflict.html
	return p.runTx(ctx, func(q pipelineQueries) error {
		if err := q.UpdateFeedFetchState(ctx, fetchState); err != nil {
			slog.Warn("feedpipeline: Tier-2 state update failed", "url", feedURL, "err", err)
		}
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   feedURL,
			SiteUrl:   nilIfEmpty(storedSite),
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

		firstSuccessfulFetch := existing.LastFetchedAt == nil
		keepUndated := boolToInt64(firstSuccessfulFetch || (p.undatedHistory != nil && p.undatedHistory(feedURL)))
		for _, item := range res.Feed.Items {
			guid := chooseGUID(item)
			if guid == "" {
				continue
			}
			published := chooseTime(item, time.Time{})
			var dateFrom, dateUntil *string
			if day, ok := dateOnly(published); ok {
				from, until := day.Add(-furthestAheadOfUTC), day.Add(24*time.Hour+furthestBehindUTC)
				noonUTC := day.Add(12 * time.Hour)
				now := p.now()
				foundByALaterFetch := existing.LastFetchedAt != nil && !now.Before(from) && now.Before(until)
				published = noonUTC
				if foundByALaterFetch || now.Before(noonUTC) {
					published = now
				}
				f, u := from.Format(time.RFC3339), until.Format(time.RFC3339)
				dateFrom, dateUntil = &f, &u
			}
			publishedAt := ""
			if !published.IsZero() {
				publishedAt = published.UTC().Format(time.RFC3339)
			}
			title := strings.TrimSpace(html.UnescapeString(item.Title))
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
				Title:       nilIfEmpty(title),
				ContentHtml: nilIfEmpty(sanitized),
				ContentType: ct,
				PublishedAt: publishedAt,
				DateFrom:    dateFrom,
				DateUntil:   dateUntil,
				FetchedAt:   nowStr,
				Metadata:    metaJSON,
				KeepUndated: keepUndated,
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
	// gofeed's Atom translator fills PublishedParsed from UpdatedParsed when an entry has no parsed publication date.
	// https://github.com/mmcdole/gofeed/blob/v1.5.0/translator.go#L643-L646
	if t, ok := parseRawDate(item.Published); ok {
		return t
	}
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed
	}
	if t, ok := parseRawDate(item.Updated); ok {
		return t
	}
	return fallback
}

const (
	furthestAheadOfUTC = 14 * time.Hour
	furthestBehindUTC  = 12 * time.Hour
)

func dateOnly(t time.Time) (time.Time, bool) {
	if t.IsZero() || t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 || t.Nanosecond() != 0 {
		return time.Time{}, false
	}
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), true
}

var extraDateFormats = []string{
	blueskyPubDate,
	"Mon, 2 Jan 2006 15:04:05 -0700",
}

const blueskyPubDate = "2 Jan 2006 15:04 -0700"

func parseRawDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if date, clock, ok := strings.Cut(s, "T"); ok {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(date)+"T"+clock); err == nil {
			return t, true
		}
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

func (p *FeedPipeline) recordFetchFailure(ctx context.Context, feedURL string, prior db.Feed, fetchErr error) {
	failures := prior.ConsecutiveFailures + 1
	lone, goneAt := prior.LoneFailures, prior.ChannelGoneAt
	delay := feedBackoff.Delay(int(failures))
	if feedfinder.IsYouTubeFeedURL(feedURL) {
		if p.youtubeUp(ctx) {
			lone++
			if channel := feedfinder.YouTubeChannelURL(feedURL); channel != "" && checksChannelPage(lone, goneAt) {
				if gone, err := p.youtubeGone(ctx, channel); err != nil {
					slog.Warn("feedpipeline: YouTube channel check failed", "channel", channel, "err", err)
				} else if !gone {
					goneAt = nil
				} else if goneAt == nil {
					at := p.now().UTC().Format(time.RFC3339)
					goneAt = &at
				}
			}
		}
		delay = youtubeRetry
		if goneAt != nil || lone >= feedfinder.YouTubeMutedAfterLoneFailures {
			delay = feedBackoff.Cap()
		}
	}

	var httpErr *fetcher.HTTPError
	if errors.As(fetchErr, &httpErr) && httpErr.RetryAfter > 0 {
		serverDelay := min(httpErr.RetryAfter, feedBackoff.Cap())
		delay = max(delay, serverDelay)
	}

	nextFetchAt := p.now().UTC().Add(delay).Format(time.RFC3339)
	if err := p.queries.UpdateFeedFetchFailure(ctx, db.UpdateFeedFetchFailureParams{
		ConsecutiveFailures: failures,
		LoneFailures:        lone,
		ChannelGoneAt:       goneAt,
		NextFetchAt:         &nextFetchAt,
		UpdatedAt:           p.now().UTC().Format(time.RFC3339),
		FeedUrl:             feedURL,
	}); err != nil {
		slog.Warn("feedpipeline: failure state update failed", "url", feedURL, "err", err)
	}
}

func (p *FeedPipeline) youtubeUp(ctx context.Context) bool {
	feeds, err := p.queries.CountYouTubeFeedFailures(ctx, feedfinder.YouTubeOutageCutoff(p.now()))
	if err != nil {
		slog.Warn("feedpipeline: YouTube feed health read failed", "err", err)
		return false
	}
	return !feedfinder.YouTubeFeedsDown(feeds.Total, feeds.Failing)
}

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

func checksChannelPage(lone int64, goneAt *string) bool {
	secondLoneFailure := lone == 2
	return secondLoneFailure || lone >= feedfinder.YouTubeMutedAfterLoneFailures || goneAt != nil
}

func origin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
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

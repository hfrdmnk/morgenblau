package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"

	"morgenblau/internal/database/db"
	"morgenblau/internal/fetcher"
)

// FeedPipeline implements FeedFetcher: pulls the feed, sanitizes entry HTML,
// classifies content type, and persists everything (Tier-2 state + entries).
type FeedPipeline struct {
	fetcher   *fetcher.Fetcher
	queries   pipelineQueries
	sanitizer *bluemonday.Policy
	now       func() time.Time
}

type pipelineQueries interface {
	GetFeed(ctx context.Context, feedURL string) (db.Feed, error)
	UpdateFeedFetchState(ctx context.Context, arg db.UpdateFeedFetchStateParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	UpsertFeedEntry(ctx context.Context, arg db.UpsertFeedEntryParams) error
}

func NewFeedPipeline(f *fetcher.Fetcher, q pipelineQueries) *FeedPipeline {
	return &FeedPipeline{
		fetcher:   f,
		queries:   q,
		sanitizer: bluemonday.UGCPolicy(),
		now:       time.Now,
	}
}

func (p *FeedPipeline) FetchAndStore(ctx context.Context, feedURL string) error {
	state := fetcher.FeedState{}
	if row, err := p.queries.GetFeed(ctx, feedURL); err == nil {
		if row.Etag != nil {
			state.ETag = *row.Etag
		}
		if row.LastModified != nil {
			state.LastModified = *row.LastModified
		}
	}

	res, err := p.fetcher.Fetch(ctx, feedURL, state)
	if err != nil {
		return err
	}

	nowStr := p.now().UTC().Format(time.RFC3339)
	if err := p.queries.UpdateFeedFetchState(ctx, db.UpdateFeedFetchStateParams{
		Etag:          nilIfEmpty(res.ETag),
		LastModified:  nilIfEmpty(res.LastModified),
		LastFetchedAt: &nowStr,
		UpdatedAt:     nowStr,
		FeedUrl:       feedURL,
	}); err != nil {
		slog.Warn("feedpipeline: Tier-2 state update failed", "url", feedURL, "err", err)
	}

	if res.NotModified || res.Feed == nil {
		return nil
	}

	// Refresh feed-level metadata opportunistically.
	feedTitle := strings.TrimSpace(res.Feed.Title)
	feedSite := strings.TrimSpace(res.Feed.Link)
	if err := p.queries.UpsertFeed(ctx, db.UpsertFeedParams{
		FeedUrl:   feedURL,
		Title:     nilIfEmpty(feedTitle),
		SiteUrl:   nilIfEmpty(feedSite),
		CreatedAt: nowStr,
		UpdatedAt: nowStr,
	}); err != nil {
		slog.Warn("feedpipeline: feed upsert failed", "url", feedURL, "err", err)
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

		if err := p.queries.UpsertFeedEntry(ctx, db.UpsertFeedEntryParams{
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
	return fallback
}

func chooseBody(item *gofeed.Item) string {
	if c := strings.TrimSpace(item.Content); c != "" {
		return c
	}
	return strings.TrimSpace(item.Description)
}

// classifyContentType derives the SPEC <content-types> classification from
// the feed URL and item shape. Conservative — defaults to "blogpost" because
// that's the safest UI fallback.
func classifyContentType(feedURL string, item *gofeed.Item) string {
	if u, err := url.Parse(feedURL); err == nil {
		host := strings.ToLower(u.Host)
		if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
			return "video"
		}
	}
	for _, enc := range item.Enclosures {
		t := strings.ToLower(enc.Type)
		if strings.HasPrefix(t, "audio/") {
			return "podcast"
		}
		if strings.HasPrefix(t, "video/") {
			return "video"
		}
	}
	if strings.TrimSpace(item.Title) == "" {
		return "microblog"
	}
	return "blogpost"
}

// buildMetadata captures type-specific metadata as a JSON blob on the entry.
// Promoted to typed columns only when their UI ships.
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

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

package discovercrawl

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/feedkey"
	"morgenblau/internal/lexicon"
)

// Reader-network save collections (SPEC <discovery> Signal ordering, weakest per-source signal). None carry a document field, so a standardfeed save is indistinguishable from rss until resolved against Tier-2.
const (
	morgenSaveCollection    = lexicon.Save
	skyreaderSaveCollection = "app.skyreader.feed.saved"
	gleanSaveCollection     = "at.glean.like"
)

// Save is one decoded save record found on a followed DID's own repo.
type Save struct {
	Kind      string // "morgen" | "skyreader" | "glean"
	ItemURL   string
	FeedURL   string // provenance, when the record carries it; empty otherwise
	CreatedAt string
}

// saveShape names the fields a collection stores its item URL and timestamp under; the three lexicons agree on nothing but the concept.
type saveShape struct {
	collection string
	kind       string
	urlField   string
	feedField  string // "" when the lexicon has no feedUrl-equivalent
	timeField  string
}

var saveShapes = [...]saveShape{
	{collection: morgenSaveCollection, kind: "morgen", urlField: "itemUrl", feedField: "feedUrl", timeField: "createdAt"},
	{collection: skyreaderSaveCollection, kind: "skyreader", urlField: "url", feedField: "", timeField: "savedAt"},
	{collection: gleanSaveCollection, kind: "glean", urlField: "articleUrl", feedField: "feedUrl", timeField: "createdAt"},
}

// CrawlSaves fetches every save record across the three save-shaped collections, deduped by itemURL; malformed records are skipped and logged, never fatal.
func (c *Client) CrawlSaves(ctx context.Context, did syntax.DID) ([]Save, error) {
	ctx, cancel := c.withCrawlTimeout(ctx)
	defer cancel()

	ident, err := c.resolver.LookupDID(ctx, did)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: resolve %s: %w", did, err)
	}
	endpoint := ident.PDSEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("discovercrawl: no PDS endpoint for %s", did)
	}
	client := c.apiClient(endpoint)
	repo := did.String()

	seen := map[string]struct{}{}
	var out []Save
	for _, shape := range saveShapes {
		records, err := pageRecords(ctx, client, repo, shape.collection)
		if err != nil {
			return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", shape.collection, did, err)
		}
		appendSaves(&out, seen, records, shape)
	}
	return out, nil
}

func appendSaves(out *[]Save, seen map[string]struct{}, records []recordEntry, shape saveShape) {
	for _, r := range records {
		itemURL, _ := r.Value[shape.urlField].(string)
		if itemURL == "" {
			slog.Warn("discovercrawl: skipping "+shape.kind+" save without "+shape.urlField, "uri", r.URI)
			continue
		}
		if _, dup := seen[itemURL]; dup {
			continue
		}
		seen[itemURL] = struct{}{}
		var feedURL string
		if shape.feedField != "" {
			feedURL, _ = r.Value[shape.feedField].(string)
			feedURL = feedkey.Normalize(feedURL)
		}
		createdAt, _ := r.Value[shape.timeField].(string)
		*out = append(*out, Save{Kind: shape.kind, ItemURL: itemURL, FeedURL: feedURL, CreatedAt: createdAt})
	}
}

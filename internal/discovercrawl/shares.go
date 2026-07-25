package discovercrawl

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/feedkey"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
)

// site.standard recommends join a lazy blue.morgen.feed.share sidecar by document AT-URI for their comment; app.skyreader is unpublished but assumed shape-identical (SPEC <sync-architecture>).
const (
	morgenShareCollection       = lexicon.Share
	standardRecommendCollection = standardfeed.CollectionRecommend
	skyreaderShareCollection    = "app.skyreader.social.share"
)

// Share is one decoded share/recommend record from a followed DID's repo; a standardfeed entry merges recommend + sidecar, mirroring the join internal/sync/reconcile_shares.go performs on the user's own repo.
type Share struct {
	Kind      string // "rss" | "standardfeed" | "skyreader"
	ItemURL   string
	Document  string
	FeedURL   string
	Comment   string
	CreatedAt string
}

type shareRecord struct {
	Rkey      string
	ItemURL   string
	Document  string
	FeedURL   string
	Comment   string
	CreatedAt string
}

type recommendRecord struct {
	Rkey      string
	Document  string
	CreatedAt string
}

// CrawlShares fetches share/recommend records across the three collections and merges standardfeed pairs by document; malformed records are skipped and logged, never fatal.
func (c *Client) CrawlShares(ctx context.Context, did syntax.DID) ([]Share, error) {
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

	morgenRecords, err := pageRecords(ctx, client, repo, morgenShareCollection)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", morgenShareCollection, did, err)
	}
	recommendRecords, err := pageRecords(ctx, client, repo, standardRecommendCollection)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", standardRecommendCollection, did, err)
	}
	skyreaderRecords, err := pageRecords(ctx, client, repo, skyreaderShareCollection)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", skyreaderShareCollection, did, err)
	}

	return MergeShares(map[string][]recordEntry{
		morgenShareCollection:       morgenRecords,
		standardRecommendCollection: recommendRecords,
		skyreaderShareCollection:    skyreaderRecords,
	}), nil
}

// decodeShareRecord decodes blue.morgen.feed.share and app.skyreader.social.share records (the latter unpublished, assumed shape-identical, mirroring app.skyreader.social.follow); itemUrl is required.
func decodeShareRecord(r recordEntry) (shareRecord, bool) {
	itemURL, _ := r.Value["itemUrl"].(string)
	if itemURL == "" {
		return shareRecord{}, false
	}
	document, _ := r.Value["document"].(string)
	feedURL, _ := r.Value["feedUrl"].(string)
	comment, _ := r.Value["comment"].(string)
	createdAt, _ := r.Value["createdAt"].(string)
	return shareRecord{
		Rkey:      atprepo.RkeyFromATURI(r.URI),
		ItemURL:   itemURL,
		Document:  document,
		FeedURL:   feedkey.Normalize(feedURL),
		Comment:   comment,
		CreatedAt: createdAt,
	}, true
}

// decodeRecommendRecord decodes a site.standard.graph.recommend record; document is required.
func decodeRecommendRecord(r recordEntry) (recommendRecord, bool) {
	document, _ := r.Value["document"].(string)
	if document == "" {
		return recommendRecord{}, false
	}
	createdAt, _ := r.Value["createdAt"].(string)
	return recommendRecord{
		Rkey:      atprepo.RkeyFromATURI(r.URI),
		Document:  document,
		CreatedAt: createdAt,
	}, true
}

package discovercrawl

import (
	"context"
	"fmt"
	"log/slog"

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

	// Document-bearing morgen shares are recommend sidecars; document-less ones are standalone rss shares.
	var rssShares []shareRecord
	sidecarByDoc := map[string]shareRecord{}
	for _, r := range morgenRecords {
		sr, ok := decodeShareRecord(r)
		if !ok {
			slog.Warn("discovercrawl: skipping malformed share", "uri", r.URI)
			continue
		}
		if sr.Document == "" {
			rssShares = append(rssShares, sr)
			continue
		}
		// Newest sidecar wins (same rule as the own-repo reconcile) so a sync/PATCH race doesn't shadow the latest comment.
		if cur, ok := sidecarByDoc[sr.Document]; !ok || sr.Rkey > cur.Rkey {
			sidecarByDoc[sr.Document] = sr
		}
	}

	// Canonical recommend per document = smallest rkey (TID ⇒ earliest created), same rule as reconcileShares.
	canonicalByDoc := map[string]recommendRecord{}
	for _, r := range recommendRecords {
		rec, ok := decodeRecommendRecord(r)
		if !ok {
			slog.Warn("discovercrawl: skipping malformed recommend", "uri", r.URI)
			continue
		}
		if cur, ok := canonicalByDoc[rec.Document]; !ok || rec.Rkey < cur.Rkey {
			canonicalByDoc[rec.Document] = rec
		}
	}

	out := make([]Share, 0, len(canonicalByDoc)+len(rssShares)+len(skyreaderRecords))
	for doc, rec := range canonicalByDoc {
		s := Share{Kind: "standardfeed", Document: doc, CreatedAt: rec.CreatedAt}
		if sc, ok := sidecarByDoc[doc]; ok {
			s.ItemURL = sc.ItemURL
			s.Comment = sc.Comment
			s.FeedURL = sc.FeedURL
		}
		out = append(out, s)
	}

	seen := map[string]struct{}{} // itemUrl dedupe within rss/skyreader shares
	for _, sr := range rssShares {
		if _, dup := seen[sr.ItemURL]; dup {
			continue
		}
		seen[sr.ItemURL] = struct{}{}
		out = append(out, Share{Kind: "rss", ItemURL: sr.ItemURL, FeedURL: sr.FeedURL, Comment: sr.Comment, CreatedAt: sr.CreatedAt})
	}
	for _, r := range skyreaderRecords {
		sr, ok := decodeShareRecord(r)
		if !ok {
			slog.Warn("discovercrawl: skipping malformed skyreader share", "uri", r.URI)
			continue
		}
		if _, dup := seen[sr.ItemURL]; dup {
			continue
		}
		seen[sr.ItemURL] = struct{}{}
		out = append(out, Share{Kind: "skyreader", ItemURL: sr.ItemURL, FeedURL: sr.FeedURL, Comment: sr.Comment, CreatedAt: sr.CreatedAt})
	}
	return out, nil
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

package discovercrawl

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/lexicon"
)

// Reader-network follow collections (SPEC <discovery> one-hop People candidates). app.skyreader.social.follow is unpublished, assumed shape-identical to blue.morgen.graph.follow by convention.
const (
	morgenFollowCollection    = lexicon.Follow
	skyreaderFollowCollection = "app.skyreader.social.follow"
)

// ReaderNetworkFollow is one person a followed DID follows inside the reader network, a one-hop People candidate.
type ReaderNetworkFollow struct {
	DID string
}

// CrawlReaderNetworkFollows lists did's blue.morgen and skyreader follows, deduped by DID. A blue.morgen fetch failure is fatal (our own lexicon); a skyreader failure degrades to none, like CrawlAdjacentFollows's Tangled handling.
func (c *Client) CrawlReaderNetworkFollows(ctx context.Context, did syntax.DID) ([]ReaderNetworkFollow, error) {
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

	seen := make(map[string]struct{})

	morgenRecords, err := pageRecords(ctx, client, repo, morgenFollowCollection)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", morgenFollowCollection, did, err)
	}
	addReaderNetworkFollows(seen, morgenRecords, repo)

	skyreaderRecords, err := pageRecords(ctx, client, repo, skyreaderFollowCollection)
	if err != nil {
		slog.Warn("discovercrawl: skyreader follow crawl failed, skipping", "did", repo, "err", err)
	} else {
		addReaderNetworkFollows(seen, skyreaderRecords, repo)
	}

	out := make([]ReaderNetworkFollow, 0, len(seen))
	for d := range seen {
		out = append(out, ReaderNetworkFollow{DID: d})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DID < out[j].DID })
	return out, nil
}

// addReaderNetworkFollows reuses decodeAdjacentFollow: both follow shapes share the same subject+createdAt fields.
func addReaderNetworkFollows(seen map[string]struct{}, records []recordEntry, self string) {
	for _, r := range records {
		subject, ok := decodeAdjacentFollow(r)
		if !ok {
			slog.Warn("discovercrawl: skipping reader-network follow without subject", "uri", r.URI)
			continue
		}
		if subject == self {
			continue
		}
		seen[subject] = struct{}{}
	}
}

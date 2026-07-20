package discovercrawl

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// Weak trust tier follow collections (SPEC <discovery> Trust tiers). sh.tangled.graph.follow is unpublished; its shape was verified empirically against a live Tangled repo.
const (
	bskyFollowCollection    = "app.bsky.graph.follow"
	tangledFollowCollection = "sh.tangled.graph.follow"
)

// maxAdjacentCrawlDIDs caps PDS fetches per Discover page load so a follow list of thousands can't turn into thousands of fetches. SPEC <discovery> bounded crawl.
const maxAdjacentCrawlDIDs = 200

// AdjacentFollow is one person the user follows on an adjacent social graph (Bluesky or Tangled), outside the reader network.
type AdjacentFollow struct {
	DID     string
	Network string // "bluesky" | "tangled"
}

// CrawlAdjacentFollows lists did's Bluesky and Tangled follows, dedupes by DID (a collision keeps Bluesky, an arbitrary but deterministic tie-break), and caps at maxAdjacentCrawlDIDs.
// Truncation keeps a deterministic (sorted DID) subset and logs the drop, never silent.
// A Tangled fetch failure degrades to zero Tangled follows (unpublished lexicon); a Bluesky failure is fatal, matching this package's other Crawl methods.
// Wrapped by CachedAdjacentFollowCrawler (SelfCrawlTTL) at the call site; this method itself always hits the network.
func (c *Client) CrawlAdjacentFollows(ctx context.Context, did syntax.DID) ([]AdjacentFollow, error) {
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

	byDID := make(map[string]string) // did -> network, first write wins

	bskyRecords, err := pageRecords(ctx, client, repo, bskyFollowCollection)
	if err != nil {
		return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", bskyFollowCollection, did, err)
	}
	addAdjacentFollows(byDID, bskyRecords, "bluesky", repo)

	tangledRecords, err := pageRecords(ctx, client, repo, tangledFollowCollection)
	if err != nil {
		// Unverified/unpublished lexicon: degrade rather than fail the whole crawl.
		slog.Warn("discovercrawl: tangled follow crawl failed, skipping", "did", repo, "err", err)
	} else {
		addAdjacentFollows(byDID, tangledRecords, "tangled", repo)
	}

	dids := make([]string, 0, len(byDID))
	for d := range byDID {
		dids = append(dids, d)
	}
	sort.Strings(dids)

	if len(dids) > maxAdjacentCrawlDIDs {
		dropped := len(dids) - maxAdjacentCrawlDIDs
		slog.Warn("discovercrawl: adjacent-graph crawl truncated",
			"did", repo, "kept", maxAdjacentCrawlDIDs, "dropped", dropped, "bound", maxAdjacentCrawlDIDs)
		dids = dids[:maxAdjacentCrawlDIDs]
	}

	out := make([]AdjacentFollow, 0, len(dids))
	for _, d := range dids {
		out = append(out, AdjacentFollow{DID: d, Network: byDID[d]})
	}
	return out, nil
}

func addAdjacentFollows(byDID map[string]string, records []recordEntry, network, self string) {
	for _, r := range records {
		subject, ok := decodeAdjacentFollow(r)
		if !ok {
			slog.Warn("discovercrawl: skipping "+network+" follow without subject", "uri", r.URI)
			continue
		}
		if subject == self {
			continue
		}
		if _, dup := byDID[subject]; !dup {
			byDID[subject] = network
		}
	}
}

func decodeAdjacentFollow(r recordEntry) (string, bool) {
	subject, _ := r.Value["subject"].(string)
	if subject == "" {
		return "", false
	}
	return subject, true
}

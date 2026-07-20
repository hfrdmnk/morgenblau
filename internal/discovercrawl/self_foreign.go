package discovercrawl

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// ForeignApp names which foreign reader app a self-crawled subscription record came from. SPEC <discovery> The user's own foreign records.
type ForeignApp string

const (
	ForeignAppSkyreader ForeignApp = "skyreader"
	ForeignAppGlean     ForeignApp = "glean"
)

// ForeignSubscription is one of the user's own Skyreader/Glean subscription
// records, found by crawling their own repo.
type ForeignSubscription struct {
	Subscription
	App ForeignApp
}

var ownForeignCollections = [...]struct {
	collection string
	app        ForeignApp
}{
	{skyreaderSubscriptionCollection, ForeignAppSkyreader},
	{gleanSubscriptionCollection, ForeignAppGlean},
}

// CrawlOwnForeignSubscriptions lists did's Skyreader and Glean subscriptions only, never blue.morgen (already Tier-1) or standardfeed (co-owned, always-synced). SPEC <discovery> The user's own foreign records.
// Wrapped by CachedOwnForeignCrawler (SelfCrawlTTL) at the call site; this method itself always hits the network.
func (c *Client) CrawlOwnForeignSubscriptions(ctx context.Context, did syntax.DID) ([]ForeignSubscription, error) {
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

	pubCache := make(map[string]Subscription)
	seen := make(map[string]ForeignSubscription)
	for _, f := range ownForeignCollections {
		records, err := pageRecords(ctx, client, repo, f.collection)
		if err != nil {
			return nil, fmt.Errorf("discovercrawl: list %s for %s: %w", f.collection, did, err)
		}
		for _, r := range records {
			sub, ok := c.decodeFeedURLRecord(ctx, f.collection, r, pubCache)
			if !ok {
				continue
			}
			if _, dup := seen[sub.Key]; dup {
				continue
			}
			seen[sub.Key] = ForeignSubscription{Subscription: sub, App: f.app}
		}
	}

	out := make([]ForeignSubscription, 0, len(seen))
	for _, s := range seen {
		out = append(out, s)
	}
	return out, nil
}

// Package discoverbatch enumerates reader-network repos from the relay, folds a
// repo's records into its strongest per-source signal, and owns the aggregate
// write helpers. Ingestion itself lives in internal/tapingest.
// SPEC <discovery> Global/Trending.
package discoverbatch

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
	"morgenblau/internal/lexicon"
)

// DefaultRelayHost is the relay enumerated for repo discovery; configurable via env, see internal/server wiring.
const DefaultRelayHost = "https://relay1.us-east.bsky.network"

// EnumerationCollections are the reader-network collections diffed into per-source trending signals. SPEC <lexicons> External Lexicons.
var EnumerationCollections = []string{
	"blue.morgen.feed.subscription",
	"blue.morgen.feed.save",
	"blue.morgen.feed.share",
	"app.skyreader.feed.subscription",
	"app.skyreader.feed.saved",
	"app.skyreader.social.share",
	"at.glean.subscription",
	"at.glean.like",
	"site.standard.publication",
	"site.standard.graph.subscription",
	"site.standard.graph.recommend",
}

// FollowEnumerationCollections feed per-DID follower counts for People trending; deliberately excludes app.bsky.graph.follow. SPEC <discovery>.
var FollowEnumerationCollections = []string{
	lexicon.Follow,
	"app.skyreader.social.follow",
}

// relayReposPerPage matches the relay's own page-size max.
const relayReposPerPage = 1000

// NormalizeRelayHost prepends https:// since atclient requires a scheme-qualified URL and .env.example configures a bare host.
func NormalizeRelayHost(host string) string {
	if strings.Contains(host, "://") {
		return host
	}
	return "https://" + host
}

type relayRepo struct {
	DID string `json:"did"`
}

type listReposByCollectionResp struct {
	Repos  []relayRepo `json:"repos"`
	Cursor string      `json:"cursor"`
}

// relayLister is the seam EnumerateCollection uses so tests swap in an httptest fake instead of a live relay.
type relayLister interface {
	Get(ctx context.Context, endpoint syntax.NSID, params map[string]any, out any) error
}

// EnumerateCollection pages listReposByCollection to exhaustion. httpClient must be the SSRF-safe client; every outbound fetch here goes through it.
func EnumerateCollection(ctx context.Context, httpClient *http.Client, relayEndpoint, collection string) ([]string, error) {
	return enumerateCollection(ctx, atxrpc.New(relayEndpoint, httpClient), collection)
}

func enumerateCollection(ctx context.Context, client relayLister, collection string) ([]string, error) {
	var (
		out    []string
		cursor string
	)
	for {
		var resp listReposByCollectionResp
		params := map[string]any{
			"collection": collection,
			"limit":      relayReposPerPage,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := client.Get(ctx, syntax.NSID("com.atproto.sync.listReposByCollection"), params, &resp); err != nil {
			return nil, fmt.Errorf("discoverbatch: listReposByCollection %s: %w", collection, err)
		}
		for _, r := range resp.Repos {
			out = append(out, r.DID)
		}
		if resp.Cursor == "" {
			return out, nil
		}
		cursor = resp.Cursor
	}
}

// EnumerateAll returns the deduped union of DIDs across collections; one collection's failure aborts the whole call, and the caller retries on the next tick.
func EnumerateAll(ctx context.Context, httpClient *http.Client, relayEndpoint string, collections []string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	for _, coll := range collections {
		dids, err := EnumerateCollection(ctx, httpClient, relayEndpoint, coll)
		if err != nil {
			return nil, err
		}
		for _, d := range dids {
			if _, dup := seen[d]; dup {
				continue
			}
			seen[d] = struct{}{}
			out = append(out, d)
		}
	}
	return out, nil
}

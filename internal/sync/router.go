package sync

import (
	"context"
	"strings"
)

// SourceRouter implements FeedFetcher over the whole Tier-2 catalog. The
// catalog key is self-describing — publication at-uris carry the at://
// scheme, feed URLs are http(s) — so no DB read is needed to route, and
// GlobalRefresher, fetchAll, and StartFetchOneFeed stay untouched.
type SourceRouter struct {
	rss      FeedFetcher
	standard FeedFetcher
}

func NewSourceRouter(rss, standard FeedFetcher) *SourceRouter {
	return &SourceRouter{rss: rss, standard: standard}
}

func (r *SourceRouter) FetchAndStore(ctx context.Context, key string) error {
	if strings.HasPrefix(key, "at://") {
		return r.standard.FetchAndStore(ctx, key)
	}
	return r.rss.FetchAndStore(ctx, key)
}

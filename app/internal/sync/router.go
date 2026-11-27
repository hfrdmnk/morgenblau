package sync

import (
	"context"
	"strings"
)

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

package sync

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"morgenblau/internal/fetcher"
)

type FeedLister interface {
	ListFollowedFeedURLs(ctx context.Context) ([]string, error)
}

const globalFetchConcurrency = fetcher.WorkerPoolSize

type GlobalRefresher struct {
	lister  FeedLister
	fetcher FeedFetcher
}

func NewGlobalRefresher(lister FeedLister, fetcher FeedFetcher) *GlobalRefresher {
	return &GlobalRefresher{lister: lister, fetcher: fetcher}
}

func (r *GlobalRefresher) RefreshAll(ctx context.Context) (int, error) {
	urls, err := r.lister.ListFollowedFeedURLs(ctx)
	if err != nil {
		return 0, err
	}
	if len(urls) == 0 {
		return 0, nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(globalFetchConcurrency)
	for _, u := range urls {
		u := u
		g.Go(func() error {
			if err := r.fetcher.FetchAndStore(gctx, u); err != nil {
				slog.Debug("global feed fetch: feed failed", "url", u, "err", err)
			}
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return len(urls), err
	}
	return len(urls), nil
}

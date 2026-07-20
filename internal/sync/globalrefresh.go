package sync

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"morgenblau/internal/fetcher"
)

// FeedLister is the slice of the catalog the global refresher reads.
type FeedLister interface {
	ListAllFeedURLs(ctx context.Context) ([]string, error)
}

// globalFetchConcurrency is aligned with fetcher.WorkerPoolSize so a sweep never spawns more than the fetcher will admit.
const globalFetchConcurrency = fetcher.WorkerPoolSize

// GlobalRefresher re-fetches every feed in the shared Tier-2 catalog on a timer; it maps to no user, so it stays out of the jobs tracker and logs failures instead of a refresh pill.
type GlobalRefresher struct {
	lister  FeedLister
	fetcher FeedFetcher
}

func NewGlobalRefresher(lister FeedLister, fetcher FeedFetcher) *GlobalRefresher {
	return &GlobalRefresher{lister: lister, fetcher: fetcher}
}

// RefreshAll fans out FetchAndStore with bounded concurrency; per-feed failures are logged and swallowed so one dead feed can't abort the sweep.
func (r *GlobalRefresher) RefreshAll(ctx context.Context) (int, error) {
	urls, err := r.lister.ListAllFeedURLs(ctx)
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
	// Closures never return errors, so only ctx cancellation ends the sweep early.
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return len(urls), err
	}
	return len(urls), nil
}

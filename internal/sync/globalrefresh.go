package sync

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"morgenblau/internal/fetcher"
)

// FeedLister is the slice of the catalog the global refresher reads.
// Satisfied by *db.Queries via ListAllFeedURLs.
type FeedLister interface {
	ListAllFeedURLs(ctx context.Context) ([]string, error)
}

// globalFetchConcurrency caps in-flight goroutines per sweep. Aligned with
// fetcher.WorkerPoolSize so we never spawn more than the fetcher will admit.
const globalFetchConcurrency = fetcher.WorkerPoolSize

// GlobalRefresher re-fetches every feed in the shared Tier-2 catalog on a
// timer. It maps to no user, so it stays out of the jobs tracker — failures
// are logged, not surfaced to a refresh pill.
type GlobalRefresher struct {
	lister  FeedLister
	fetcher FeedFetcher
}

func NewGlobalRefresher(lister FeedLister, fetcher FeedFetcher) *GlobalRefresher {
	return &GlobalRefresher{lister: lister, fetcher: fetcher}
}

// RefreshAll lists every catalog feed and fans out FetchAndStore with bounded
// concurrency. Returns the number of feeds attempted. The error is non-nil
// only for a list-query failure or a cancelled context; per-feed fetch
// failures are logged and swallowed so one dead feed can't abort the sweep.
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
	// Closures never return errors, so Wait only unblocks once every fetch
	// settles. The sole thing that ends a sweep early is ctx cancellation.
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return len(urls), err
	}
	return len(urls), nil
}

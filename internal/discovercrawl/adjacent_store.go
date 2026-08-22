package discovercrawl

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// SelfCrawlTTL bounds the session user's own-repo crawl caches (adjacent follows, foreign subscriptions), shorter than DefaultTTL because a stale cache here hides the viewer's own recent actions, not a followed person's.
const SelfCrawlTTL = time.Hour

// AdjacentFollowSource is the seam CachedAdjacentFollowCrawler wraps; *Client satisfies it.
type AdjacentFollowSource interface {
	CrawlAdjacentFollows(ctx context.Context, did syntax.DID) ([]AdjacentFollow, error)
}

// AdjacentFollowCacheReader is what CrawlAdjacentFollows reads from; wire to the reader pool.
type AdjacentFollowCacheReader interface {
	GetDiscoverCrawlAdjacentState(ctx context.Context, did string) (db.DiscoverCrawlAdjacentState, error)
	ListDiscoverCrawlAdjacentFollows(ctx context.Context, did string) ([]db.DiscoverCrawlAdjacentFollow, error)
}

// AdjacentFollowCacheWriter is the slice used inside the write transaction.
type AdjacentFollowCacheWriter interface {
	DeleteDiscoverCrawlAdjacentFollows(ctx context.Context, did string) error
	InsertDiscoverCrawlAdjacentFollow(ctx context.Context, arg db.InsertDiscoverCrawlAdjacentFollowParams) error
	UpsertDiscoverCrawlAdjacentState(ctx context.Context, arg db.UpsertDiscoverCrawlAdjacentStateParams) error
}

// CachedAdjacentFollowCrawler wraps the session user's own adjacent-graph crawl with SelfCrawlTTL caching; satisfies api.AdjacentFollowCrawler.
type CachedAdjacentFollowCrawler struct {
	crawler AdjacentFollowSource
	reader  AdjacentFollowCacheReader
	runTx   func(ctx context.Context, fn func(AdjacentFollowCacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time
}

// NewCachedAdjacentFollowCrawler builds the wrapper. Without WithTxRunner it degrades to crawl-always, like the other Cached* constructors.
func NewCachedAdjacentFollowCrawler(crawler AdjacentFollowSource, reader AdjacentFollowCacheReader, ttl time.Duration) *CachedAdjacentFollowCrawler {
	return &CachedAdjacentFollowCrawler{
		crawler: crawler,
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		runTx: func(ctx context.Context, fn func(AdjacentFollowCacheWriter) error) error {
			return errors.New("discovercrawl: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner wires cache writes to the writer pool in one transaction.
func (c *CachedAdjacentFollowCrawler) WithTxRunner(w *sql.DB) *CachedAdjacentFollowCrawler {
	c.runTx = func(ctx context.Context, fn func(AdjacentFollowCacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// CrawlAdjacentFollows serves cached follows within ttl, else crawls fresh; the write tx opens only after the crawl returns.
func (c *CachedAdjacentFollowCrawler) CrawlAdjacentFollows(ctx context.Context, did syntax.DID) ([]AdjacentFollow, error) {
	return cachedFetch[AdjacentFollow]{
		label: "adjacent-follows",
		ttl:   c.ttl,
		now:   c.now,
		getState: func(ctx context.Context, didStr string) (string, error) {
			state, err := c.reader.GetDiscoverCrawlAdjacentState(ctx, didStr)
			if err != nil {
				return "", err
			}
			return state.FetchedAt, nil
		},
		listCached: func(ctx context.Context, didStr string) ([]AdjacentFollow, error) {
			rows, err := c.reader.ListDiscoverCrawlAdjacentFollows(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToAdjacentFollows(rows), nil
		},
		crawl: c.crawler.CrawlAdjacentFollows,
		store: func(ctx context.Context, didStr string, results []AdjacentFollow, fetchedAt string) error {
			return c.runTx(ctx, func(w AdjacentFollowCacheWriter) error {
				return storeAdjacentFollowResults(ctx, w, didStr, results, fetchedAt)
			})
		},
	}.fetch(ctx, did)
}

func storeAdjacentFollowResults(ctx context.Context, w AdjacentFollowCacheWriter, did string, results []AdjacentFollow, fetchedAt string) error {
	if err := w.DeleteDiscoverCrawlAdjacentFollows(ctx, did); err != nil {
		return err
	}
	for _, f := range results {
		if err := w.InsertDiscoverCrawlAdjacentFollow(ctx, db.InsertDiscoverCrawlAdjacentFollowParams{
			Did:        did,
			SubjectDid: f.DID,
			Network:    f.Network,
			FetchedAt:  fetchedAt,
		}); err != nil {
			return err
		}
	}
	return w.UpsertDiscoverCrawlAdjacentState(ctx, db.UpsertDiscoverCrawlAdjacentStateParams{Did: did, FetchedAt: fetchedAt})
}

func rowsToAdjacentFollows(rows []db.DiscoverCrawlAdjacentFollow) []AdjacentFollow {
	out := make([]AdjacentFollow, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdjacentFollow{DID: r.SubjectDid, Network: r.Network})
	}
	return out
}

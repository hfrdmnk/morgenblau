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

// OwnForeignSubscriptionSource is the seam CachedOwnForeignCrawler wraps; *Client satisfies it.
type OwnForeignSubscriptionSource interface {
	CrawlOwnForeignSubscriptions(ctx context.Context, did syntax.DID) ([]ForeignSubscription, error)
}

// OwnForeignSubscriptionCacheReader is what CrawlOwnForeignSubscriptions reads from; wire to the reader pool.
type OwnForeignSubscriptionCacheReader interface {
	GetDiscoverCrawlOwnForeignState(ctx context.Context, did string) (db.DiscoverCrawlOwnForeignState, error)
	ListDiscoverCrawlOwnForeignSubscriptions(ctx context.Context, did string) ([]db.DiscoverCrawlOwnForeignSubscription, error)
}

// OwnForeignSubscriptionCacheWriter is the slice used inside the write transaction.
type OwnForeignSubscriptionCacheWriter interface {
	DeleteDiscoverCrawlOwnForeignSubscriptions(ctx context.Context, did string) error
	InsertDiscoverCrawlOwnForeignSubscription(ctx context.Context, arg db.InsertDiscoverCrawlOwnForeignSubscriptionParams) error
	UpsertDiscoverCrawlOwnForeignState(ctx context.Context, arg db.UpsertDiscoverCrawlOwnForeignStateParams) error
}

// CachedOwnForeignCrawler wraps the session user's own foreign-subscription crawl with SelfCrawlTTL caching; satisfies api.OwnForeignSubscriptionCrawler.
type CachedOwnForeignCrawler struct {
	crawler OwnForeignSubscriptionSource
	reader  OwnForeignSubscriptionCacheReader
	runTx   func(ctx context.Context, fn func(OwnForeignSubscriptionCacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time
}

// NewCachedOwnForeignCrawler builds the wrapper. Without WithTxRunner it degrades to crawl-always, like the other Cached* constructors.
func NewCachedOwnForeignCrawler(crawler OwnForeignSubscriptionSource, reader OwnForeignSubscriptionCacheReader, ttl time.Duration) *CachedOwnForeignCrawler {
	return &CachedOwnForeignCrawler{
		crawler: crawler,
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		runTx: func(ctx context.Context, fn func(OwnForeignSubscriptionCacheWriter) error) error {
			return errors.New("discovercrawl: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner wires cache writes to the writer pool in one transaction.
func (c *CachedOwnForeignCrawler) WithTxRunner(w *sql.DB) *CachedOwnForeignCrawler {
	c.runTx = func(ctx context.Context, fn func(OwnForeignSubscriptionCacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// CrawlOwnForeignSubscriptions serves cached subscriptions within ttl, else crawls fresh; the write tx opens only after the crawl returns.
func (c *CachedOwnForeignCrawler) CrawlOwnForeignSubscriptions(ctx context.Context, did syntax.DID) ([]ForeignSubscription, error) {
	return cachedFetch[ForeignSubscription]{
		label: "own-foreign-subscriptions",
		ttl:   c.ttl,
		now:   c.now,
		getState: func(ctx context.Context, didStr string) (string, error) {
			state, err := c.reader.GetDiscoverCrawlOwnForeignState(ctx, didStr)
			if err != nil {
				return "", err
			}
			return state.FetchedAt, nil
		},
		listCached: func(ctx context.Context, didStr string) ([]ForeignSubscription, error) {
			rows, err := c.reader.ListDiscoverCrawlOwnForeignSubscriptions(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToForeignSubscriptions(rows), nil
		},
		crawl: c.crawler.CrawlOwnForeignSubscriptions,
		store: func(ctx context.Context, didStr string, results []ForeignSubscription, fetchedAt string) error {
			return c.runTx(ctx, func(w OwnForeignSubscriptionCacheWriter) error {
				return storeOwnForeignSubscriptionResults(ctx, w, didStr, results, fetchedAt)
			})
		},
	}.fetch(ctx, did)
}

func storeOwnForeignSubscriptionResults(ctx context.Context, w OwnForeignSubscriptionCacheWriter, did string, results []ForeignSubscription, fetchedAt string) error {
	if err := w.DeleteDiscoverCrawlOwnForeignSubscriptions(ctx, did); err != nil {
		return err
	}
	for _, s := range results {
		if err := w.InsertDiscoverCrawlOwnForeignSubscription(ctx, db.InsertDiscoverCrawlOwnForeignSubscriptionParams{
			Did:          did,
			CanonicalKey: s.Key,
			Kind:         s.Kind,
			App:          string(s.App),
			Title:        nilIfEmpty(s.Title),
			SiteUrl:      nilIfEmpty(s.SiteURL),
			CreatedAt:    nilIfEmpty(s.CreatedAt),
			FetchedAt:    fetchedAt,
		}); err != nil {
			return err
		}
	}
	return w.UpsertDiscoverCrawlOwnForeignState(ctx, db.UpsertDiscoverCrawlOwnForeignStateParams{Did: did, FetchedAt: fetchedAt})
}

func rowsToForeignSubscriptions(rows []db.DiscoverCrawlOwnForeignSubscription) []ForeignSubscription {
	out := make([]ForeignSubscription, 0, len(rows))
	for _, r := range rows {
		out = append(out, ForeignSubscription{
			Subscription: Subscription{
				Key:       r.CanonicalKey,
				Kind:      r.Kind,
				Title:     derefString(r.Title),
				SiteURL:   derefString(r.SiteUrl),
				CreatedAt: derefString(r.CreatedAt),
			},
			App: ForeignApp(r.App),
		})
	}
	return out
}

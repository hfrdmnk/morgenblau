package discovercrawl

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// DefaultTTL matches the daily-digest cadence: re-crawl at most once a day (SPEC <discovery>).
const DefaultTTL = 24 * time.Hour

// Crawler is the seam CachedCrawler depends on so cache-logic tests inject a fake instead of a fake PDS.
type Crawler interface {
	Crawl(ctx context.Context, did syntax.DID) ([]Subscription, error)
}

// CacheReader is what FetchSubscriptions reads from; wire it to the reader pool (SPEC <discovery>).
type CacheReader interface {
	GetDiscoverCrawlState(ctx context.Context, followedDid string) (db.DiscoverCrawlState, error)
	ListDiscoverCrawlSubscriptions(ctx context.Context, followedDid string) ([]db.DiscoverCrawlSubscription, error)
}

// CacheWriter is the slice used inside the write transaction.
type CacheWriter interface {
	DeleteDiscoverCrawlSubscriptions(ctx context.Context, followedDid string) error
	InsertDiscoverCrawlSubscription(ctx context.Context, arg db.InsertDiscoverCrawlSubscriptionParams) error
	UpsertDiscoverCrawlState(ctx context.Context, arg db.UpsertDiscoverCrawlStateParams) error
}

// CachedCrawler wraps a Crawler with a TTL'd local cache (SPEC <discovery> "Personal" acquisition).
// The write transaction opens only after Crawl returns, never held open across the network fetch.
type CachedCrawler struct {
	crawler Crawler
	reader  CacheReader
	runTx   func(ctx context.Context, fn func(CacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time
}

// NewCachedCrawler builds a cache wrapper with the given TTL; without WithTxRunner, cache writes are skipped and logged rather than panicking, degrading to "always crawl".
func NewCachedCrawler(crawler Crawler, reader CacheReader, ttl time.Duration) *CachedCrawler {
	return &CachedCrawler{
		crawler: crawler,
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		runTx: func(ctx context.Context, fn func(CacheWriter) error) error {
			return errors.New("discovercrawl: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits cache writes in one transaction on the writer pool.
func (c *CachedCrawler) WithTxRunner(w *sql.DB) *CachedCrawler {
	c.runTx = func(ctx context.Context, fn func(CacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// FetchSubscriptions serves from cache within ttl, otherwise crawls fresh and refreshes the cache.
func (c *CachedCrawler) FetchSubscriptions(ctx context.Context, did syntax.DID) ([]Subscription, error) {
	didStr := did.String()

	state, err := c.reader.GetDiscoverCrawlState(ctx, didStr)
	switch {
	case err == nil:
		fetchedAt, perr := time.Parse(time.RFC3339, state.FetchedAt)
		if perr == nil && c.now().Sub(fetchedAt) < c.ttl {
			rows, err := c.reader.ListDiscoverCrawlSubscriptions(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToSubscriptions(rows), nil
		}
	case errors.Is(err, sql.ErrNoRows):
		// Never crawled: falls through to a fresh crawl below.
	default:
		return nil, err
	}

	results, err := c.crawler.Crawl(ctx, did)
	if err != nil {
		return nil, err
	}

	fetchedAt := c.now().UTC().Format(time.RFC3339)
	if err := c.runTx(ctx, func(w CacheWriter) error {
		return storeResults(ctx, w, didStr, results, fetchedAt)
	}); err != nil {
		// Cache-write failure isn't fatal: the next call just re-crawls instead of using a stale cache.
		slog.Warn("discovercrawl: cache write failed", "did", didStr, "err", err)
	}
	return results, nil
}

func storeResults(ctx context.Context, w CacheWriter, did string, results []Subscription, fetchedAt string) error {
	if err := w.DeleteDiscoverCrawlSubscriptions(ctx, did); err != nil {
		return err
	}
	for _, s := range results {
		if err := w.InsertDiscoverCrawlSubscription(ctx, db.InsertDiscoverCrawlSubscriptionParams{
			FollowedDid:  did,
			CanonicalKey: s.Key,
			Kind:         s.Kind,
			Title:        nilIfEmpty(s.Title),
			SiteUrl:      nilIfEmpty(s.SiteURL),
			CreatedAt:    nilIfEmpty(s.CreatedAt),
			FetchedAt:    fetchedAt,
		}); err != nil {
			return err
		}
	}
	return w.UpsertDiscoverCrawlState(ctx, db.UpsertDiscoverCrawlStateParams{FollowedDid: did, FetchedAt: fetchedAt})
}

func rowsToSubscriptions(rows []db.DiscoverCrawlSubscription) []Subscription {
	out := make([]Subscription, 0, len(rows))
	for _, r := range rows {
		out = append(out, Subscription{
			Key:       r.CanonicalKey,
			Kind:      r.Kind,
			Title:     derefString(r.Title),
			SiteURL:   derefString(r.SiteUrl),
			CreatedAt: derefString(r.CreatedAt),
		})
	}
	return out
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

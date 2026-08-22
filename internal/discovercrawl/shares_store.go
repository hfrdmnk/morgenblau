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

// ShareCrawler is the seam CachedShareCrawler depends on so cache-logic tests inject a fake instead of a fake PDS.
type ShareCrawler interface {
	CrawlShares(ctx context.Context, did syntax.DID) ([]Share, error)
}

// ShareCacheReader is what FetchShares reads from; wire it to the reader pool.
type ShareCacheReader interface {
	GetDiscoverCrawlShareState(ctx context.Context, followedDid string) (db.DiscoverCrawlShareState, error)
	ListDiscoverCrawlShares(ctx context.Context, followedDid string) ([]db.DiscoverCrawlShare, error)
	ListDiscoverCrawlShareStatesByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlShareState, error)
	ListDiscoverCrawlSharesByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlShare, error)
}

// ShareCacheWriter is the slice used inside the write transaction.
type ShareCacheWriter interface {
	DeleteDiscoverCrawlShares(ctx context.Context, followedDid string) error
	InsertDiscoverCrawlShare(ctx context.Context, arg db.InsertDiscoverCrawlShareParams) error
	UpsertDiscoverCrawlShareState(ctx context.Context, arg db.UpsertDiscoverCrawlShareStateParams) error
}

// CachedShareCrawler mirrors CachedCrawler's TTL'd cache posture (see store.go) for shares instead of subscriptions.
// The write transaction opens only after Crawl returns, never held open across the network fetch.
type CachedShareCrawler struct {
	crawler ShareCrawler
	reader  ShareCacheReader
	runTx   func(ctx context.Context, fn func(ShareCacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time
}

// NewCachedShareCrawler builds a share cache wrapper with the given TTL; see NewCachedCrawler for the degrade-without-WithTxRunner posture.
func NewCachedShareCrawler(crawler ShareCrawler, reader ShareCacheReader, ttl time.Duration) *CachedShareCrawler {
	return &CachedShareCrawler{
		crawler: crawler,
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		runTx: func(ctx context.Context, fn func(ShareCacheWriter) error) error {
			return errors.New("discovercrawl: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits cache writes in one transaction on the writer pool.
func (c *CachedShareCrawler) WithTxRunner(w *sql.DB) *CachedShareCrawler {
	c.runTx = func(ctx context.Context, fn func(ShareCacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// FetchShares serves from cache within ttl, otherwise crawls fresh and refreshes the cache.
func (c *CachedShareCrawler) FetchShares(ctx context.Context, did syntax.DID) ([]Share, error) {
	return cachedFetch[Share]{
		label: "shares",
		ttl:   c.ttl,
		now:   c.now,
		getState: func(ctx context.Context, didStr string) (string, error) {
			state, err := c.reader.GetDiscoverCrawlShareState(ctx, didStr)
			if err != nil {
				return "", err
			}
			return state.FetchedAt, nil
		},
		listCached: func(ctx context.Context, didStr string) ([]Share, error) {
			rows, err := c.reader.ListDiscoverCrawlShares(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToShares(rows), nil
		},
		crawl: c.crawler.CrawlShares,
		store: func(ctx context.Context, didStr string, results []Share, fetchedAt string) error {
			return c.runTx(ctx, func(w ShareCacheWriter) error {
				return storeShareResults(ctx, w, didStr, results, fetchedAt)
			})
		},
	}.fetch(ctx, did)
}

// FetchSharesBatch is FetchShares over a whole fan-out; see FetchSubscriptionsBatch for the read/crawl split and degrade posture.
func (c *CachedShareCrawler) FetchSharesBatch(ctx context.Context, dids []string) map[string][]Share {
	return batchCache[Share]{
		label: "shares",
		ttl:   c.ttl,
		now:   c.now,
		fetchedAtByDID: func(ctx context.Context, dids []string) (map[string]string, error) {
			rows, err := c.reader.ListDiscoverCrawlShareStatesByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			out := make(map[string]string, len(rows))
			for _, r := range rows {
				out[r.FollowedDid] = r.FetchedAt
			}
			return out, nil
		},
		rowsByDID: func(ctx context.Context, dids []string) (map[string][]Share, error) {
			rows, err := c.reader.ListDiscoverCrawlSharesByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			return groupRowsByDID(rows, func(r db.DiscoverCrawlShare) string { return r.FollowedDid }, rowsToShares), nil
		},
		fetchOne: c.FetchShares,
	}.fetch(ctx, dids)
}

func storeShareResults(ctx context.Context, w ShareCacheWriter, did string, results []Share, fetchedAt string) error {
	if err := w.DeleteDiscoverCrawlShares(ctx, did); err != nil {
		return err
	}
	for _, s := range results {
		key := shareDedupeKey(s)
		if key == "" {
			continue
		}
		if err := w.InsertDiscoverCrawlShare(ctx, db.InsertDiscoverCrawlShareParams{
			FollowedDid: did,
			DedupeKey:   key,
			Kind:        s.Kind,
			ItemUrl:     nilIfEmpty(s.ItemURL),
			Document:    nilIfEmpty(s.Document),
			FeedUrl:     nilIfEmpty(s.FeedURL),
			Comment:     nilIfEmpty(s.Comment),
			CreatedAt:   s.CreatedAt,
			FetchedAt:   fetchedAt,
		}); err != nil {
			return err
		}
	}
	return w.UpsertDiscoverCrawlShareState(ctx, db.UpsertDiscoverCrawlShareStateParams{FollowedDid: did, FetchedAt: fetchedAt})
}

// shareDedupeKey mirrors CrawlShares' merge identity (document for standardfeed, itemUrl otherwise); a share with neither is dropped rather than colliding with every other row at an empty key.
func shareDedupeKey(s Share) string {
	if s.Kind == "standardfeed" {
		return s.Document
	}
	return s.ItemURL
}

func rowsToShares(rows []db.DiscoverCrawlShare) []Share {
	out := make([]Share, 0, len(rows))
	for _, r := range rows {
		out = append(out, Share{
			Kind:      r.Kind,
			ItemURL:   derefString(r.ItemUrl),
			Document:  derefString(r.Document),
			FeedURL:   derefString(r.FeedUrl),
			Comment:   derefString(r.Comment),
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

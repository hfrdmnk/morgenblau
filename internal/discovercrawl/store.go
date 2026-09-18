package discovercrawl

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// DefaultTTL matches the daily-digest cadence: re-crawl at most once a day (SPEC <discovery>).
const DefaultTTL = 24 * time.Hour

// BatchCrawlFanout bounds concurrent per-DID crawls inside one batch fetch so a cold cache with hundreds of candidates can't flood SQLite's single writer connection or blow the server's 30s WriteTimeout.
const BatchCrawlFanout = 8

// batchCache is the shape the four Fetch*Batch methods share: one IN-read of the freshness states,
// one IN-read of the cached rows for whatever is still fresh, then a bounded fan-out that re-crawls
// the rest through the per-DID path (which owns the cache write-through).
type batchCache[T any] struct {
	label          string
	ttl            time.Duration
	now            func() time.Time
	fetchedAtByDID func(ctx context.Context, dids []string) (map[string]string, error)
	rowsByDID      func(ctx context.Context, dids []string) (map[string][]T, error)
	fetchOne       func(ctx context.Context, did syntax.DID) ([]T, error)
}

// fetch never returns an error: every failure degrades the affected DIDs to a nil entry so one bad repo, or one bad read, can't cost the caller the whole page.
func (b batchCache[T]) fetch(ctx context.Context, dids []string) map[string][]T {
	out := make(map[string][]T, len(dids))
	unique := make([]string, 0, len(dids))
	for _, did := range dids {
		if _, seen := out[did]; seen || did == "" {
			continue
		}
		out[did] = nil
		unique = append(unique, did)
	}
	if len(unique) == 0 {
		return out
	}

	fetchedAt, err := b.fetchedAtByDID(ctx, unique)
	if err != nil {
		// Same outcome the per-DID path produced when a state read failed: no signals, no crawl stampede against a struggling DB.
		slog.Warn("discovercrawl: batch cache state read failed", "cache", b.label, "dids", len(unique), "err", err)
		return out
	}

	var fresh, stale []string
	for _, did := range unique {
		if b.isFresh(fetchedAt[did]) {
			fresh = append(fresh, did)
			continue
		}
		stale = append(stale, did)
	}

	if len(fresh) > 0 {
		if rows, err := b.rowsByDID(ctx, fresh); err != nil {
			slog.Warn("discovercrawl: batch cache row read failed", "cache", b.label, "dids", len(fresh), "err", err)
		} else {
			for did, list := range rows {
				out[did] = list
			}
		}
	}

	// crawled[i] is written only by goroutine i, so the fold below needs no lock.
	crawled := make([][]T, len(stale))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(BatchCrawlFanout)
	for i, didStr := range stale {
		i, didStr := i, didStr
		g.Go(func() error {
			did, err := syntax.ParseDID(didStr)
			if err != nil {
				slog.Warn("discovercrawl: malformed did in batch", "cache", b.label, "did", didStr, "err", err)
				return nil
			}
			results, err := b.fetchOne(gctx, did)
			if err != nil {
				slog.Warn("discovercrawl: batch crawl failed", "cache", b.label, "did", didStr, "err", err)
				return nil
			}
			crawled[i] = results
			return nil
		})
	}
	_ = g.Wait()
	for i, didStr := range stale {
		out[didStr] = crawled[i]
	}
	return out
}

func (b batchCache[T]) isFresh(fetchedAt string) bool {
	if fetchedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, fetchedAt)
	return err == nil && b.now().Sub(t) < b.ttl
}

// groupRowsByDID folds a batched IN-read into per-DID slices, converting each group with the cache's existing row mapper.
func groupRowsByDID[R, T any](rows []R, did func(R) string, convert func([]R) []T) map[string][]T {
	byDID := map[string][]R{}
	for _, r := range rows {
		key := did(r)
		byDID[key] = append(byDID[key], r)
	}
	out := make(map[string][]T, len(byDID))
	for k, group := range byDID {
		out[k] = convert(group)
	}
	return out
}

// cachedFetch is the singular-path twin of batchCache: serve from cache within ttl, else crawl and write through; CachedX methods build one per call from closures so the generic never sees their concrete types.
type cachedFetch[T any] struct {
	label      string
	ttl        time.Duration
	now        func() time.Time
	getState   func(ctx context.Context, didStr string) (fetchedAt string, err error)
	listCached func(ctx context.Context, didStr string) ([]T, error)
	crawl      func(ctx context.Context, did syntax.DID) ([]T, error)
	store      func(ctx context.Context, didStr string, results []T, fetchedAt string) error
	// postProcess runs on whichever result set is served, cached or freshly crawled; nil means identity.
	postProcess func([]T) []T
}

func (c cachedFetch[T]) fetch(ctx context.Context, did syntax.DID) ([]T, error) {
	didStr := did.String()

	fetchedAtStr, err := c.getState(ctx, didStr)
	switch {
	case err == nil:
		fetchedAt, perr := time.Parse(time.RFC3339, fetchedAtStr)
		if perr == nil && c.now().Sub(fetchedAt) < c.ttl {
			rows, err := c.listCached(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return c.apply(rows), nil
		}
	case errors.Is(err, sql.ErrNoRows):
		// Never crawled: falls through to a fresh crawl below.
	default:
		return nil, err
	}

	results, err := c.crawl(ctx, did)
	if err != nil {
		return nil, err
	}

	fetchedAt := c.now().UTC().Format(time.RFC3339)
	if err := c.store(ctx, didStr, results, fetchedAt); err != nil {
		// Cache-write failure isn't fatal: the next call just re-crawls instead of using a stale cache.
		slog.Warn("discovercrawl: cache write failed", "cache", c.label, "did", didStr, "err", err)
	}
	return c.apply(results), nil
}

func (c cachedFetch[T]) apply(rows []T) []T {
	if c.postProcess == nil {
		return rows
	}
	return c.postProcess(rows)
}

// Crawler is the seam CachedCrawler depends on so cache-logic tests inject a fake instead of a fake PDS.
type Crawler interface {
	Crawl(ctx context.Context, did syntax.DID) ([]Subscription, error)
}

// CacheReader is what FetchSubscriptions reads from; wire it to the reader pool (SPEC <discovery>).
type CacheReader interface {
	GetDiscoverCrawlState(ctx context.Context, followedDid string) (db.DiscoverCrawlState, error)
	ListDiscoverCrawlSubscriptions(ctx context.Context, followedDid string) ([]db.DiscoverCrawlSubscription, error)
	ListDiscoverCrawlStatesByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlState, error)
	ListDiscoverCrawlSubscriptionsByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlSubscription, error)
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
	return cachedFetch[Subscription]{
		label: "subscriptions",
		ttl:   c.ttl,
		now:   c.now,
		getState: func(ctx context.Context, didStr string) (string, error) {
			state, err := c.reader.GetDiscoverCrawlState(ctx, didStr)
			if err != nil {
				return "", err
			}
			return state.FetchedAt, nil
		},
		listCached: func(ctx context.Context, didStr string) ([]Subscription, error) {
			rows, err := c.reader.ListDiscoverCrawlSubscriptions(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToSubscriptions(rows), nil
		},
		crawl: c.crawler.Crawl,
		store: func(ctx context.Context, didStr string, results []Subscription, fetchedAt string) error {
			return c.runTx(ctx, func(w CacheWriter) error {
				return storeResults(ctx, w, didStr, results, fetchedAt)
			})
		},
	}.fetch(ctx, did)
}

// FetchSubscriptionsBatch serves a whole fan-out of followed repos from two IN-reads, crawling only the DIDs whose cache is stale or missing. A DID whose crawl fails maps to nil rather than failing the batch, mirroring how the handlers degraded one candidate at a time.
func (c *CachedCrawler) FetchSubscriptionsBatch(ctx context.Context, dids []string) map[string][]Subscription {
	return batchCache[Subscription]{
		label: "subscriptions",
		ttl:   c.ttl,
		now:   c.now,
		fetchedAtByDID: func(ctx context.Context, dids []string) (map[string]string, error) {
			rows, err := c.reader.ListDiscoverCrawlStatesByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			out := make(map[string]string, len(rows))
			for _, r := range rows {
				out[r.FollowedDid] = r.FetchedAt
			}
			return out, nil
		},
		rowsByDID: func(ctx context.Context, dids []string) (map[string][]Subscription, error) {
			rows, err := c.reader.ListDiscoverCrawlSubscriptionsByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			return groupRowsByDID(rows, func(r db.DiscoverCrawlSubscription) string { return r.FollowedDid }, rowsToSubscriptions), nil
		},
		fetchOne: c.FetchSubscriptions,
	}.fetch(ctx, dids)
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

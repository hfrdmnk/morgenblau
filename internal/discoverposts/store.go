package discoverposts

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// DefaultTTL is shorter than discovercrawl's 24h cache: a posts preview should feel fresher.
const DefaultTTL = 6 * time.Hour

// postsBackoff paces retries on transient preview-fetch failures; shorter than discovercrawl's
// resolutionBackoff (1h->168h) since a preview should recover the same day.
var postsBackoff = backoff.Policy{Steps: backoff.Exponential(5*time.Minute, 2, 2*time.Hour)}

// postsFetchConcurrencyLimit bounds concurrent live preview fetches, same cap as discoverCrawlFanoutLimit
// in internal/api/discover_sources.go, so a cold cache with many candidates can't flood SQLite's writer.
const postsFetchConcurrencyLimit = 8

// previewFetchBudget bounds the live fetch step (semaphore wait + network call): the server's
// WriteTimeout is 30s and safehttp allows up to 30s per request, so without a shorter budget a slow
// upstream holds the connection open instead of degrading to a stale/empty preview.
const previewFetchBudget = 10 * time.Second

// PostsFetcher is the slice of *Fetcher CachedFetcher depends on.
type PostsFetcher interface {
	FetchPosts(ctx context.Context, key string) (FetchResult, error)
}

// PostsCacheReader is the slice of *db.Queries CachedFetcher.FetchPosts reads from. Wire to the reader pool.
type PostsCacheReader interface {
	GetDiscoverSourcePostsState(ctx context.Context, sourceKey string) (db.DiscoverSourcePostsState, error)
	ListDiscoverSourcePosts(ctx context.Context, sourceKey string) ([]db.DiscoverSourcePost, error)
}

// PostsCacheWriter is the slice used inside the write transaction.
type PostsCacheWriter interface {
	DeleteDiscoverSourcePosts(ctx context.Context, sourceKey string) error
	InsertDiscoverSourcePost(ctx context.Context, arg db.InsertDiscoverSourcePostParams) error
	UpsertDiscoverSourcePostsState(ctx context.Context, arg db.UpsertDiscoverSourcePostsStateParams) error
	RecordDiscoverSourcePostsFailure(ctx context.Context, arg db.RecordDiscoverSourcePostsFailureParams) error
}

// CachedFetcher wraps a PostsFetcher with a TTL cache, negative caching with backoff, stale-while-error,
// a per-key singleflight collapse, and a concurrency cap, mirroring discovercrawl's resolveCachedOnce.
type CachedFetcher struct {
	fetcher PostsFetcher
	reader  PostsCacheReader
	runTx   func(ctx context.Context, fn func(PostsCacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time

	fetchGroup  singleflight.Group
	sem         chan struct{}
	fetchBudget time.Duration
}

// NewCachedFetcher builds a posts-preview cache wrapper. Without WithTxRunner it silently degrades to fetch-always.
func NewCachedFetcher(fetcher PostsFetcher, reader PostsCacheReader, ttl time.Duration) *CachedFetcher {
	return &CachedFetcher{
		fetcher:     fetcher,
		reader:      reader,
		ttl:         ttl,
		now:         time.Now,
		sem:         make(chan struct{}, postsFetchConcurrencyLimit),
		fetchBudget: previewFetchBudget,
		runTx: func(ctx context.Context, fn func(PostsCacheWriter) error) error {
			return errors.New("discoverposts: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner wires cache writes to the writer pool in one transaction (*db.Queries satisfies PostsCacheWriter).
func (c *CachedFetcher) WithTxRunner(w *sql.DB) *CachedFetcher {
	c.runTx = func(ctx context.Context, fn func(PostsCacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// FetchPosts serves cached rows when fresh or backing off, else fetches live; the write tx opens only
// after the fetch returns. Concurrent calls for the same key collapse to one live fetch.
func (c *CachedFetcher) FetchPosts(ctx context.Context, key string) ([]Post, error) {
	state, haveState, err := c.loadState(ctx, key)
	if err != nil {
		return nil, err
	}
	now := c.now()

	if haveState && state.FetchedAt != nil {
		if fetchedAt, perr := time.Parse(time.RFC3339, *state.FetchedAt); perr == nil && now.Sub(fetchedAt) < c.ttl {
			return c.cachedRows(ctx, key)
		}
	}

	if haveState && state.NextRetryAt != nil {
		if nextRetry, perr := time.Parse(time.RFC3339, *state.NextRetryAt); perr == nil && now.Before(nextRetry) {
			if state.FetchedAt == nil {
				return []Post{}, nil
			}
			return c.cachedRows(ctx, key)
		}
	}

	v, err, _ := c.fetchGroup.Do(key, func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(ctx, c.fetchBudget)
		defer cancel()
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
		case <-fetchCtx.Done():
			return nil, fetchCtx.Err()
		}
		// ctx (not fetchCtx) carries the cache write: a fetch that exhausts the budget must still get its failure recorded.
		return c.liveFetchAndStore(ctx, fetchCtx, key, state, haveState)
	})
	if err != nil {
		return nil, err
	}
	return v.([]Post), nil
}

func (c *CachedFetcher) loadState(ctx context.Context, key string) (db.DiscoverSourcePostsState, bool, error) {
	state, err := c.reader.GetDiscoverSourcePostsState(ctx, key)
	switch {
	case err == nil:
		return state, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return db.DiscoverSourcePostsState{}, false, nil
	default:
		return db.DiscoverSourcePostsState{}, false, err
	}
}

func (c *CachedFetcher) cachedRows(ctx context.Context, key string) ([]Post, error) {
	rows, err := c.reader.ListDiscoverSourcePosts(ctx, key)
	if err != nil {
		return nil, err
	}
	return rowsToPosts(rows), nil
}

func (c *CachedFetcher) liveFetchAndStore(ctx, fetchCtx context.Context, key string, prior db.DiscoverSourcePostsState, havePrior bool) ([]Post, error) {
	result, err := c.fetcher.FetchPosts(fetchCtx, key)
	now := c.now()
	if err != nil {
		return c.recordFailure(ctx, key, prior, havePrior, err, now)
	}
	return c.recordSuccess(ctx, key, result, now)
}

// recordFailure persists the backoff step and, for stale-while-error, serves the last-known-good rows
// instead of the error; a source with no prior success has nothing to serve, so the error propagates.
func (c *CachedFetcher) recordFailure(ctx context.Context, key string, prior db.DiscoverSourcePostsState, havePrior bool, fetchErr error, now time.Time) ([]Post, error) {
	var failures int64 = 1
	if havePrior {
		failures = prior.FailureCount + 1
	}
	nextRetryAt := now.Add(postsBackoff.Delay(int(failures))).UTC().Format(time.RFC3339)
	if err := c.runTx(ctx, func(w PostsCacheWriter) error {
		return w.RecordDiscoverSourcePostsFailure(ctx, db.RecordDiscoverSourcePostsFailureParams{
			SourceKey:    key,
			FailureCount: failures,
			NextRetryAt:  &nextRetryAt,
		})
	}); err != nil {
		slog.Warn("discoverposts: posts failure record write failed", "key", key, "err", err)
	}

	if havePrior && prior.FetchedAt != nil {
		if rows, rerr := c.cachedRows(ctx, key); rerr == nil {
			return rows, nil
		} else {
			slog.Warn("discoverposts: stale-while-error cache read failed", "key", key, "err", rerr)
		}
	}
	return nil, fetchErr
}

func (c *CachedFetcher) recordSuccess(ctx context.Context, key string, result FetchResult, now time.Time) ([]Post, error) {
	fetchedAt := now.UTC().Format(time.RFC3339)
	if err := c.runTx(ctx, func(w PostsCacheWriter) error {
		return storePostsResult(ctx, w, key, result, fetchedAt)
	}); err != nil {
		// The fetch already succeeded; a cache-write failure just means the next call refetches, never fatal.
		slog.Warn("discoverposts: posts cache write failed", "key", key, "err", err)
	}
	return result.Posts, nil
}

func storePostsResult(ctx context.Context, w PostsCacheWriter, key string, result FetchResult, fetchedAt string) error {
	if err := w.DeleteDiscoverSourcePosts(ctx, key); err != nil {
		return err
	}
	seenKeys := make(map[string]bool, len(result.Posts))
	position := int64(0)
	for _, p := range result.Posts {
		// post_key is UNIQUE; same-source posts sharing an identity (no URL, matching title+publishedAt) would otherwise fail the whole insert.
		if seenKeys[p.Key] {
			continue
		}
		seenKeys[p.Key] = true
		if err := w.InsertDiscoverSourcePost(ctx, db.InsertDiscoverSourcePostParams{
			SourceKey:   key,
			Position:    position,
			Title:       p.Title,
			PublishedAt: nilIfEmpty(p.PublishedAt),
			Url:         nilIfEmpty(p.URL),
			PostKey:     p.Key,
		}); err != nil {
			return err
		}
		position++
	}
	return w.UpsertDiscoverSourcePostsState(ctx, db.UpsertDiscoverSourcePostsStateParams{
		SourceKey:  key,
		FetchedAt:  &fetchedAt,
		FaviconUrl: nilIfEmpty(result.FaviconURL),
	})
}

func rowsToPosts(rows []db.DiscoverSourcePost) []Post {
	out := make([]Post, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToPost(r))
	}
	return out
}

func rowToPost(r db.DiscoverSourcePost) Post {
	return Post{
		Title:       r.Title,
		PublishedAt: derefString(r.PublishedAt),
		URL:         derefString(r.Url),
		Key:         r.PostKey,
	}
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

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

// ReaderFollowCrawler is the slice of *Client the cache wrapper depends on, a narrow seam for fake-based tests instead of a live PDS.
type ReaderFollowCrawler interface {
	CrawlReaderNetworkFollows(ctx context.Context, did syntax.DID) ([]ReaderNetworkFollow, error)
}

// ReaderFollowCacheReader is the slice of *db.Queries FetchReaderNetworkFollows reads from. Wire to the reader pool.
type ReaderFollowCacheReader interface {
	GetDiscoverCrawlFollowState(ctx context.Context, followedDid string) (db.DiscoverCrawlFollowState, error)
	ListDiscoverCrawlFollows(ctx context.Context, followedDid string) ([]db.DiscoverCrawlFollow, error)
	ListDiscoverCrawlFollowStatesByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlFollowState, error)
	ListDiscoverCrawlFollowsByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlFollow, error)
}

// ReaderFollowCacheWriter is the slice used inside the write transaction.
type ReaderFollowCacheWriter interface {
	DeleteDiscoverCrawlFollows(ctx context.Context, followedDid string) error
	InsertDiscoverCrawlFollow(ctx context.Context, arg db.InsertDiscoverCrawlFollowParams) error
	UpsertDiscoverCrawlFollowState(ctx context.Context, arg db.UpsertDiscoverCrawlFollowStateParams) error
}

// CachedReaderFollowCrawler wraps a ReaderFollowCrawler with the same TTL cache posture as CachedCrawler; feeds the People tab's one-hop candidates. SPEC <discovery> Personal acquisition.
type CachedReaderFollowCrawler struct {
	crawler ReaderFollowCrawler
	reader  ReaderFollowCacheReader
	runTx   func(ctx context.Context, fn func(ReaderFollowCacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time
}

// NewCachedReaderFollowCrawler builds a reader-network follow cache wrapper. Without WithTxRunner it silently degrades to crawl-always, like NewCachedCrawler.
func NewCachedReaderFollowCrawler(crawler ReaderFollowCrawler, reader ReaderFollowCacheReader, ttl time.Duration) *CachedReaderFollowCrawler {
	return &CachedReaderFollowCrawler{
		crawler: crawler,
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		runTx: func(ctx context.Context, fn func(ReaderFollowCacheWriter) error) error {
			return errors.New("discovercrawl: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner wires cache writes to the writer pool in one transaction (*db.Queries satisfies ReaderFollowCacheWriter).
func (c *CachedReaderFollowCrawler) WithTxRunner(w *sql.DB) *CachedReaderFollowCrawler {
	c.runTx = func(ctx context.Context, fn func(ReaderFollowCacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// FetchReaderNetworkFollows returns cached follows when within ttl, else crawls fresh; the write tx opens only after the crawl returns.
func (c *CachedReaderFollowCrawler) FetchReaderNetworkFollows(ctx context.Context, did syntax.DID) ([]ReaderNetworkFollow, error) {
	return cachedFetch[ReaderNetworkFollow]{
		label: "reader-network-follows",
		ttl:   c.ttl,
		now:   c.now,
		getState: func(ctx context.Context, didStr string) (string, error) {
			state, err := c.reader.GetDiscoverCrawlFollowState(ctx, didStr)
			if err != nil {
				return "", err
			}
			return state.FetchedAt, nil
		},
		listCached: func(ctx context.Context, didStr string) ([]ReaderNetworkFollow, error) {
			rows, err := c.reader.ListDiscoverCrawlFollows(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToReaderNetworkFollows(rows), nil
		},
		crawl: c.crawler.CrawlReaderNetworkFollows,
		store: func(ctx context.Context, didStr string, results []ReaderNetworkFollow, fetchedAt string) error {
			return c.runTx(ctx, func(w ReaderFollowCacheWriter) error {
				return storeReaderNetworkFollowResults(ctx, w, didStr, results, fetchedAt)
			})
		},
	}.fetch(ctx, did)
}

// FetchReaderNetworkFollowsBatch is FetchReaderNetworkFollows over the viewer's whole follow list; see FetchSubscriptionsBatch for the read/crawl split and degrade posture.
func (c *CachedReaderFollowCrawler) FetchReaderNetworkFollowsBatch(ctx context.Context, dids []string) map[string][]ReaderNetworkFollow {
	return batchCache[ReaderNetworkFollow]{
		label: "reader-network-follows",
		ttl:   c.ttl,
		now:   c.now,
		fetchedAtByDID: func(ctx context.Context, dids []string) (map[string]string, error) {
			rows, err := c.reader.ListDiscoverCrawlFollowStatesByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			out := make(map[string]string, len(rows))
			for _, r := range rows {
				out[r.FollowedDid] = r.FetchedAt
			}
			return out, nil
		},
		rowsByDID: func(ctx context.Context, dids []string) (map[string][]ReaderNetworkFollow, error) {
			rows, err := c.reader.ListDiscoverCrawlFollowsByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			return groupRowsByDID(rows, func(r db.DiscoverCrawlFollow) string { return r.FollowedDid }, rowsToReaderNetworkFollows), nil
		},
		fetchOne: c.FetchReaderNetworkFollows,
	}.fetch(ctx, dids)
}

func storeReaderNetworkFollowResults(ctx context.Context, w ReaderFollowCacheWriter, did string, results []ReaderNetworkFollow, fetchedAt string) error {
	if err := w.DeleteDiscoverCrawlFollows(ctx, did); err != nil {
		return err
	}
	for _, f := range results {
		if err := w.InsertDiscoverCrawlFollow(ctx, db.InsertDiscoverCrawlFollowParams{
			FollowedDid: did,
			SubjectDid:  f.DID,
			FetchedAt:   fetchedAt,
		}); err != nil {
			return err
		}
	}
	return w.UpsertDiscoverCrawlFollowState(ctx, db.UpsertDiscoverCrawlFollowStateParams{FollowedDid: did, FetchedAt: fetchedAt})
}

func rowsToReaderNetworkFollows(rows []db.DiscoverCrawlFollow) []ReaderNetworkFollow {
	out := make([]ReaderNetworkFollow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReaderNetworkFollow{DID: r.SubjectDid})
	}
	return out
}

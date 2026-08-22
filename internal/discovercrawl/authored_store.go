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

// AuthoredCrawler is the slice of *Client the cache wrapper depends on, a narrow seam for fake-based tests instead of a live PDS.
type AuthoredCrawler interface {
	CrawlAuthoredPublications(ctx context.Context, did syntax.DID) ([]AuthoredPublication, error)
}

// AuthoredCacheReader is the slice of *db.Queries FetchAuthoredPublications reads from. Wire to the reader pool.
type AuthoredCacheReader interface {
	GetDiscoverCrawlAuthoredState(ctx context.Context, followedDid string) (db.DiscoverCrawlAuthoredState, error)
	ListDiscoverCrawlAuthored(ctx context.Context, followedDid string) ([]db.DiscoverCrawlAuthored, error)
	ListDiscoverCrawlAuthoredStatesByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlAuthoredState, error)
	ListDiscoverCrawlAuthoredByDids(ctx context.Context, dids []string) ([]db.DiscoverCrawlAuthored, error)
}

// AuthoredCacheWriter is the slice used inside the write transaction.
type AuthoredCacheWriter interface {
	DeleteDiscoverCrawlAuthored(ctx context.Context, followedDid string) error
	InsertDiscoverCrawlAuthored(ctx context.Context, arg db.InsertDiscoverCrawlAuthoredParams) error
	UpsertDiscoverCrawlAuthoredState(ctx context.Context, arg db.UpsertDiscoverCrawlAuthoredStateParams) error
}

// CachedAuthoredCrawler wraps an AuthoredCrawler with the same TTL cache posture as CachedCrawler. SPEC <discovery> Personal acquisition.
type CachedAuthoredCrawler struct {
	crawler AuthoredCrawler
	reader  AuthoredCacheReader
	runTx   func(ctx context.Context, fn func(AuthoredCacheWriter) error) error
	ttl     time.Duration
	now     func() time.Time
}

// NewCachedAuthoredCrawler builds an authored-publication cache wrapper. Without WithTxRunner it silently degrades to crawl-always, like NewCachedCrawler.
func NewCachedAuthoredCrawler(crawler AuthoredCrawler, reader AuthoredCacheReader, ttl time.Duration) *CachedAuthoredCrawler {
	return &CachedAuthoredCrawler{
		crawler: crawler,
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		runTx: func(ctx context.Context, fn func(AuthoredCacheWriter) error) error {
			return errors.New("discovercrawl: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner wires cache writes to the writer pool in one transaction (*db.Queries satisfies AuthoredCacheWriter).
func (c *CachedAuthoredCrawler) WithTxRunner(w *sql.DB) *CachedAuthoredCrawler {
	c.runTx = func(ctx context.Context, fn func(AuthoredCacheWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return c
}

// FetchAuthoredPublications returns cached publications when within ttl, else crawls fresh; the write tx opens only after the crawl returns.
// filterVerified is the one behavioral divergence from the other five CachedX types: it gates both the cached and freshly crawled paths alike.
func (c *CachedAuthoredCrawler) FetchAuthoredPublications(ctx context.Context, did syntax.DID) ([]AuthoredPublication, error) {
	return cachedFetch[AuthoredPublication]{
		label: "authored",
		ttl:   c.ttl,
		now:   c.now,
		getState: func(ctx context.Context, didStr string) (string, error) {
			state, err := c.reader.GetDiscoverCrawlAuthoredState(ctx, didStr)
			if err != nil {
				return "", err
			}
			return state.FetchedAt, nil
		},
		listCached: func(ctx context.Context, didStr string) ([]AuthoredPublication, error) {
			rows, err := c.reader.ListDiscoverCrawlAuthored(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return rowsToAuthored(rows), nil
		},
		crawl: c.crawler.CrawlAuthoredPublications,
		store: func(ctx context.Context, didStr string, results []AuthoredPublication, fetchedAt string) error {
			return c.runTx(ctx, func(w AuthoredCacheWriter) error {
				return storeAuthoredResults(ctx, w, didStr, results, fetchedAt)
			})
		},
		postProcess: filterVerified,
	}.fetch(ctx, did)
}

// FetchAuthoredPublicationsBatch is FetchAuthoredPublications over a whole fan-out; see FetchSubscriptionsBatch for the read/crawl split and degrade posture. The cached path keeps filterVerified as its single gate, same as the per-DID one.
func (c *CachedAuthoredCrawler) FetchAuthoredPublicationsBatch(ctx context.Context, dids []string) map[string][]AuthoredPublication {
	return batchCache[AuthoredPublication]{
		label: "authored",
		ttl:   c.ttl,
		now:   c.now,
		fetchedAtByDID: func(ctx context.Context, dids []string) (map[string]string, error) {
			rows, err := c.reader.ListDiscoverCrawlAuthoredStatesByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			out := make(map[string]string, len(rows))
			for _, r := range rows {
				out[r.FollowedDid] = r.FetchedAt
			}
			return out, nil
		},
		rowsByDID: func(ctx context.Context, dids []string) (map[string][]AuthoredPublication, error) {
			rows, err := c.reader.ListDiscoverCrawlAuthoredByDids(ctx, dids)
			if err != nil {
				return nil, err
			}
			convert := func(group []db.DiscoverCrawlAuthored) []AuthoredPublication {
				return filterVerified(rowsToAuthored(group))
			}
			return groupRowsByDID(rows, func(r db.DiscoverCrawlAuthored) string { return r.FollowedDid }, convert), nil
		},
		fetchOne: c.FetchAuthoredPublications,
	}.fetch(ctx, dids)
}

// filterVerified drops anything but a verified outcome; CrawlAuthoredPublications only ever emits verified today, but this stays the single gate the signal passes through so a stale pre-migration row (or a future non-verified outcome) can never leak into the API/batch consumers.
func filterVerified(pubs []AuthoredPublication) []AuthoredPublication {
	out := make([]AuthoredPublication, 0, len(pubs))
	for _, p := range pubs {
		if p.Verification == verifiedOutcome {
			out = append(out, p)
		}
	}
	return out
}

func storeAuthoredResults(ctx context.Context, w AuthoredCacheWriter, did string, results []AuthoredPublication, fetchedAt string) error {
	if err := w.DeleteDiscoverCrawlAuthored(ctx, did); err != nil {
		return err
	}
	for _, a := range results {
		if err := w.InsertDiscoverCrawlAuthored(ctx, db.InsertDiscoverCrawlAuthoredParams{
			FollowedDid:     did,
			CanonicalKey:    a.Key,
			Kind:            a.Kind,
			Title:           nilIfEmpty(a.Title),
			SiteUrl:         nilIfEmpty(a.SiteURL),
			LastPublishedAt: nilIfEmpty(a.LastPublishedAt),
			FetchedAt:       fetchedAt,
			Verification:    a.Verification,
		}); err != nil {
			return err
		}
	}
	return w.UpsertDiscoverCrawlAuthoredState(ctx, db.UpsertDiscoverCrawlAuthoredStateParams{FollowedDid: did, FetchedAt: fetchedAt})
}

func rowsToAuthored(rows []db.DiscoverCrawlAuthored) []AuthoredPublication {
	out := make([]AuthoredPublication, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuthoredPublication{
			Key:             r.CanonicalKey,
			Kind:            r.Kind,
			Title:           derefString(r.Title),
			SiteURL:         derefString(r.SiteUrl),
			LastPublishedAt: derefString(r.LastPublishedAt),
			Verification:    r.Verification,
		})
	}
	return out
}

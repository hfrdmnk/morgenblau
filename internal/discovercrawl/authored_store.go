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
func (c *CachedAuthoredCrawler) FetchAuthoredPublications(ctx context.Context, did syntax.DID) ([]AuthoredPublication, error) {
	didStr := did.String()

	state, err := c.reader.GetDiscoverCrawlAuthoredState(ctx, didStr)
	switch {
	case err == nil:
		fetchedAt, perr := time.Parse(time.RFC3339, state.FetchedAt)
		if perr == nil && c.now().Sub(fetchedAt) < c.ttl {
			rows, err := c.reader.ListDiscoverCrawlAuthored(ctx, didStr)
			if err != nil {
				return nil, err
			}
			return filterVerified(rowsToAuthored(rows)), nil
		}
	case errors.Is(err, sql.ErrNoRows):
		// Never crawled, fall through to a fresh crawl.
	default:
		return nil, err
	}

	results, err := c.crawler.CrawlAuthoredPublications(ctx, did)
	if err != nil {
		return nil, err
	}

	fetchedAt := c.now().UTC().Format(time.RFC3339)
	if err := c.runTx(ctx, func(w AuthoredCacheWriter) error {
		return storeAuthoredResults(ctx, w, didStr, results, fetchedAt)
	}); err != nil {
		// Network already succeeded; a cache-write failure just means the next call re-crawls, never fatal.
		slog.Warn("discovercrawl: authored cache write failed", "did", didStr, "err", err)
	}
	return filterVerified(results), nil
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

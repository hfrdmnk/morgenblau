// Package discoverfavicon resolves and caches a discover-candidate's favicon on demand, independent of
// the posts-preview fetch, so a collapsed card still gets a real favicon once one becomes discoverable.
package discoverfavicon

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/singleflight"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/favicon"
	"morgenblau/internal/safehttp"
)

// discoveryBudget bounds resolving+fetching a favicon, leaving headroom under the server's 30s
// WriteTimeout and the proxy's own 5s icon fetch that follows a successful Resolve.
const discoveryBudget = 8 * time.Second

// resolveConcurrencyLimit bounds concurrent live discovery, same cap as discoverposts' postsFetchConcurrencyLimit.
const resolveConcurrencyLimit = 8

// faviconBackoff paces retries on transient discovery failures, independent of discoverposts' postsBackoff.
var faviconBackoff = backoff.Policy{Steps: backoff.Exponential(15*time.Minute, 2, 24*time.Hour)}

// PublicationResolutionReader is the first (best) site source: the discover-crawl resolution cache.
type PublicationResolutionReader interface {
	GetDiscoverPublicationResolutionByCanonicalKey(ctx context.Context, canonicalKey *string) (db.DiscoverPublicationResolution, error)
}

// SubscriptionSiteReader is the second site source: a followed repo's crawled subscription record.
type SubscriptionSiteReader interface {
	GetDiscoverCrawlSubscriptionSiteURLByKey(ctx context.Context, canonicalKey string) (*string, error)
}

// TrendingSiteReader is the last site source: a network-wide trending signal.
type TrendingSiteReader interface {
	GetDiscoverTrendingSignalTitle(ctx context.Context, sourceKey string) (db.GetDiscoverTrendingSignalTitleRow, error)
}

// FaviconDiscoverer lets tests stub favicon discovery.
type FaviconDiscoverer interface {
	Discover(ctx context.Context, siteURL string) (string, error)
}

// StateReader is the reader-pool slice Resolve checks for an existing favicon backoff window.
type StateReader interface {
	GetDiscoverSourcePostsState(ctx context.Context, sourceKey string) (db.DiscoverSourcePostsState, error)
}

// StateWriter is the writer-pool slice used inside the write transaction.
type StateWriter interface {
	UpsertDiscoverSourceFaviconURL(ctx context.Context, arg db.UpsertDiscoverSourceFaviconURLParams) error
	RecordDiscoverSourceFaviconDiscoveryFailure(ctx context.Context, arg db.RecordDiscoverSourceFaviconDiscoveryFailureParams) error
}

type httpDiscoverer struct{ client *http.Client }

// NewHTTPDiscoverer builds the production FaviconDiscoverer, mirroring discoverposts' favicon client (10s timeout, 5 redirects).
func NewHTTPDiscoverer() FaviconDiscoverer {
	return &httpDiscoverer{client: safehttp.NewClient(10*time.Second, 5)}
}

func (c *httpDiscoverer) Discover(ctx context.Context, siteURL string) (string, error) {
	return favicon.Discover(ctx, c.client, siteURL)
}

// Resolver discovers a discover-candidate's favicon on demand: a cheap DB-only backoff check every call,
// a live favicon fetch at most once per key at a time (singleflight), capped concurrency, and a backoff
// ladder on failure that never delays or is delayed by the posts-preview fetch's own ladder.
type Resolver struct {
	resolutions   PublicationResolutionReader
	subscriptions SubscriptionSiteReader
	trending      TrendingSiteReader
	discoverer    FaviconDiscoverer
	stateReader   StateReader
	runTx         func(ctx context.Context, fn func(StateWriter) error) error

	group  singleflight.Group
	sem    chan struct{}
	budget time.Duration
	now    func() time.Time
}

// NewResolver builds an on-demand favicon resolver. Without WithTxRunner it errors on every discovery write.
func NewResolver(resolutions PublicationResolutionReader, subscriptions SubscriptionSiteReader, trending TrendingSiteReader, discoverer FaviconDiscoverer, stateReader StateReader) *Resolver {
	return &Resolver{
		resolutions:   resolutions,
		subscriptions: subscriptions,
		trending:      trending,
		discoverer:    discoverer,
		stateReader:   stateReader,
		sem:           make(chan struct{}, resolveConcurrencyLimit),
		budget:        discoveryBudget,
		now:           time.Now,
		runTx: func(ctx context.Context, fn func(StateWriter) error) error {
			return errors.New("discoverfavicon: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner wires cache writes to the writer pool in one transaction (*db.Queries satisfies StateWriter).
func (r *Resolver) WithTxRunner(w *sql.DB) *Resolver {
	r.runTx = func(ctx context.Context, fn func(StateWriter) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return r
}

// Resolve returns candidateKey's favicon URL, discovering it live at most once per key at a time. ""
// with a nil error means nothing is available yet (in backoff, no known site, or a fresh discovery
// failure); callers never branch on a special error, only a genuine lookup failure propagates.
func (r *Resolver) Resolve(ctx context.Context, candidateKey string) (string, error) {
	state, haveState, err := r.loadState(ctx, candidateKey)
	if err != nil {
		return "", err
	}
	now := r.now()
	if haveState && state.FaviconNextRetryAt != nil {
		if nextRetry, perr := time.Parse(time.RFC3339, *state.FaviconNextRetryAt); perr == nil && now.Before(nextRetry) {
			return "", nil
		}
	}

	v, err, _ := r.group.Do(candidateKey, func() (any, error) {
		discCtx, cancel := context.WithTimeout(ctx, r.budget)
		defer cancel()
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		case <-discCtx.Done():
			return "", discCtx.Err()
		}
		// ctx (not discCtx) carries the cache write: a discovery that exhausts the budget must still get its failure recorded.
		return r.discoverOnce(ctx, discCtx, candidateKey, state, haveState)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (r *Resolver) loadState(ctx context.Context, key string) (db.DiscoverSourcePostsState, bool, error) {
	state, err := r.stateReader.GetDiscoverSourcePostsState(ctx, key)
	switch {
	case err == nil:
		return state, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return db.DiscoverSourcePostsState{}, false, nil
	default:
		return db.DiscoverSourcePostsState{}, false, err
	}
}

func (r *Resolver) discoverOnce(ctx, discCtx context.Context, key string, prior db.DiscoverSourcePostsState, havePrior bool) (string, error) {
	site, ok := r.findSite(discCtx, key)
	if !ok {
		// Nothing to try yet: no backoff recorded, so the very next request retries all three sources.
		return "", nil
	}

	iconURL, err := r.discoverer.Discover(discCtx, site)
	if err != nil {
		r.recordFailure(ctx, key, prior, havePrior)
		return "", nil
	}

	if werr := r.runTx(ctx, func(w StateWriter) error {
		return w.UpsertDiscoverSourceFaviconURL(ctx, db.UpsertDiscoverSourceFaviconURLParams{SourceKey: key, FaviconUrl: nilIfEmpty(iconURL)})
	}); werr != nil {
		slog.Warn("discoverfavicon: favicon cache write failed", "key", key, "err", werr)
	}
	return iconURL, nil
}

// findSite tries the resolution cache, then a followed repo's subscription, then a trending signal, in
// that order; a per-source DB error degrades to the next source instead of failing discovery outright.
func (r *Resolver) findSite(ctx context.Context, key string) (string, bool) {
	if row, err := r.resolutions.GetDiscoverPublicationResolutionByCanonicalKey(ctx, &key); err == nil {
		if row.SiteUrl != nil && *row.SiteUrl != "" {
			return *row.SiteUrl, true
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("discoverfavicon: resolution site lookup failed", "key", key, "err", err)
	}

	if siteURL, err := r.subscriptions.GetDiscoverCrawlSubscriptionSiteURLByKey(ctx, key); err == nil {
		if siteURL != nil && *siteURL != "" {
			return *siteURL, true
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("discoverfavicon: subscription site lookup failed", "key", key, "err", err)
	}

	if row, err := r.trending.GetDiscoverTrendingSignalTitle(ctx, key); err == nil {
		if row.SiteUrl != nil && *row.SiteUrl != "" {
			return *row.SiteUrl, true
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("discoverfavicon: trending site lookup failed", "key", key, "err", err)
	}

	return "", false
}

func (r *Resolver) recordFailure(ctx context.Context, key string, prior db.DiscoverSourcePostsState, havePrior bool) {
	var failures int64 = 1
	if havePrior {
		failures = prior.FaviconFailureCount + 1
	}
	nextRetryAt := r.now().Add(faviconBackoff.Delay(int(failures))).UTC().Format(time.RFC3339)
	if err := r.runTx(ctx, func(w StateWriter) error {
		return w.RecordDiscoverSourceFaviconDiscoveryFailure(ctx, db.RecordDiscoverSourceFaviconDiscoveryFailureParams{
			SourceKey:           key,
			FaviconFailureCount: failures,
			FaviconNextRetryAt:  &nextRetryAt,
		})
	}); err != nil {
		slog.Warn("discoverfavicon: favicon failure record write failed", "key", key, "err", err)
	}
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

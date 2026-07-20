package discoverbatch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
)

// RepoCrawler pulls one repo's reader-network signals, reusing discovercrawl's per-lexicon decode.
type RepoCrawler interface {
	Crawl(ctx context.Context, did syntax.DID) ([]discovercrawl.Subscription, error)
	CrawlAuthoredPublications(ctx context.Context, did syntax.DID) ([]discovercrawl.AuthoredPublication, error)
	CrawlShares(ctx context.Context, did syntax.DID) ([]discovercrawl.Share, error)
	CrawlSaves(ctx context.Context, did syntax.DID) ([]discovercrawl.Save, error)
	// CrawlReaderNetworkFollows lists a repo's outbound reader-network follows. SPEC <discovery> People Global/Trending.
	CrawlReaderNetworkFollows(ctx context.Context, did syntax.DID) ([]discovercrawl.ReaderNetworkFollow, error)
}

// Resolver peeks a repo's PDS host to throttle before crawling; the crawler resolves again internally, but identity lookups are cached.
type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

// Writer replaces one repo's aggregate rows inside a transaction. SPEC <discovery>: diff, not accumulate.
type Writer interface {
	DeleteDiscoverTrendingSignalsForRepo(ctx context.Context, repoDid string) error
	InsertDiscoverTrendingSignal(ctx context.Context, arg db.InsertDiscoverTrendingSignalParams) error
	DeleteDiscoverTrendingFollowsForRepo(ctx context.Context, repoDid string) error
	InsertDiscoverTrendingFollow(ctx context.Context, arg db.InsertDiscoverTrendingFollowParams) error
}

const (
	// defaultGlobalConcurrency bounds total in-flight repo crawls across all hosts.
	defaultGlobalConcurrency = 32
	// defaultPerHostConcurrency bounds in-flight requests to one PDS host; repos cluster heavily on a few hosts.
	defaultPerHostConcurrency = 4
)

// Batch is the daily trending aggregator.
type Batch struct {
	relayEndpoint     string
	relayClient       *http.Client
	resolver          Resolver
	crawler           RepoCrawler
	entries           EntryResolver
	collections       []string
	followCollections []string
	runTx             func(ctx context.Context, fn func(Writer) error) error
	now               func() time.Time

	globalConcurrency  int
	perHostConcurrency int
}

// New builds a Batch. Callers must chain WithTxRunner before production use; without one, every run crawls but writes nothing (logged).
func New(relayEndpoint string, relayClient *http.Client, resolver Resolver, crawler RepoCrawler, entries EntryResolver) *Batch {
	return &Batch{
		relayEndpoint:      normalizeRelayHost(relayEndpoint),
		relayClient:        relayClient,
		resolver:           resolver,
		crawler:            crawler,
		entries:            entries,
		collections:        EnumerationCollections,
		followCollections:  FollowEnumerationCollections,
		now:                time.Now,
		globalConcurrency:  defaultGlobalConcurrency,
		perHostConcurrency: defaultPerHostConcurrency,
		runTx: func(ctx context.Context, fn func(Writer) error) error {
			return errors.New("discoverbatch: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits each repo's write batch in one transaction on the writer pool.
func (b *Batch) WithTxRunner(w *sql.DB) *Batch {
	b.runTx = func(ctx context.Context, fn func(Writer) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return b
}

// Run crawls every enumerated repo and diffs its signals into the aggregate tables. The error return is non-nil only for relay enumeration failure or context cancellation; per-repo failures are swallowed.
func (b *Batch) Run(ctx context.Context) (int, error) {
	dids, err := EnumerateAll(ctx, b.relayClient, b.relayEndpoint, b.collections)
	if err != nil {
		return 0, err
	}
	followDIDs, err := EnumerateAll(ctx, b.relayClient, b.relayEndpoint, b.followCollections)
	if err != nil {
		return 0, err
	}
	if len(dids) == 0 && len(followDIDs) == 0 {
		return 0, nil
	}

	throttle := newHostThrottle(b.perHostConcurrency)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(b.globalConcurrency)
	for _, d := range dids {
		d := d
		g.Go(func() error {
			b.processRepo(gctx, throttle, d)
			return nil
		})
	}
	for _, d := range followDIDs {
		d := d
		g.Go(func() error {
			b.processFollowRepo(gctx, throttle, d)
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return len(dids), err
	}
	return len(dids), nil
}

// processRepo crawls before opening the write transaction so no network I/O happens inside it; any failure logs and skips the write.
func (b *Batch) processRepo(ctx context.Context, throttle *hostThrottle, didStr string) {
	did, err := syntax.ParseDID(didStr)
	if err != nil {
		slog.Warn("discoverbatch: malformed repo did", "did", didStr, "err", err)
		return
	}

	host, err := b.repoHost(ctx, did)
	if err != nil {
		slog.Warn("discoverbatch: resolve failed", "did", didStr, "err", err)
		return
	}

	release, err := throttle.acquire(ctx, host)
	if err != nil {
		return // context cancelled
	}
	defer release()

	signals, complete := b.crawlRepo(ctx, did)
	if ctx.Err() != nil || !complete {
		return
	}

	fetchedAt := b.now().UTC().Format(time.RFC3339)
	if err := b.runTx(ctx, func(w Writer) error {
		return replaceRepoSignals(ctx, w, didStr, signals, fetchedAt)
	}); err != nil {
		slog.Warn("discoverbatch: write failed", "did", didStr, "err", err)
	}
}

// processFollowRepo skips the write on crawl failure rather than wiping the repo's rows: a
// transient outage shouldn't cost someone a day of trending rank.
func (b *Batch) processFollowRepo(ctx context.Context, throttle *hostThrottle, didStr string) {
	did, err := syntax.ParseDID(didStr)
	if err != nil {
		slog.Warn("discoverbatch: malformed follow repo did", "did", didStr, "err", err)
		return
	}

	host, err := b.repoHost(ctx, did)
	if err != nil {
		slog.Warn("discoverbatch: resolve failed for follow repo", "did", didStr, "err", err)
		return
	}

	release, err := throttle.acquire(ctx, host)
	if err != nil {
		return // context cancelled
	}
	defer release()

	follows, err := b.crawler.CrawlReaderNetworkFollows(ctx, did)
	if err != nil {
		slog.Debug("discoverbatch: follow crawl failed, keeping prior rows", "did", did, "err", err)
		return
	}
	if ctx.Err() != nil {
		return
	}

	fetchedAt := b.now().UTC().Format(time.RFC3339)
	if err := b.runTx(ctx, func(w Writer) error {
		return replaceRepoFollows(ctx, w, didStr, follows, fetchedAt)
	}); err != nil {
		slog.Warn("discoverbatch: follow write failed", "did", didStr, "err", err)
	}
}

func (b *Batch) repoHost(ctx context.Context, did syntax.DID) (string, error) {
	ident, err := b.resolver.LookupDID(ctx, did)
	if err != nil {
		return "", err
	}
	endpoint := ident.PDSEndpoint()
	if endpoint == "" {
		return "", fmt.Errorf("discoverbatch: no PDS endpoint for %s", did)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("discoverbatch: invalid PDS endpoint %q: %w", endpoint, err)
	}
	return u.Host, nil
}

// crawlRepo finishes every collection so successful cache fills survive, but only a complete snapshot may replace aggregate rows.
func (b *Batch) crawlRepo(ctx context.Context, did syntax.DID) (map[string]repoSource, bool) {
	complete := true
	var subs []discovercrawl.Subscription
	if s, err := b.crawler.Crawl(ctx, did); err != nil {
		slog.Debug("discoverbatch: subscription crawl failed", "did", did, "err", err)
		complete = false
	} else {
		subs = s
	}
	var pubs []discovercrawl.AuthoredPublication
	if p, err := b.crawler.CrawlAuthoredPublications(ctx, did); err != nil {
		slog.Debug("discoverbatch: authored crawl failed", "did", did, "err", err)
		complete = false
	} else {
		pubs = p
	}
	var shares []discovercrawl.Share
	if s, err := b.crawler.CrawlShares(ctx, did); err != nil {
		slog.Debug("discoverbatch: share crawl failed", "did", did, "err", err)
		complete = false
	} else {
		shares = s
	}
	var saves []discovercrawl.Save
	if s, err := b.crawler.CrawlSaves(ctx, did); err != nil {
		slog.Debug("discoverbatch: save crawl failed", "did", did, "err", err)
		complete = false
	} else {
		saves = s
	}
	return reduceRepoSignals(ctx, subs, pubs, shares, saves, b.entries), complete
}

// replaceRepoSignals deletes then reinserts a repo's rows in one transaction so reruns diff rather than accumulate. SPEC <discovery>.
func replaceRepoSignals(ctx context.Context, w Writer, repoDID string, signals map[string]repoSource, fetchedAt string) error {
	if err := w.DeleteDiscoverTrendingSignalsForRepo(ctx, repoDID); err != nil {
		return err
	}
	for key, s := range signals {
		if err := w.InsertDiscoverTrendingSignal(ctx, db.InsertDiscoverTrendingSignalParams{
			RepoDid:    repoDID,
			SourceKey:  key,
			Kind:       s.Kind,
			Title:      nilIfEmpty(s.Title),
			SiteUrl:    nilIfEmpty(s.SiteURL),
			SignalKind: s.Signal.Kind.String(),
			SignalAt:   formatOptionalTime(s.Signal.At),
			FetchedAt:  fetchedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

// replaceRepoFollows applies the same diff/replace contract as replaceRepoSignals, scoped to the follower aggregate.
func replaceRepoFollows(ctx context.Context, w Writer, repoDID string, follows []discovercrawl.ReaderNetworkFollow, fetchedAt string) error {
	if err := w.DeleteDiscoverTrendingFollowsForRepo(ctx, repoDID); err != nil {
		return err
	}
	for _, f := range follows {
		if err := w.InsertDiscoverTrendingFollow(ctx, db.InsertDiscoverTrendingFollowParams{
			RepoDid:    repoDID,
			SubjectDid: f.DID,
			FetchedAt:  fetchedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func formatOptionalTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

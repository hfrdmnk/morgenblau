package sync

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

// guardWindow matches SPEC <feed-sources>: a repeat login or refresh within
// this window collapses to the already-running job id rather than starting
// a new one.
const guardWindow = 5 * time.Minute

// PDSLister snapshots app.skyreader.feed.subscription records for a user.
// Stubbed in tests with a canned fake.
type PDSLister interface {
	ListSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSSubscription, error)
}

// PDSSubscription is the trimmed shape we care about — at-uri, rkey, feedUrl,
// plus optional metadata that lands on the Tier-1 row.
type PDSSubscription struct {
	URI         string
	Rkey        string
	FeedURL     string
	Title       string
	CustomTitle string
}

// SyncStore is the slice of *db.Queries SyncUser depends on. Defined here so
// the orchestrator's full surface remains hideable behind one interface.
type SyncStore interface {
	ListUserSubscriptionsForSync(ctx context.Context, did string) ([]db.ListUserSubscriptionsForSyncRow, error)
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
}

// SessionResumer hands SyncUser an authenticated session for the given DID,
// independent of any incoming request. The login path may complete before the
// SyncUser goroutine starts, so we resume by (did, sessionID).
type SessionResumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error)
}

// Engine is the deep module surface — SyncUser hides errgroup orchestration,
// PDS reconcile, dual-track fan-out, and the 5-min in-flight guard.
type Engine struct {
	jobs    *jobs.Tracker
	store   SyncStore
	lister  PDSLister
	fetcher FeedFetcher
	resumer SessionResumer
	now     func() time.Time

	parentCtx context.Context
	wg        *sync.WaitGroup
}

// attachLifecycle binds the engine to a parent ctx + WaitGroup so its goroutines
// participate in graceful shutdown. Called by Orchestrator.WithEngine.
func (e *Engine) attachLifecycle(ctx context.Context, wg *sync.WaitGroup) {
	e.parentCtx = ctx
	e.wg = wg
}

// NewEngine wires the dependencies. nil-safe for the fetcher and resumer —
// missing pieces collapse to a no-op so unit tests can run sync_user end to
// end without a real upstream.
func NewEngine(
	tracker *jobs.Tracker,
	store SyncStore,
	lister PDSLister,
	fetcher FeedFetcher,
	resumer SessionResumer,
) *Engine {
	if fetcher == nil {
		fetcher = noopFeedFetcher{}
	}
	return &Engine{
		jobs:      tracker,
		store:     store,
		lister:    lister,
		fetcher:   fetcher,
		resumer:   resumer,
		now:       time.Now,
		parentCtx: context.Background(),
	}
}

// SyncUser orchestrates the dual-track refresh for the given (did, sessionID).
// Returns the created job's id. The 5-minute in-flight guard coalesces repeat
// triggers — a second call within the guard window returns the existing id.
func (e *Engine) SyncUser(ctx context.Context, did syntax.DID, sessionID string, trigger jobs.Trigger) (string, error) {
	j, existed := e.jobs.CreateOrReturnExisting(jobs.KindSyncUser, did, trigger, guardWindow)
	if existed {
		return j.ID, nil
	}
	if e.wg != nil {
		e.wg.Add(1)
	}
	go func() {
		if e.wg != nil {
			defer e.wg.Done()
		}
		e.run(j.ID, did, sessionID)
	}()
	return j.ID, nil
}

func (e *Engine) run(id string, did syntax.DID, sessionID string) {
	e.jobs.SetRunning(id)
	bg, cancel := context.WithTimeout(e.parentCtx, 5*time.Minute)
	defer cancel()

	sess, err := e.resumer.ResumeSession(bg, did, sessionID)
	if err != nil {
		slog.Warn("sync_user: resume failed", "did", did, "err", err)
		e.jobs.SetFailed(id)
		return
	}

	if err := e.runDualTrack(bg, did, sess); err != nil {
		slog.Warn("sync_user: failed", "did", did, "err", err)
		e.jobs.SetFailed(id)
		return
	}
	e.jobs.SetDone(id)
}

// runDualTrack is the heart of the engine — public-by-package so tests can
// drive it directly without the goroutine wrapping.
func (e *Engine) runDualTrack(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	// Snapshot Tier-1 BEFORE reconcile so Phase 1B doesn't wait on 1A.
	snapshot, err := e.store.ListUserSubscriptionsForSync(ctx, did.String())
	if err != nil {
		return err
	}
	snapURLs := make([]string, 0, len(snapshot))
	for _, row := range snapshot {
		snapURLs = append(snapURLs, row.FeedUrl)
	}

	// addedFeedURLs is populated by Phase 1A; Phase 2 reads it after fan-in.
	var (
		addedMu      sync.Mutex
		addedFeedURLs []string
	)

	g, gctx := errgroup.WithContext(ctx)

	// Phase 1A: PDS reconcile.
	g.Go(func() error {
		return e.reconcileTier1(gctx, did, sess, snapshot, func(url string) {
			addedMu.Lock()
			addedFeedURLs = append(addedFeedURLs, url)
			addedMu.Unlock()
		})
	})

	// Phase 1B: Local-known fan-out fetch.
	g.Go(func() error {
		fetchAll(gctx, snapURLs, e.fetcher)
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	// Phase 2: top-up — fetch newly-discovered URLs that 1B didn't already cover.
	already := make(map[string]struct{}, len(snapURLs))
	for _, u := range snapURLs {
		already[u] = struct{}{}
	}
	var topUp []string
	addedMu.Lock()
	for _, u := range addedFeedURLs {
		if _, ok := already[u]; !ok {
			topUp = append(topUp, u)
		}
	}
	addedMu.Unlock()
	fetchAll(ctx, topUp, e.fetcher)
	return nil
}

func fetchAll(ctx context.Context, urls []string, f FeedFetcher) {
	if len(urls) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			if err := f.FetchAndStore(ctx, url); err != nil {
				slog.Debug("sync_user: fetch failed", "url", url, "err", err)
			}
		}(u)
	}
	wg.Wait()
}

func (e *Engine) reconcileTier1(
	ctx context.Context,
	did syntax.DID,
	sess *oauth.ClientSession,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	onAdded func(feedURL string),
) error {
	remote, err := e.lister.ListSubscriptions(ctx, sess)
	if err != nil {
		return err
	}

	localByRkey := make(map[string]db.ListUserSubscriptionsForSyncRow, len(snapshot))
	for _, row := range snapshot {
		localByRkey[row.Rkey] = row
	}
	remoteByRkey := make(map[string]PDSSubscription, len(remote))
	for _, r := range remote {
		remoteByRkey[r.Rkey] = r
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// Inserts + updates from remote.
	for rkey, r := range remoteByRkey {
		_, existed := localByRkey[rkey]
		// Tier-2 first; only on success can the FK from feed_entries.feed_url
		// be satisfied — so onAdded (Phase 2 fetch trigger) is gated on it.
		if err := e.store.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   r.FeedURL,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("reconcile: Tier-2 upsert failed", "feedUrl", r.FeedURL, "err", err)
			continue
		}
		if !existed {
			onAdded(r.FeedURL)
		}
		if err := e.store.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
			Did:         didStr,
			Rkey:        rkey,
			AtUri:       r.URI,
			FeedUrl:     r.FeedURL,
			Title:       nilIfEmpty(r.Title),
			CustomTitle: nilIfEmpty(r.CustomTitle),
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			slog.Warn("reconcile: Tier-1 upsert failed", "err", err)
		}
	}

	// Deletes: locals that no longer exist remotely.
	for rkey := range localByRkey {
		if _, stillThere := remoteByRkey[rkey]; stillThere {
			continue
		}
		if err := e.store.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{
			Did:  didStr,
			Rkey: rkey,
		}); err != nil {
			slog.Warn("reconcile: Tier-1 delete failed", "err", err)
		}
	}
	return nil
}

// ErrNoLister is returned when the engine is misconfigured and asked to sync.
var ErrNoLister = errors.New("sync: no PDS lister configured")

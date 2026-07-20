package sync

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

// SPEC <feed-sources>: repeat trigger within this window reuses the running job id.
const guardWindow = 5 * time.Minute

// Engine runs the dual-track sync for one user, coalesced by the in-flight guard.
type Engine struct {
	jobs    *jobs.Tracker
	store   SyncStore
	lister  PDSLister
	fetcher FeedFetcher
	resumer SessionResumer
	pds     atprepo.Writer
	locker  SessionLocker
	now     func() time.Time

	// refreshSession is a seam so the resume-and-refresh-under-one-lock guarantee is testable without a live AS.
	refreshSession func(ctx context.Context, sess *oauth.ClientSession) error

	// runTx wraps one reconcile pass's writes in a transaction (no-op default; WithTxRunner
	// installs the real one). Deadlock rule: the closure must never touch a non-tx writer
	// Queries, it would block on the sole writer connection.
	runTx func(ctx context.Context, fn func(SyncStore) error) error

	parentCtx context.Context
	wg        *sync.WaitGroup
}

// WithLocker installs the session locker so each run refreshes its access token eagerly under lock; nil skips the refresh.
func (e *Engine) WithLocker(l SessionLocker) *Engine {
	e.locker = l
	return e
}

// attachLifecycle binds the engine to a parent ctx and WaitGroup so its goroutines join graceful shutdown.
func (e *Engine) attachLifecycle(ctx context.Context, wg *sync.WaitGroup) {
	e.parentCtx = ctx
	e.wg = wg
}

// NewEngine wires the dependencies; resumer may be nil for tests, pds may be nil to disable deletion of orphaned sidecar records (the one reconcile write exception).
func NewEngine(
	tracker *jobs.Tracker,
	store SyncStore,
	lister PDSLister,
	fetcher FeedFetcher,
	resumer SessionResumer,
	pds atprepo.Writer,
) *Engine {
	e := &Engine{
		jobs:      tracker,
		store:     store,
		lister:    lister,
		fetcher:   fetcher,
		resumer:   resumer,
		pds:       pds,
		now:       time.Now,
		parentCtx: context.Background(),
	}
	e.runTx = func(ctx context.Context, fn func(SyncStore) error) error {
		return fn(e.store)
	}
	e.refreshSession = func(ctx context.Context, sess *oauth.ClientSession) error {
		_, err := sess.RefreshTokens(ctx)
		return err
	}
	return e
}

// WithTxRunner commits each reconcile pass's writes in one transaction on the writer pool.
func (e *Engine) WithTxRunner(w *sql.DB) *Engine {
	e.runTx = func(ctx context.Context, fn func(SyncStore) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return e
}

// SyncUser starts the dual-track refresh for (did, sessionID); the in-flight guard coalesces a repeat trigger into the existing job id.
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

	sess, err := e.resumeAndRefresh(bg, did, sessionID)
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

// resumeAndRefresh resumes and refreshes under one continuous lock hold: resuming outside
// the lock would let the request path rotate the refresh token first, so the eager refresh
// would silently no-op on a stale token. Resume failure fails the run; refresh failure is best-effort.
func (e *Engine) resumeAndRefresh(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error) {
	if e.locker != nil {
		unlock := e.locker.LockSession(did, sessionID)
		defer unlock()
	}
	sess, err := e.resumer.ResumeSession(ctx, did, sessionID)
	if err != nil {
		return nil, err
	}
	if e.locker != nil {
		if err := e.refreshSession(ctx, sess); err != nil {
			slog.Warn("sync_user: eager token refresh failed", "did", did, "err", err)
		}
	}
	return sess, nil
}

// runDualTrack is unexported-but-callable so tests can drive it directly, without the goroutine wrapping.
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
		addedMu       sync.Mutex
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

	// Phase 1C: saves reconcile, independent and best-effort; a hiccup here must never fail the primary refresh.
	g.Go(func() error {
		if err := e.reconcileSaves(gctx, did, sess); err != nil {
			slog.Warn("sync_user: saves reconcile failed", "did", did, "err", err)
		}
		return nil
	})

	// Phase 1D: shares reconcile. Same contract as saves: best-effort.
	g.Go(func() error {
		if err := e.reconcileShares(gctx, did, sess); err != nil {
			slog.Warn("sync_user: shares reconcile failed", "did", did, "err", err)
		}
		return nil
	})

	// Phase 1E: follows reconcile. Same contract as saves/shares: best-effort.
	g.Go(func() error {
		if err := e.reconcileFollows(gctx, did, sess); err != nil {
			slog.Warn("sync_user: follows reconcile failed", "did", did, "err", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	// Phase 2: top-up, fetch newly-discovered URLs that 1B didn't already cover.
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

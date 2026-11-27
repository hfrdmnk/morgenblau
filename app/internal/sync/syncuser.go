package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/atprepo"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
	"morgenblau/internal/session"
)

type Engine struct {
	jobs    *jobs.Tracker
	store   SyncStore
	lister  PDSLister
	fetcher FeedFetcher
	resumer SessionResumer
	pds     atprepo.Writer
	locker  SessionLocker
	now     func() time.Time

	refreshSession func(ctx context.Context, sess *session.Session) error
	runTx          func(ctx context.Context, fn func(SyncStore) error) error

	parentCtx context.Context
	wg        *sync.WaitGroup
	runMu     sync.Mutex
	active    map[string]*userRun
}

type userRun struct {
	id            string
	did           syntax.DID
	sessionID     string
	passSessionID string
	rerun         bool
}

func (e *Engine) WithLocker(l SessionLocker) *Engine {
	e.locker = l
	return e
}

func (e *Engine) attachLifecycle(ctx context.Context, wg *sync.WaitGroup) {
	e.parentCtx = ctx
	e.wg = wg
}

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
		active:    make(map[string]*userRun),
	}
	e.runTx = func(ctx context.Context, fn func(SyncStore) error) error {
		return fn(e.store)
	}
	e.refreshSession = func(ctx context.Context, sess *session.Session) error {
		return sess.RefreshTokens(ctx)
	}
	return e
}

func (e *Engine) WithTxRunner(w *sql.DB) *Engine {
	e.runTx = func(ctx context.Context, fn func(SyncStore) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return e
}

func (e *Engine) SyncUser(ctx context.Context, did syntax.DID, sessionID string, trigger jobs.Trigger) (string, error) {
	e.runMu.Lock()
	if active := e.active[did.String()]; active != nil {
		sessionChanged := active.sessionID != sessionID
		if sessionChanged {
			active.sessionID = sessionID
		}
		if active.passSessionID != "" && (trigger == jobs.TriggerManual || sessionChanged && active.passSessionID != sessionID) {
			active.rerun = true
		}
		e.runMu.Unlock()
		return active.id, nil
	}
	j := e.jobs.Create(jobs.KindSyncUser, did, trigger)
	run := &userRun{id: j.ID, did: did, sessionID: sessionID}
	e.active[did.String()] = run
	if e.wg != nil {
		e.wg.Add(1)
	}
	go func() {
		if e.wg != nil {
			defer e.wg.Done()
		}
		e.run(run)
	}()
	e.runMu.Unlock()
	return j.ID, nil
}

func (e *Engine) run(run *userRun) {
	for {
		e.runMu.Lock()
		sessionID := run.sessionID
		run.passSessionID = sessionID
		e.runMu.Unlock()

		e.jobs.SetRunning(run.id)
		bg, cancel := context.WithTimeout(e.parentCtx, 5*time.Minute)
		var receipt syncReceipt
		sess, err := e.resumeAndRefresh(bg, run.did, sessionID)
		if err != nil {
			slog.Warn("sync_user: resume failed", "did", run.did, "err", err)
		} else if receipt, err = e.runDualTrack(bg, run.did, sess); err != nil {
			slog.Warn("sync_user: failed", "did", run.did, "err", err)
		}
		cancel()

		e.runMu.Lock()
		if run.rerun && e.parentCtx.Err() == nil {
			run.rerun = false
			run.passSessionID = ""
			e.runMu.Unlock()
			continue
		}
		e.finishSync(run.id, receipt, err)
		delete(e.active, run.did.String())
		e.runMu.Unlock()
		return
	}
}

type syncReceipt struct {
	rss          *committed[db.UserSubscription]
	standardfeed *committed[db.UserSubscription]
	saves        *committed[db.UserSave]
}

func (e *Engine) finishSync(id string, r syncReceipt, err error) {
	if err != nil || r.rss == nil || r.standardfeed == nil || r.saves == nil {
		e.jobs.SetFailed(id)
		return
	}
	e.jobs.SetDone(id)
}

func (e *Engine) resumeAndRefresh(ctx context.Context, did syntax.DID, sessionID string) (*session.Session, error) {
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

func (e *Engine) runDualTrack(ctx context.Context, did syntax.DID, sess *session.Session) (syncReceipt, error) {
	snapshotAt := e.now().UTC()
	snapshot, err := e.store.ListUserSubscriptionsForSync(ctx, did.String())
	if err != nil {
		return syncReceipt{}, err
	}
	snapURLs := make([]string, 0, len(snapshot))
	for _, row := range snapshot {
		snapURLs = append(snapURLs, row.FeedUrl)
	}

	var (
		addedMu       sync.Mutex
		addedFeedURLs []string
		wg            sync.WaitGroup
		receipt       syncReceipt
	)
	errC := make(chan error, 2)
	wg.Add(3)
	go func() {
		defer wg.Done()
		var err error
		receipt.rss, receipt.standardfeed, err = e.reconcileTier1(ctx, did, sess, snapshot, snapshotAt, func(url string) {
			addedMu.Lock()
			addedFeedURLs = append(addedFeedURLs, url)
			addedMu.Unlock()
		})
		if err != nil {
			errC <- fmt.Errorf("subscriptions reconcile: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		fetchAll(ctx, snapURLs, e.fetcher)
	}()

	go func() {
		defer wg.Done()
		var err error
		if receipt.saves, err = e.reconcileSaves(ctx, did, sess); err != nil {
			slog.Warn("sync_user: saves reconcile failed", "did", did, "err", err)
			errC <- fmt.Errorf("saves reconcile: %w", err)
		}
	}()

	wg.Wait()
	close(errC)
	var reconcileErrs []error
	for err := range errC {
		reconcileErrs = append(reconcileErrs, err)
	}

	addedMu.Lock()
	topUp := notAlreadyFetched(addedFeedURLs, snapURLs)
	addedMu.Unlock()
	fetchAll(ctx, topUp, e.fetcher)
	return receipt, errors.Join(reconcileErrs...)
}

func notAlreadyFetched(added, fetched []string) []string {
	already := make(map[string]struct{}, len(fetched))
	for _, u := range fetched {
		already[u] = struct{}{}
	}
	var topUp []string
	for _, u := range added {
		if _, ok := already[u]; !ok {
			topUp = append(topUp, u)
		}
	}
	return topUp
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

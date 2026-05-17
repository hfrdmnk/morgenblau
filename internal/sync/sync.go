// Package sync orchestrates the dual-track refresh: PDS reconcile + fan-out
// fetch. This slice ships the no-op skeleton so the route + pill mechanic can
// be wired end-to-end; Issue 07 fills in the real errgroup + fetcher fan-out.
package sync

import (
	"context"
	"log/slog"
	"sync"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/jobs"
)

// FeedFetcher is the slice of internal/fetcher the orchestrator uses for
// single-feed pulls. Wired in Issue 06 so the add-source path can dispatch a
// concrete fetch job; Issue 07 reuses it during sync_user.
type FeedFetcher interface {
	FetchAndStore(ctx context.Context, feedURL string) error
}

// noopFeedFetcher does nothing — used when the slice that wires fetcher
// → storage isn't loaded yet (e.g. unit tests, early bootstrap).
type noopFeedFetcher struct{}

func (noopFeedFetcher) FetchAndStore(_ context.Context, _ string) error { return nil }

// Orchestrator owns the job tracker, the feed pipeline, and the sync engine.
// When an engine is attached, manual + login refresh dispatch real dual-track
// sync; otherwise they fall back to the no-op transition used during the
// early bootstrap (Issue 03).
type Orchestrator struct {
	jobs    *jobs.Tracker
	fetcher FeedFetcher
	engine  *Engine

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New returns a no-op orchestrator backed by the given tracker.
func New(tracker *jobs.Tracker) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{
		jobs:    tracker,
		fetcher: noopFeedFetcher{},
		ctx:     ctx,
		cancel:  cancel,
	}
}

// WithFetcher attaches a fetch+store implementation. Called from the server
// once the feed pipeline (fetcher + Tier-2 storage) is constructed.
func (o *Orchestrator) WithFetcher(f FeedFetcher) *Orchestrator {
	o.fetcher = f
	return o
}

// WithEngine attaches the real dual-track sync engine. Once attached, the
// orchestrator's StartManualRefresh / StartLoginRefresh entrypoints route
// through the engine rather than the no-op fallback. The engine inherits the
// orchestrator's parent ctx + WaitGroup so Shutdown drains its goroutines too.
func (o *Orchestrator) WithEngine(e *Engine) *Orchestrator {
	o.engine = e
	e.attachLifecycle(o.ctx, &o.wg)
	return o
}

// Shutdown cancels the orchestrator's parent ctx and waits for in-flight
// goroutines to finish. Returns ctx.Err() if the wait deadline trips before
// the WaitGroup drains.
func (o *Orchestrator) Shutdown(ctx context.Context) error {
	o.cancel()
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StartManualRefresh creates a sync_user job for (did, sessionID). When an
// engine is attached the real dual-track sync runs; otherwise the orchestrator
// transitions the job through pending → done synchronously.
func (o *Orchestrator) StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error) {
	if o.engine != nil {
		return o.engine.SyncUser(ctx, did, sessionID, jobs.TriggerManual)
	}
	j := o.jobs.Create(jobs.KindSyncUser, did, jobs.TriggerManual)
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.runNoop(j.ID)
	}()
	return j.ID, nil
}

// StartLoginRefresh is the entrypoint OAuth callback uses. Same code path as
// manual; trigger metadata differs.
func (o *Orchestrator) StartLoginRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error) {
	if o.engine != nil {
		return o.engine.SyncUser(ctx, did, sessionID, jobs.TriggerLogin)
	}
	j := o.jobs.Create(jobs.KindSyncUser, did, jobs.TriggerLogin)
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.runNoop(j.ID)
	}()
	return j.ID, nil
}

func (o *Orchestrator) runNoop(id string) {
	o.jobs.SetRunning(id)
	o.jobs.SetDone(id)
}

// StartFetchOneFeed creates a fetch_one_feed job for feedURL and dispatches
// it through the configured FeedFetcher. Used by the add-source path so the
// refresh pill activates the moment a user adds a source.
func (o *Orchestrator) StartFetchOneFeed(ctx context.Context, did syntax.DID, feedURL string) string {
	j := o.jobs.Create(jobs.KindFetchOneFeed, did, jobs.TriggerAddFeed)
	o.wg.Add(1)
	go func(id, url string) {
		defer o.wg.Done()
		o.jobs.SetRunning(id)
		if err := o.fetcher.FetchAndStore(o.ctx, url); err != nil {
			slog.Warn("fetch_one_feed failed", "url", url, "err", err)
			o.jobs.SetFailed(id)
			return
		}
		o.jobs.SetDone(id)
	}(j.ID, feedURL)
	_ = ctx
	return j.ID
}

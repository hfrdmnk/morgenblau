// Package sync orchestrates the dual-track refresh: PDS reconcile + fan-out
// fetch.
package sync

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/jobs"
)

// FeedFetcher is the slice of internal/fetcher the orchestrator uses.
type FeedFetcher interface {
	FetchAndStore(ctx context.Context, feedURL string) error
}

var ErrNoEngine = errors.New("sync: engine not configured")

// Orchestrator owns the job tracker, the feed pipeline, and the sync engine.
type Orchestrator struct {
	jobs    *jobs.Tracker
	fetcher FeedFetcher
	engine  *Engine

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(tracker *jobs.Tracker, fetcher FeedFetcher, engine *Engine) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	o := &Orchestrator{
		jobs:    tracker,
		fetcher: fetcher,
		engine:  engine,
		ctx:     ctx,
		cancel:  cancel,
	}
	if engine != nil {
		engine.attachLifecycle(o.ctx, &o.wg)
	}
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

// StartManualRefresh creates a sync_user job for (did, sessionID).
func (o *Orchestrator) StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error) {
	if o.engine == nil {
		return "", ErrNoEngine
	}
	return o.engine.SyncUser(ctx, did, sessionID, jobs.TriggerManual)
}

// StartLoginRefresh is the entrypoint OAuth callback uses. Same code path as
// manual; trigger metadata differs.
func (o *Orchestrator) StartLoginRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error) {
	if o.engine == nil {
		return "", ErrNoEngine
	}
	return o.engine.SyncUser(ctx, did, sessionID, jobs.TriggerLogin)
}

// StartFetchOneFeed creates a fetch_one_feed job for feedURL and dispatches
// it through the configured FeedFetcher. Used by the add-source path so the
// refresh pill activates the moment a user adds a source.
func (o *Orchestrator) StartFetchOneFeed(did syntax.DID, feedURL string) string {
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
	return j.ID
}

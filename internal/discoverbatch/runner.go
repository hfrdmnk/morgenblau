package discoverbatch

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"morgenblau/internal/database/db"
)

// runFunc is the batch-run seam Runner ticks against; tests inject a fake to control timing.
type runFunc func(ctx context.Context) (int, error)

// BatchStateReader reads the trending batch's last-successful-run stamp.
type BatchStateReader interface {
	GetDiscoverBatchState(ctx context.Context) (db.DiscoverBatchState, error)
}

// BatchStateWriter persists the trending batch's last-successful-run stamp.
type BatchStateWriter interface {
	UpsertDiscoverBatchState(ctx context.Context, lastRunAt string) error
}

// Runner owns the daily batch's timer goroutine lifecycle: context cancellation plus a drained WaitGroup, mirroring sync.Orchestrator. SPEC <discovery>.
type Runner struct {
	run      runFunc
	interval time.Duration
	stateR   BatchStateReader
	stateW   BatchStateWriter
	now      func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRunner builds a Runner around a Batch; call Start once to launch the ticker goroutine.
func NewRunner(batch *Batch, interval time.Duration) *Runner {
	return newRunnerWithRunFunc(batch.Run, interval)
}

func newRunnerWithRunFunc(run runFunc, interval time.Duration) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{run: run, interval: interval, now: time.Now, ctx: ctx, cancel: cancel}
}

// WithStateStore persists last-run stamps so restarts inside the interval (air hot-reload) skip the immediate startup run; nil-safe chaining.
func (r *Runner) WithStateStore(reader BatchStateReader, writer BatchStateWriter) *Runner {
	r.stateR = reader
	r.stateW = writer
	return r
}

// Start launches the ticker goroutine. Not safe to call more than once.
func (r *Runner) Start() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.loop()
	}()
}

func (r *Runner) loop() {
	// A restart mid-interval should resume where the schedule left off, not skip straight to a full interval nor rerun immediately; initialDelay computes the residual wait, and NewTimer(0) fires immediately when there's no usable stamp. The select keeps even a multi-hour wait interruptible by Shutdown.
	timer := time.NewTimer(r.initialDelay())
	defer timer.Stop()
	select {
	case <-r.ctx.Done():
		return
	case <-timer.C:
		r.runOnce()
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.runOnce()
		}
	}
}

// initialDelay fails open (0, run immediately) whenever the last-run stamp is unavailable or untrustworthy, matching the loop comment: a missed run costs a day of trending, so every failure mode favors running.
func (r *Runner) initialDelay() time.Duration {
	if r.stateR == nil {
		return 0
	}
	state, err := r.stateR.GetDiscoverBatchState(r.ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("discover trending batch state read failed, running immediately", "err", err)
		}
		return 0
	}
	last, err := time.Parse(time.RFC3339, state.LastRunAt)
	if err != nil {
		slog.Warn("discover trending batch state stamp unparseable, running immediately", "stamp", state.LastRunAt, "err", err)
		return 0
	}
	remaining := r.interval - r.now().Sub(last)
	if remaining <= 0 {
		return 0
	}
	if remaining > r.interval {
		// clamp: a future-dated stamp (clock skew, restored database) must not delay the first run past a full interval
		remaining = r.interval
	}
	slog.Info("discover trending batch startup run delayed", "delay", remaining, "last_run_at", state.LastRunAt)
	return remaining
}

func (r *Runner) runOnce() {
	n, err := r.run(r.ctx)
	if err != nil {
		if r.ctx.Err() != nil {
			return
		}
		slog.Warn("discover trending batch failed", "err", err)
		return
	}
	slog.Info("discover trending batch complete", "repos", n)
	if r.stateW != nil {
		if err := r.stateW.UpsertDiscoverBatchState(r.ctx, r.now().UTC().Format(time.RFC3339)); err != nil {
			slog.Warn("discover trending batch state write failed", "err", err)
		}
	}
}

// Shutdown cancels the runner's context and waits for the in-flight run to finish or ctx to expire, whichever comes first.
func (r *Runner) Shutdown(ctx context.Context) error {
	r.cancel()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

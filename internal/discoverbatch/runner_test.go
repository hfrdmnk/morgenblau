package discoverbatch

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/database/db"
)

type fakeRunnable struct {
	calls    int64
	blockCh  chan struct{} // when non-nil, each call blocks until closed or ctx cancelled
	unblockN int32         // set to signal "unblocked" once via atomic
}

func (f *fakeRunnable) Run(ctx context.Context) (int, error) {
	atomic.AddInt64(&f.calls, 1)
	if f.blockCh != nil {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	atomic.AddInt32(&f.unblockN, 1)
	return 1, nil
}

func TestRunner_RunsOnceOnStartBeforeFirstTick(t *testing.T) {
	fake := &fakeRunnable{}
	r := newRunnerWithRunFunc(fake.Run, time.Hour)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Run was not invoked on Start; first execution would wait a full interval")
}

func TestRunner_TicksAndInvokesRunOnEachInterval(t *testing.T) {
	fake := &fakeRunnable{}
	r := newRunnerWithRunFunc(fake.Run, 5*time.Millisecond)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Run was called %d times in 500ms, want at least 2 ticks", atomic.LoadInt64(&fake.calls))
}

func TestRunner_Shutdown_CancelsContextAndDrainsWaitGroup(t *testing.T) {
	fake := &fakeRunnable{blockCh: make(chan struct{})}
	r := newRunnerWithRunFunc(fake.Run, time.Millisecond)
	r.Start()

	// Wait for the in-flight run to start; it blocks on fake.blockCh until ctx is cancelled.
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt64(&fake.calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if atomic.LoadInt64(&fake.calls) == 0 {
		t.Fatal("Run was never called")
	}

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- r.Shutdown(ctx)
	}()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return — WaitGroup never drained")
	}
}

func TestRunner_Shutdown_TimesOutIfRunNeverReturns(t *testing.T) {
	// The fake ignores ctx cancellation entirely (unlike the real batch), so this proves Shutdown returns via its own deadline rather than hanging.
	fake := &fakeRunnable{}
	stuck := func(ctx context.Context) (int, error) {
		atomic.AddInt64(&fake.calls, 1)
		<-make(chan struct{}) // never unblocks
		return 0, nil
	}
	r := newRunnerWithRunFunc(stuck, time.Millisecond)
	r.Start()

	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt64(&fake.calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := r.Shutdown(ctx); err == nil {
		t.Fatal("expected Shutdown to time out against a run that never returns")
	}
}

// fakeStateStore is a mutex-guarded in-memory BatchStateReader/Writer; loop() and runOnce() touch it from the ticker goroutine while tests read it, so every access is guarded.
type fakeStateStore struct {
	mu      sync.Mutex
	state   db.DiscoverBatchState
	hasRow  bool
	readErr error
	writes  []string
}

func (f *fakeStateStore) GetDiscoverBatchState(ctx context.Context) (db.DiscoverBatchState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return db.DiscoverBatchState{}, f.readErr
	}
	if !f.hasRow {
		return db.DiscoverBatchState{}, sql.ErrNoRows
	}
	return f.state, nil
}

func (f *fakeStateStore) UpsertDiscoverBatchState(ctx context.Context, lastRunAt string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = db.DiscoverBatchState{ID: 1, LastRunAt: lastRunAt}
	f.hasRow = true
	f.writes = append(f.writes, lastRunAt)
	return nil
}

func (f *fakeStateStore) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func (f *fakeStateStore) lastWrite() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes[len(f.writes)-1]
}

func withFixedNow(r *Runner, now time.Time) *Runner {
	r.now = func() time.Time { return now }
	return r
}

func TestRunner_WithStateStore_FreshStamp_SkipsImmediateRun(t *testing.T) {
	fake := &fakeRunnable{}
	now := time.Now()
	store := &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(-1 * time.Hour).UTC().Format(time.RFC3339)}, hasRow: true}

	r := withFixedNow(newRunnerWithRunFunc(fake.Run, 24*time.Hour).WithStateStore(store, store), now)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt64(&fake.calls); got != 0 {
		t.Fatalf("Run was called %d times; want 0, a fresh stamp inside the interval should skip the immediate run", got)
	}
}

func TestRunner_WithStateStore_StaleStamp_RunsImmediately(t *testing.T) {
	fake := &fakeRunnable{}
	now := time.Now()
	store := &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(-25 * time.Hour).UTC().Format(time.RFC3339)}, hasRow: true}

	r := withFixedNow(newRunnerWithRunFunc(fake.Run, 24*time.Hour).WithStateStore(store, store), now)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Run was not invoked on Start; a stamp older than the interval should trigger the immediate run")
}

func TestRunner_WithStateStore_NoRow_RunsImmediately(t *testing.T) {
	fake := &fakeRunnable{}
	store := &fakeStateStore{} // no row inserted; GetDiscoverBatchState returns sql.ErrNoRows
	r := newRunnerWithRunFunc(fake.Run, 24*time.Hour).WithStateStore(store, store)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Run was not invoked on Start; a missing state row should trigger the immediate run")
}

func TestRunner_WithStateStore_ReadError_RunsImmediately(t *testing.T) {
	fake := &fakeRunnable{}
	store := &fakeStateStore{readErr: errors.New("db unavailable")}
	r := newRunnerWithRunFunc(fake.Run, 24*time.Hour).WithStateStore(store, store)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Run was not invoked on Start; a state read error should fail open into the immediate run")
}

func TestRunner_WithStateStore_UnparseableStamp_RunsImmediately(t *testing.T) {
	fake := &fakeRunnable{}
	store := &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: "not-a-timestamp"}, hasRow: true}
	r := newRunnerWithRunFunc(fake.Run, 24*time.Hour).WithStateStore(store, store)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Run was not invoked on Start; an unparseable stamp should fail open into the immediate run")
}

func TestRunner_SuccessfulRun_PersistsStamp(t *testing.T) {
	fake := &fakeRunnable{}
	now := time.Now()
	store := &fakeStateStore{}
	r := withFixedNow(newRunnerWithRunFunc(fake.Run, time.Hour).WithStateStore(store, store), now)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.writeCount() >= 1 {
			if want, got := now.UTC().Format(time.RFC3339), store.lastWrite(); got != want {
				t.Fatalf("persisted stamp = %q, want %q", got, want)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no stamp was persisted after a successful run")
}

func TestRunner_FailedRun_DoesNotPersistStamp(t *testing.T) {
	failing := func(ctx context.Context) (int, error) { return 0, errors.New("boom") }
	store := &fakeStateStore{}
	r := newRunnerWithRunFunc(failing, time.Hour).WithStateStore(store, store)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	if n := store.writeCount(); n != 0 {
		t.Fatalf("stamp was persisted %d times after a failed run; want 0", n)
	}
}

func TestRunner_InitialDelay_Table(t *testing.T) {
	interval := 24 * time.Hour
	now := time.Now().Truncate(time.Second) // avoid RFC3339's second-precision truncation shifting the expected duration

	tests := []struct {
		name  string
		store *fakeStateStore
		want  time.Duration
	}{
		{name: "nil state store", store: nil, want: 0},
		{name: "no row", store: &fakeStateStore{}, want: 0},
		{name: "read error", store: &fakeStateStore{readErr: errors.New("db unavailable")}, want: 0},
		{name: "unparseable stamp", store: &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: "not-a-timestamp"}, hasRow: true}, want: 0},
		{name: "stamp older than interval", store: &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(-25 * time.Hour).UTC().Format(time.RFC3339)}, hasRow: true}, want: 0},
		{name: "stamp within interval", store: &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(-1 * time.Hour).UTC().Format(time.RFC3339)}, hasRow: true}, want: 23 * time.Hour},
		{name: "future-dated stamp clamps to interval", store: &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(1 * time.Hour).UTC().Format(time.RFC3339)}, hasRow: true}, want: interval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRunnerWithRunFunc(func(ctx context.Context) (int, error) { return 0, nil }, interval)
			if tt.store != nil {
				r.WithStateStore(tt.store, tt.store)
			}
			withFixedNow(r, now)
			if got := r.initialDelay(); got != tt.want {
				t.Fatalf("initialDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunner_RestartLateInInterval_FirstRunAfterRemainder(t *testing.T) {
	fake := &fakeRunnable{}
	now := time.Now()
	interval := 2 * time.Second
	store := &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(-1950 * time.Millisecond).UTC().Format(time.RFC3339)}, hasRow: true}

	r := withFixedNow(newRunnerWithRunFunc(fake.Run, interval).WithStateStore(store, store), now)
	r.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	}()

	deadline := time.Now().Add(1100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&fake.calls) >= 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Run was not invoked within ~1s of restart; the old skip-then-full-ticker behavior would wait the full 2s interval")
}

func TestRunner_Shutdown_InterruptsInitialDelayWait(t *testing.T) {
	fake := &fakeRunnable{}
	now := time.Now()
	interval := 24 * time.Hour
	store := &fakeStateStore{state: db.DiscoverBatchState{ID: 1, LastRunAt: now.Add(-1 * time.Hour).UTC().Format(time.RFC3339)}, hasRow: true}

	r := withFixedNow(newRunnerWithRunFunc(fake.Run, interval).WithStateStore(store, store), now)
	r.Start()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v, want nil (must interrupt a multi-hour initial-delay wait, not block on it)", err)
	}
}

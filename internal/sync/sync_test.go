package sync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/jobs"
)

type blockingFetcher struct {
	mu       sync.Mutex
	calls    int32
	ctxErrs  int32
	release  chan struct{}
	released atomic.Bool
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{release: make(chan struct{})}
}

func (f *blockingFetcher) FetchAndStore(ctx context.Context, _ string) error {
	atomic.AddInt32(&f.calls, 1)
	select {
	case <-f.release:
		return nil
	case <-ctx.Done():
		atomic.AddInt32(&f.ctxErrs, 1)
		return ctx.Err()
	}
}

func (f *blockingFetcher) Release() {
	if f.released.CompareAndSwap(false, true) {
		close(f.release)
	}
}

type errFetcher struct{ err error }

func (e errFetcher) FetchAndStore(_ context.Context, _ string) error { return e.err }

func TestStartFetchOneFeed_HappyPath(t *testing.T) {
	tracker := jobs.New()
	cf := &countingFetcher{}
	orch := New(tracker, cf, nil)

	did := mustDID("did:plc:alice")
	id := orch.StartFetchOneFeed(did, "https://example.com/feed")

	// Drain via Shutdown — guarantees the goroutine completed.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orch.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	j, err := tracker.Get(id, did)
	if err != nil {
		t.Fatalf("tracker.Get: %v", err)
	}
	if j.Status != jobs.StatusDone {
		t.Errorf("status = %v, want done", j.Status)
	}
	if got := cf.seen(); len(got) != 1 || got[0] != "https://example.com/feed" {
		t.Errorf("fetched = %v, want [https://example.com/feed]", got)
	}
}

func TestStartFetchOneFeed_FetchErrorMarksFailed(t *testing.T) {
	tracker := jobs.New()
	orch := New(tracker, errFetcher{err: errors.New("boom")}, nil)

	did := mustDID("did:plc:alice")
	id := orch.StartFetchOneFeed(did, "https://example.com/feed")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orch.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	j, err := tracker.Get(id, did)
	if err != nil {
		t.Fatalf("tracker.Get: %v", err)
	}
	if j.Status != jobs.StatusFailed {
		t.Errorf("status = %v, want failed", j.Status)
	}
}

func TestStartFetchOneFeed_ShutdownDrains(t *testing.T) {
	tracker := jobs.New()
	bf := newBlockingFetcher()
	orch := New(tracker, bf, nil)

	did := mustDID("did:plc:alice")
	orch.StartFetchOneFeed(did, "https://example.com/feed")

	// Wait until the fetcher entered FetchAndStore so we know the goroutine
	// is actually mid-flight when Shutdown fires.
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt32(&bf.calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if atomic.LoadInt32(&bf.calls) == 0 {
		t.Fatal("fetcher never entered FetchAndStore")
	}

	// Shutdown cancels parent ctx; the fetcher exits via ctx.Done(). The wait
	// returns nil because the goroutine drained before our shutdown deadline.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := orch.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if atomic.LoadInt32(&bf.ctxErrs) == 0 {
		t.Error("fetcher did not observe ctx cancellation on Shutdown")
	}
}

func TestStartFetchOneFeed_ShutdownDeadlineExceeded(t *testing.T) {
	tracker := jobs.New()
	bf := newBlockingFetcher()
	defer bf.Release()
	orch := New(tracker, stubbornFetcher{bf: bf}, nil)

	did := mustDID("did:plc:alice")
	orch.StartFetchOneFeed(did, "https://example.com/feed")

	// Wait until in-flight.
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt32(&bf.calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := orch.Shutdown(shutdownCtx); err == nil {
		t.Fatal("Shutdown returned nil; want deadline exceeded")
	}
}

// stubbornFetcher ignores ctx cancellation — used to verify Shutdown's
// deadline path.
type stubbornFetcher struct{ bf *blockingFetcher }

func (s stubbornFetcher) FetchAndStore(_ context.Context, _ string) error {
	atomic.AddInt32(&s.bf.calls, 1)
	<-s.bf.release
	return nil
}

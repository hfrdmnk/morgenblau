package discoverbatch

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostThrottle_BoundsConcurrencyPerHost(t *testing.T) {
	th := newHostThrottle(2)
	var inFlight, maxInFlight int64
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := th.acquire(context.Background(), "host-a")
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			defer release()

			cur := atomic.AddInt64(&inFlight, 1)
			for {
				max := atomic.LoadInt64(&maxInFlight)
				if cur <= max || atomic.CompareAndSwapInt64(&maxInFlight, max, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt64(&inFlight, -1)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt64(&maxInFlight); got > 2 {
		t.Errorf("max concurrent = %d, want <= 2", got)
	}
}

func TestHostThrottle_DifferentHostsDoNotShareCapacity(t *testing.T) {
	th := newHostThrottle(1)
	releaseA, err := th.acquire(context.Background(), "host-a")
	if err != nil {
		t.Fatalf("acquire host-a: %v", err)
	}
	defer releaseA()

	done := make(chan struct{})
	go func() {
		releaseB, err := th.acquire(context.Background(), "host-b")
		if err != nil {
			t.Errorf("acquire host-b: %v", err)
			return
		}
		defer releaseB()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("acquiring a different host blocked on host-a's slot")
	}
}

func TestHostThrottle_AcquireRespectsContextCancellation(t *testing.T) {
	th := newHostThrottle(1)
	release, err := th.acquire(context.Background(), "host-a")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := th.acquire(ctx, "host-a"); err == nil {
		t.Fatal("expected acquire to fail on a cancelled context")
	}
}

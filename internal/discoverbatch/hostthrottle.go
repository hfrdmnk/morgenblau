package discoverbatch

import (
	"context"
	"sync"
)

// hostThrottle bounds concurrent requests per PDS host since repos cluster
// heavily on a few hosts; a fresh throttle per batch run leaks nothing across runs.
type hostThrottle struct {
	mu      sync.Mutex
	perHost map[string]chan struct{}
	cap     int
}

func newHostThrottle(perHostCap int) *hostThrottle {
	return &hostThrottle{perHost: make(map[string]chan struct{}), cap: perHostCap}
}

// acquire blocks until a slot for host is free or ctx is cancelled, returning a release func to defer.
func (h *hostThrottle) acquire(ctx context.Context, host string) (func(), error) {
	h.mu.Lock()
	ch, ok := h.perHost[host]
	if !ok {
		ch = make(chan struct{}, h.cap)
		h.perHost[host] = ch
	}
	h.mu.Unlock()

	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

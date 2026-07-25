// Package discovermemo holds the short-lived per-user memo of an assembled discover payload, so refreshes and infinite-scroll pages reuse one candidate assembly instead of re-running the crawl fan-out per request. SPEC <discovery>.
package discovermemo

import (
	"sync"
	"time"
)

// DefaultTTL is short on purpose: the memo exists to collapse one browsing session's requests, not to hold suggestions across sessions. Writes that change what a user should see invalidate their entry outright.
const DefaultTTL = 5 * time.Minute

type entry[T any] struct {
	value    T
	storedAt time.Time
}

// Cache memoizes one payload per user DID. Expiry is lazy: nothing here owns a goroutine, so a user who stops requesting simply stops costing memory at the next Invalidate or process restart.
type Cache[T any] struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[string]entry[T]
}

func New[T any](ttl time.Duration) *Cache[T] {
	return newWithClock[T](ttl, time.Now)
}

func newWithClock[T any](ttl time.Duration, now func() time.Time) *Cache[T] {
	return &Cache[T]{ttl: ttl, now: now, entries: map[string]entry[T]{}}
}

func (c *Cache[T]) Get(did string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stored, ok := c.entries[did]
	if !ok {
		var zero T
		return zero, false
	}
	if c.now().Sub(stored.storedAt) > c.ttl {
		delete(c.entries, did)
		var zero T
		return zero, false
	}
	return stored.value, true
}

func (c *Cache[T]) Put(did string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[did] = entry[T]{value: value, storedAt: c.now()}
}

func (c *Cache[T]) Invalidate(did string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, did)
}

func (c *Cache[T]) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}

func (c *Cache[T]) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

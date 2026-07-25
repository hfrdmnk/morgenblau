package discovermemo

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

type payload struct {
	keys []string
}

// fakeClock is the injectable now func the TTL tests drive by hand.
type fakeClock struct {
	at time.Time
}

func (c *fakeClock) now() time.Time { return c.at }

func (c *fakeClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newTestClock() *fakeClock {
	return &fakeClock{at: time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)}
}

func TestCache_GetReturnsStoredValue(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)

	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{keys: []string{"https://one.example/feed"}})

	got, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa")
	if !ok {
		t.Fatal("Get after Put = miss, want hit")
	}
	if len(got.keys) != 1 || got.keys[0] != "https://one.example/feed" {
		t.Errorf("Get = %+v, want the stored payload", got)
	}
}

func TestCache_GetUnknownDIDMisses(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)

	if _, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Error("Get on an empty cache = hit, want miss")
	}
}

func TestCache_GetWithinTTLStillHits(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})

	clock.advance(DefaultTTL - time.Second)

	if _, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); !ok {
		t.Error("Get one second before the TTL = miss, want hit")
	}
}

func TestCache_GetAfterTTLMissesAndDropsTheEntry(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})

	clock.advance(DefaultTTL + time.Second)

	if _, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Fatal("Get past the TTL = hit, want miss")
	}
	if n := c.size(); n != 0 {
		t.Errorf("entries after an expired Get = %d, want 0 (expiry must be lazy but real)", n)
	}
}

func TestCache_PutRefreshesTheStoredAtStamp(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})

	clock.advance(DefaultTTL - time.Second)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{keys: []string{"https://two.example/feed"}})
	clock.advance(2 * time.Second)

	got, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa")
	if !ok {
		t.Fatal("Get after a re-Put = miss, want hit")
	}
	if len(got.keys) != 1 || got.keys[0] != "https://two.example/feed" {
		t.Errorf("Get = %+v, want the re-stored payload", got)
	}
}

func TestCache_InvalidateDropsOnlyThatDID(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})
	c.Put("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb", payload{})

	c.Invalidate("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa")

	if _, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Error("Get on the invalidated did = hit, want miss")
	}
	if _, ok := c.Get("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"); !ok {
		t.Error("Get on an untouched did = miss, want hit")
	}
}

func TestCache_InvalidateUnknownDIDIsANoop(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})

	c.Invalidate("did:plc:cccccccccccccccccccccccc")

	if _, ok := c.Get("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"); !ok {
		t.Error("Get after invalidating an unknown did = miss, want hit")
	}
}

func TestCache_InvalidateAllDropsEveryDID(t *testing.T) {
	clock := newTestClock()
	c := newWithClock[payload](DefaultTTL, clock.now)
	c.Put("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa", payload{})
	c.Put("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb", payload{})

	c.InvalidateAll()

	if n := c.size(); n != 0 {
		t.Fatalf("entries after InvalidateAll = %d, want 0", n)
	}
	if _, ok := c.Get("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"); ok {
		t.Error("Get after InvalidateAll = hit, want miss")
	}
}

func TestCache_ConcurrentUseIsRaceFree(t *testing.T) {
	c := New[payload](DefaultTTL)

	var wg sync.WaitGroup
	for i := range 8 {
		did := "did:plc:" + strconv.Itoa(i)
		wg.Add(4)
		go func() {
			defer wg.Done()
			for range 200 {
				c.Put(did, payload{keys: []string{did}})
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				c.Get(did)
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				c.Invalidate(did)
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				c.InvalidateAll()
			}
		}()
	}
	wg.Wait()
}

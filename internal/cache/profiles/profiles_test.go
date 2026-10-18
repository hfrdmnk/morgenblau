package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type fakeResolver struct {
	calls    int32
	deadline atomic.Bool
	byDID    map[syntax.DID]*identity.Identity
	err      error
}

func (f *fakeResolver) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	atomic.AddInt32(&f.calls, 1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, ok := ctx.Deadline()
	f.deadline.Store(ok)
	if f.err != nil {
		return nil, f.err
	}
	if ident, ok := f.byDID[did]; ok {
		return ident, nil
	}
	return nil, fmt.Errorf("not found")
}

type fakeFetcher struct {
	calls       int32
	displayName *string
	avatar      *string
	description *string
	err         error
}

func (f *fakeFetcher) FetchProfile(_ context.Context, _ syntax.DID, _ string) (ProfileRecord, error) {
	atomic.AddInt32(&f.calls, 1)
	return ProfileRecord{DisplayName: f.displayName, Avatar: f.avatar, Description: f.description}, f.err
}

func ptr(s string) *string { return &s }

func identityFor(t *testing.T, didStr, handleStr, pds string) (syntax.DID, *identity.Identity) {
	t.Helper()
	did, err := syntax.ParseDID(didStr)
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	handle, err := syntax.ParseHandle(handleStr)
	if err != nil {
		t.Fatalf("ParseHandle: %v", err)
	}
	ident := &identity.Identity{
		DID:    did,
		Handle: handle,
		Services: map[string]identity.ServiceEndpoint{
			"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: pds},
		},
	}
	return did, ident
}

func TestCache_GetMissThenHit(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &fakeFetcher{displayName: ptr("Alice"), avatar: ptr("https://service.example.com/avatar.jpg"), description: ptr("Reads calmly.")}

	c := New(res, fet)
	p, err := c.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Handle != "user.example.com" || p.DisplayName == nil || *p.DisplayName != "Alice" {
		t.Fatalf("first Get unexpected: %+v", p)
	}
	if p.Description == nil || *p.Description != "Reads calmly." {
		t.Fatalf("Description = %v, want \"Reads calmly.\"", p.Description)
	}

	// Second Get should hit cache: no extra fetcher / resolver calls.
	if _, err := c.Get(context.Background(), did); err != nil {
		t.Fatalf("Get(2): %v", err)
	}
	if got := atomic.LoadInt32(&res.calls); got != 1 {
		t.Errorf("resolver.calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&fet.calls); got != 1 {
		t.Errorf("fetcher.calls = %d, want 1", got)
	}
}

func TestCache_RefreshBypasses(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &fakeFetcher{displayName: ptr("Alice")}

	c := New(res, fet)
	if _, err := c.Get(context.Background(), did); err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Simulate user editing their Bluesky display name.
	fet.displayName = ptr("Alice Smith")
	p, err := c.Refresh(context.Background(), did)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if p.DisplayName == nil || *p.DisplayName != "Alice Smith" {
		t.Errorf("displayName = %v, want Alice Smith", p.DisplayName)
	}
	// Subsequent Get should serve the refreshed value.
	p2, _ := c.Get(context.Background(), did)
	if p2.DisplayName == nil || *p2.DisplayName != "Alice Smith" {
		t.Errorf("post-refresh Get returned stale: %+v", p2)
	}
}

func TestCache_MissingProfileRecordCollapsesToNulls(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &fakeFetcher{err: fmt.Errorf("record not found")}

	c := New(res, fet)
	p, err := c.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.DisplayName != nil || p.Avatar != nil || p.Description != nil {
		t.Errorf("expected nulls, got %+v", p)
	}
	if p.Handle != "user.example.com" {
		t.Errorf("handle = %q, want user.example.com", p.Handle)
	}
}

// gatedFetcher holds the first fetch until the test releases it, then answers from whatever context it was given.
type gatedFetcher struct {
	displayName *string
	gate        sync.Once
	entered     chan struct{}
	proceed     chan struct{}
	calls       atomic.Int32
	deadline    atomic.Bool
}

func newGatedFetcher(displayName string) *gatedFetcher {
	return &gatedFetcher{displayName: &displayName, entered: make(chan struct{}), proceed: make(chan struct{})}
}

func (f *gatedFetcher) FetchProfile(ctx context.Context, _ syntax.DID, _ string) (ProfileRecord, error) {
	f.calls.Add(1)
	_, ok := ctx.Deadline()
	f.deadline.Store(ok)
	f.gate.Do(func() {
		close(f.entered)
		<-f.proceed
	})
	if err := ctx.Err(); err != nil {
		return ProfileRecord{}, err
	}
	return ProfileRecord{DisplayName: f.displayName}, nil
}

// A caller that gives up mid-fetch must not leave a nameless profile cached for everyone after it.
func TestCache_CallerCancelMidFetchDoesNotCacheNullProfile(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := newGatedFetcher("Alice")
	c := New(res, fet)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Get(ctx, did)
	}()
	<-fet.entered
	cancel()
	close(fet.proceed)
	<-done

	p, err := c.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.DisplayName == nil || *p.DisplayName != "Alice" {
		t.Fatalf("displayName = %v, want Alice", p.DisplayName)
	}
	if !res.deadline.Load() || !fet.deadline.Load() {
		t.Fatalf("deadline seen by resolver = %v, fetcher = %v; want both bounded", res.deadline.Load(), fet.deadline.Load())
	}
}

// A waiter coalesced onto a leader whose caller gives up must still get the real profile.
func TestCache_CoalescedWaiterSurvivesLeaderCancel(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	// Built outside the bubble: the expirable LRU runs a ticker goroutine that never exits.
	c := New(res, nil)

	synctest.Test(t, func(t *testing.T) {
		fet := newGatedFetcher("Alice")
		c.fetcher = fet

		ctxA, cancelA := context.WithCancel(context.Background())
		doneA := make(chan struct{})
		go func() {
			defer close(doneA)
			_, _ = c.Get(ctxA, did)
		}()
		<-fet.entered

		var pB Profile
		var errB error
		doneB := make(chan struct{})
		go func() {
			defer close(doneB)
			pB, errB = c.Get(context.Background(), did)
		}()
		synctest.Wait()

		cancelA()
		close(fet.proceed)
		<-doneA
		<-doneB

		if errB != nil {
			t.Fatalf("waiter Get: %v", errB)
		}
		if pB.DisplayName == nil || *pB.DisplayName != "Alice" {
			t.Fatalf("waiter displayName = %v, want Alice", pB.DisplayName)
		}
		if got := fet.calls.Load(); got != 1 {
			t.Fatalf("fetcher calls = %d, want 1 (waiter must coalesce)", got)
		}
	})
}

// A degraded profile heals after degradedTTL, while a complete one keeps the full TTL.
func TestCache_DegradedProfileExpiresAfterShortTTL(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &fakeFetcher{err: fmt.Errorf("service unavailable")}
	c := New(res, fet)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	if p, _ := c.Get(context.Background(), did); p.DisplayName != nil {
		t.Fatalf("degraded displayName = %v, want nil", *p.DisplayName)
	}
	fet.err = nil
	fet.displayName = ptr("Alice")

	now = now.Add(degradedTTL - time.Second)
	if p, _ := c.Get(context.Background(), did); p.DisplayName != nil {
		t.Fatalf("displayName before degradedTTL = %v, want the cached nil", *p.DisplayName)
	}

	now = now.Add(2 * time.Second)
	p, err := c.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.DisplayName == nil || *p.DisplayName != "Alice" {
		t.Fatalf("displayName after degradedTTL = %v, want Alice", p.DisplayName)
	}

	fet.displayName = ptr("Alice Smith")
	now = now.Add(time.Hour)
	if p, _ := c.Get(context.Background(), did); p.DisplayName == nil || *p.DisplayName != "Alice" {
		t.Fatalf("complete profile displayName = %v, want the cached Alice", p.DisplayName)
	}
}

func TestCache_DescriptionInJSON(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &fakeFetcher{description: ptr("Reads calmly.")}

	c := New(res, fet)
	p, err := c.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"description":"Reads calmly."`) {
		t.Errorf("json = %s, want description field", b)
	}
}

func TestCache_DescriptionOmittedNullInJSON(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &fakeFetcher{}

	c := New(res, fet)
	p, err := c.Get(context.Background(), did)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"description":null`) {
		t.Errorf("json = %s, want null description field", b)
	}
}

func TestCache_HandleInvalid_Errors(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	ident := &identity.Identity{DID: did, Handle: syntax.HandleInvalid}
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	c := New(res, &fakeFetcher{})
	if _, err := c.Get(context.Background(), did); err == nil {
		t.Fatal("expected error on invalid handle")
	}
}

type blockingResolver struct{ calls atomic.Int32 }

func (r *blockingResolver) LookupDID(ctx context.Context, _ syntax.DID) (*identity.Identity, error) {
	r.calls.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}

// A caller with its own deadline gets its answer by that deadline, not after the shared fetch timeout.
func TestCache_LookupGivesUpWithTheCaller(t *testing.T) {
	c := New(&blockingResolver{}, nil)
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		if _, err := c.Lookup(ctx, "did:plc:slow"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Lookup err = %v, want the caller's deadline", err)
		}
		if waited := time.Since(start); waited != time.Second {
			t.Fatalf("Lookup returned after %v, want the caller's 1s deadline", waited)
		}
	})
}

func TestCache_LookupSharesTheCache(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	c := New(res, &fakeFetcher{displayName: ptr("Alice")})
	if p, err := c.Lookup(context.Background(), did); err != nil || p.DisplayName == nil || *p.DisplayName != "Alice" {
		t.Fatalf("Lookup = %+v, %v", p, err)
	}
	if p, err := c.Get(context.Background(), did); err != nil || p.Handle != "user.example.com" {
		t.Fatalf("Get after Lookup = %+v, %v", p, err)
	}
	if got := atomic.LoadInt32(&res.calls); got != 1 {
		t.Fatalf("resolver calls = %d, want 1; Get must reuse what Lookup cached", got)
	}
}

// Giving up mid-fetch must not cache the nameless profile the record failure would otherwise degrade to.
func TestCache_LookupCancelMidFetchCachesNothing(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := newGatedFetcher("Alice")
	c := New(res, fet)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := c.Lookup(ctx, did)
		done <- err
	}()
	<-fet.entered
	cancel()
	close(fet.proceed)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Lookup err = %v, want the caller's cancellation", err)
	}
	p, err := c.Get(context.Background(), did)
	if err != nil || p.DisplayName == nil || *p.DisplayName != "Alice" {
		t.Fatalf("Get after an abandoned Lookup = %+v, %v; want the real profile, not a cached nameless one", p, err)
	}
}

// Past its deadline a caller still gets cached profiles but starts no lookup, since identity resolution may ignore cancellation.
func TestCache_LookupPastTheDeadlineServesOnlyTheCache(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	c := New(res, &fakeFetcher{})
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Lookup(expired, "did:plc:uncached"); !errors.Is(err, context.Canceled) {
		t.Fatalf("uncached Lookup err = %v, want the caller's cancellation", err)
	}
	if got := atomic.LoadInt32(&res.calls); got != 0 {
		t.Fatalf("resolver calls = %d, want 0 after the caller gave up", got)
	}
	if _, err := c.Get(context.Background(), did); err != nil {
		t.Fatal(err)
	}
	if p, err := c.Lookup(expired, did); err != nil || p.Handle != "user.example.com" {
		t.Fatalf("cached Lookup past the deadline = %+v, %v; want the cached profile", p, err)
	}
}

// pastDeadline is a caller whose deadline has passed before its Done channel closed, the window in which a socket deadline copied from it has already fired.
type pastDeadline struct{ context.Context }

func (pastDeadline) Deadline() (time.Time, bool) { return time.Now().Add(-time.Millisecond), true }

type timeoutFetcher struct{ calls atomic.Int32 }

func (f *timeoutFetcher) FetchProfile(context.Context, syntax.DID, string) (ProfileRecord, error) {
	f.calls.Add(1)
	return ProfileRecord{}, errors.New("dial tcp: i/o timeout")
}

func TestCache_LookupPastItsDeadlineCachesNoNamelessProfile(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	fet := &timeoutFetcher{}
	c := New(res, fet)
	if _, err := c.Lookup(pastDeadline{context.Background()}, did); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lookup err = %v, want the caller's deadline", err)
	}
	if got := fet.calls.Load(); got != 0 {
		t.Fatalf("fetcher calls = %d, want none once the caller's deadline has passed", got)
	}
	if _, ok := c.lru.Get(did.String()); ok {
		t.Fatal("a caller past its deadline cached a profile")
	}
}

// A record fetch that times out on the caller's deadline before its context reports done must still count as the caller giving up.
func TestCache_LookupRecordTimeoutAtTheDeadlineCachesNothing(t *testing.T) {
	did, ident := identityFor(t, "did:plc:alice", "user.example.com", "https://service.example.com")
	res := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{did: ident}}
	c := New(res, nil)
	synctest.Test(t, func(t *testing.T) {
		c.fetcher = deadlineFetcher{}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := c.Lookup(callerDeadlineOnly{ctx}, did); err == nil {
			t.Fatal("Lookup succeeded with a degraded profile fetched past the caller's deadline")
		}
		if _, ok := c.lru.Get(did.String()); ok {
			t.Fatal("cached a nameless profile for a caller that ran out of time")
		}
	})
}

// callerDeadlineOnly exposes its parent's deadline but never reports done, like a context whose timer has not fired yet.
type callerDeadlineOnly struct{ context.Context }

func (callerDeadlineOnly) Done() <-chan struct{} { return nil }
func (callerDeadlineOnly) Err() error            { return nil }

// deadlineFetcher fails the way a dial does when its socket deadline, copied from ctx, fires.
type deadlineFetcher struct{}

func (deadlineFetcher) FetchProfile(ctx context.Context, _ syntax.DID, _ string) (ProfileRecord, error) {
	d, _ := ctx.Deadline()
	time.Sleep(time.Until(d))
	return ProfileRecord{}, errors.New("dial tcp: i/o timeout")
}

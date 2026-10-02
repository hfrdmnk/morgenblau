package profiles

import (
	"context"
	"encoding/json"
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

// Package profiles is a small in-memory LRU cache that resolves a DID to a profile via identity + PDS lookup, behind Get/Refresh.
package profiles

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

const (
	defaultCapacity = 10_000
	defaultTTL      = 6 * time.Hour
)

// Profile is the cached shape returned to handlers.
type Profile struct {
	DID         string  `json:"did"`
	Handle      string  `json:"handle"`
	DisplayName *string `json:"displayName"`
	Avatar      *string `json:"avatar"`
	Description *string `json:"description"`
}

// ProfileRecord is the subset of app.bsky.actor.profile/self a RecordFetcher parses.
type ProfileRecord struct {
	DisplayName *string
	Avatar      *string
	Description *string
}

// Resolver mirrors the slice of identity.Directory we use. Stubbed in tests.
type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

// RecordFetcher reads app.bsky.actor.profile/self from a PDS endpoint; an absent record must return a zero ProfileRecord rather than an error.
type RecordFetcher interface {
	FetchProfile(ctx context.Context, did syntax.DID, pdsEndpoint string) (ProfileRecord, error)
}

// Cache resolves DIDs to a profile payload via an expirable LRU. Concurrency-safe.
type Cache struct {
	lru      *lru.LRU[string, Profile]
	resolver Resolver
	fetcher  RecordFetcher

	// guards the in-flight singleflight map: prevents duplicate PDS round-trips for the same DID.
	mu       sync.Mutex
	inflight map[string]*inflight
}

type inflight struct {
	done    chan struct{}
	profile Profile
	err     error
}

// New constructs a cache with the default capacity (10k entries) and TTL (6h).
func New(resolver Resolver, fetcher RecordFetcher) *Cache {
	return NewWithOptions(resolver, fetcher, defaultCapacity, defaultTTL)
}

// NewWithOptions is the test-friendly constructor.
func NewWithOptions(resolver Resolver, fetcher RecordFetcher, capacity int, ttl time.Duration) *Cache {
	return &Cache{
		lru:      lru.NewLRU[string, Profile](capacity, nil, ttl),
		resolver: resolver,
		fetcher:  fetcher,
		inflight: make(map[string]*inflight),
	}
}

// Get returns the profile for did, resolving and back-filling on a cache miss; concurrent misses for the same DID collapse to one upstream load.
func (c *Cache) Get(ctx context.Context, did syntax.DID) (Profile, error) {
	key := did.String()
	if p, ok := c.lru.Get(key); ok {
		return p, nil
	}
	return c.load(ctx, did, true)
}

// Refresh re-fetches bypassing the cache and re-stores the result; used by the self-view path so users see their own profile edits immediately.
func (c *Cache) Refresh(ctx context.Context, did syntax.DID) (Profile, error) {
	return c.load(ctx, did, false)
}

func (c *Cache) load(ctx context.Context, did syntax.DID, useInflight bool) (Profile, error) {
	key := did.String()

	if useInflight {
		c.mu.Lock()
		if wait, ok := c.inflight[key]; ok {
			c.mu.Unlock()
			select {
			case <-wait.done:
				return wait.profile, wait.err
			case <-ctx.Done():
				return Profile{}, ctx.Err()
			}
		}
		f := &inflight{done: make(chan struct{})}
		c.inflight[key] = f
		c.mu.Unlock()

		defer func() {
			c.mu.Lock()
			delete(c.inflight, key)
			c.mu.Unlock()
			close(f.done)
		}()

		profile, err := c.fetch(ctx, did)
		f.profile = profile
		f.err = err
		if err == nil {
			c.lru.Add(key, profile)
		}
		return profile, err
	}

	profile, err := c.fetch(ctx, did)
	if err == nil {
		c.lru.Add(key, profile)
	}
	return profile, err
}

// ErrHandleInvalid means bidirectional handle verification failed; callers must surface a 500, never a sentinel handle.
var ErrHandleInvalid = errors.New("bidirectional handle verification failed")

// ErrNoPDS is returned when the identity has no atproto_pds service endpoint.
var ErrNoPDS = errors.New("identity has no PDS endpoint")

func (c *Cache) fetch(ctx context.Context, did syntax.DID) (Profile, error) {
	ident, err := c.resolver.LookupDID(ctx, did)
	if err != nil {
		return Profile{}, fmt.Errorf("resolve did: %w", err)
	}
	if ident.Handle.IsInvalidHandle() {
		return Profile{}, ErrHandleInvalid
	}
	endpoint := ident.PDSEndpoint()
	p := Profile{
		DID:    did.String(),
		Handle: ident.Handle.String(),
	}
	if endpoint == "" {
		return p, nil
	}
	record, err := c.fetcher.FetchProfile(ctx, did, endpoint)
	if err != nil {
		// Profile fetch failure is non-fatal: collapse to nulls so chrome still renders with the handle.
		return p, nil
	}
	p.DisplayName = record.DisplayName
	p.Avatar = record.Avatar
	p.Description = record.Description
	return p, nil
}

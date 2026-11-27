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
	degradedTTL     = 2 * time.Minute
	fetchTimeout    = 15 * time.Second
)

type Profile struct {
	DID         string  `json:"did"`
	Handle      string  `json:"handle"`
	DisplayName *string `json:"displayName"`
	Avatar      *string `json:"avatar"`
	Description *string `json:"description"`
}

type ProfileRecord struct {
	DisplayName *string
	Avatar      *string
	Description *string
}

type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

type RecordFetcher interface {
	FetchProfile(ctx context.Context, did syntax.DID, pdsEndpoint string) (ProfileRecord, error)
}

type Cache struct {
	lru      *lru.LRU[string, entry]
	now      func() time.Time
	resolver Resolver
	fetcher  RecordFetcher

	mu       sync.Mutex
	inflight map[string]*inflight
}

type entry struct {
	profile       Profile
	degradedUntil time.Time
}

type inflight struct {
	done    chan struct{}
	profile Profile
	err     error
}

func New(resolver Resolver, fetcher RecordFetcher) *Cache {
	return NewWithOptions(resolver, fetcher, defaultCapacity, defaultTTL)
}

func NewWithOptions(resolver Resolver, fetcher RecordFetcher, capacity int, ttl time.Duration) *Cache {
	return &Cache{
		lru:      lru.NewLRU[string, entry](capacity, nil, ttl),
		now:      time.Now,
		resolver: resolver,
		fetcher:  fetcher,
		inflight: make(map[string]*inflight),
	}
}

func (c *Cache) Get(ctx context.Context, did syntax.DID) (Profile, error) {
	key := did.String()
	if e, ok := c.lru.Get(key); ok && (e.degradedUntil.IsZero() || c.now().Before(e.degradedUntil)) {
		return e.profile, nil
	}
	return c.load(ctx, did, true)
}

func (c *Cache) Refresh(ctx context.Context, did syntax.DID) (Profile, error) {
	return c.load(ctx, did, false)
}

func (c *Cache) Lookup(ctx context.Context, did syntax.DID) (Profile, error) {
	key := did.String()
	if e, ok := c.lru.Get(key); ok && (e.degradedUntil.IsZero() || c.now().Before(e.degradedUntil)) {
		return e.profile, nil
	}
	if err := gaveUp(ctx); err != nil {
		return Profile{}, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	e, err := c.resolve(fetchCtx, did)
	if err == nil && !e.degradedUntil.IsZero() {
		err = gaveUp(ctx)
	}
	if err != nil {
		return Profile{}, err
	}
	c.lru.Add(key, e)
	return e.profile, nil
}

func gaveUp(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d, ok := ctx.Deadline(); ok && !time.Now().Before(d) {
		return context.DeadlineExceeded
	}
	return nil
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

		e, err := c.fetch(ctx, did)
		f.profile = e.profile
		f.err = err
		if err == nil {
			c.lru.Add(key, e)
		}
		return e.profile, err
	}

	e, err := c.fetch(ctx, did)
	if err == nil {
		c.lru.Add(key, e)
	}
	return e.profile, err
}

var ErrHandleInvalid = errors.New("bidirectional handle verification failed")

var ErrNoPDS = errors.New("identity has no PDS endpoint")

func (c *Cache) fetch(ctx context.Context, did syntax.DID) (entry, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
	defer cancel()
	return c.resolve(ctx, did)
}

func (c *Cache) resolve(ctx context.Context, did syntax.DID) (entry, error) {
	ident, err := c.resolver.LookupDID(ctx, did)
	if err != nil {
		return entry{}, fmt.Errorf("resolve did: %w", err)
	}
	if ident.Handle.IsInvalidHandle() {
		return entry{}, ErrHandleInvalid
	}
	endpoint := ident.PDSEndpoint()
	p := Profile{
		DID:    did.String(),
		Handle: ident.Handle.String(),
	}
	if endpoint == "" {
		return entry{profile: p}, nil
	}
	record, err := c.fetcher.FetchProfile(ctx, did, endpoint)
	if err != nil {
		return entry{profile: p, degradedUntil: c.now().Add(degradedTTL)}, nil
	}
	p.DisplayName = record.DisplayName
	p.Avatar = record.Avatar
	p.Description = record.Description
	return entry{profile: p}, nil
}

// Package atidentity builds an atproto identity directory whose HTTP fetches pass an SSRF guard, since did:web/did:plc/handle well-known lookups resolve attacker-supplied authorities.
package atidentity

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// lookupTimeout bounds a lookup that callers can no longer cancel.
const lookupTimeout = 10 * time.Second

// Guarded mirrors indigo's DefaultDirectory but swaps in client (the safehttp client) for identity HTTP fetches, preserving DNS/PLC settings.
// The SSRF guard lives on client's dial Control, so it fires against the resolved peer IP at connect time and defeats DNS rebinding.
func Guarded(client *http.Client) identity.Directory {
	base := identity.BaseDirectory{
		PLCURL:     identity.DefaultPLCURL,
		HTTPClient: *client,
		Resolver: net.Resolver{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, address)
			},
		},
		TryAuthoritativeDNS:   true,
		SkipDNSDomainSuffixes: []string{".bsky.social"},
		UserAgent:             "morgenblau-identity",
	}
	return identity.NewCacheDirectory(detached{&base}, 250_000, 24*time.Hour, 2*time.Minute, 5*time.Minute)
}

// detached shields lookups from caller cancellation, because CacheDirectory caches whatever its inner directory returns and shares it with every later caller.
type detached struct{ inner identity.Directory }

func (d detached) LookupHandle(ctx context.Context, h syntax.Handle) (*identity.Identity, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
	defer cancel()
	return d.inner.LookupHandle(ctx, h)
}

func (d detached) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
	defer cancel()
	return d.inner.LookupDID(ctx, did)
}

func (d detached) Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
	defer cancel()
	return d.inner.Lookup(ctx, atid)
}

func (d detached) Purge(ctx context.Context, atid syntax.AtIdentifier) error {
	return d.inner.Purge(ctx, atid)
}

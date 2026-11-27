package atidentity

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const lookupTimeout = 10 * time.Second

func Guarded(client *http.Client) identity.Directory {
	return cached(guardedBase(client))
}

func guardedBase(client *http.Client) *identity.BaseDirectory {
	return &identity.BaseDirectory{
		PLCURL:     identity.DefaultPLCURL,
		HTTPClient: *client,
		Resolver: net.Resolver{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, address)
			},
		},
		TryAuthoritativeDNS: true,
		UserAgent:           "morgenblau-identity",
	}
}

func cached(inner identity.Directory) identity.Directory {
	return identity.NewCacheDirectory(detached{inner}, 250_000, 24*time.Hour, 2*time.Minute, 5*time.Minute)
}

type detached struct{ inner identity.Directory }

func detach(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
}

func (d detached) LookupHandle(ctx context.Context, h syntax.Handle) (*identity.Identity, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	return d.inner.LookupHandle(ctx, h)
}

func (d detached) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	return d.inner.LookupDID(ctx, did)
}

func (d detached) Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	return d.inner.Lookup(ctx, atid)
}

func (d detached) Purge(ctx context.Context, atid syntax.AtIdentifier) error {
	return d.inner.Purge(ctx, atid)
}

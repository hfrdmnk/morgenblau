// Package atidentity builds an atproto identity directory whose HTTP fetches
// pass an SSRF guard. did:web, did:plc, and handle well-known lookups resolve
// attacker-supplied authorities, so those fetches must not be able to reach
// loopback or private addresses.
package atidentity

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
)

// Guarded mirrors indigo's DefaultDirectory but swaps in client (the safehttp
// client) for identity HTTP fetches. DNS and PLC settings are preserved; the
// SSRF guard lives on client's dial Control, so it fires against the resolved
// peer IP at connect time and defeats DNS rebinding.
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
	return identity.NewCacheDirectory(&base, 250_000, 24*time.Hour, 2*time.Minute, 5*time.Minute)
}

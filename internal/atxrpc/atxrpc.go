// Package atxrpc constructs indigo XRPC clients with Morgenblau's outbound identity; indigo's default leaks "indigo-sdk" otherwise.
package atxrpc

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"

	"morgenblau/internal/safehttp"
)

// defaultCooldown is the one seam that reaches every atxrpc call site; threading it through six constructors would buy nothing.
var defaultCooldown = &HostCooldown{}

// New builds an APIClient for host with Morgenblau's User-Agent and the shared per-host rate-limit cooldown.
func New(host string, httpClient *http.Client) *atclient.APIClient {
	c := atclient.NewAPIClient(host)
	c.Client = withCooldown(httpClient, defaultCooldown)
	c.Headers.Set("User-Agent", safehttp.UserAgent)
	return c
}

// withCooldown clones base so wrapping the transport never mutates a client the caller shares elsewhere.
func withCooldown(base *http.Client, cd *HostCooldown) *http.Client {
	clone := &http.Client{}
	if base != nil {
		*clone = *base
	}
	inner := clone.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}
	clone.Transport = &cooldownTransport{inner: inner, cooldown: cd}
	return clone
}

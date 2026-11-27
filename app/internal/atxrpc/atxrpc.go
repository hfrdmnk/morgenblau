package atxrpc

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"

	"morgenblau/internal/safehttp"
)

var defaultCooldown = &HostCooldown{}

func New(host string, httpClient *http.Client) *atclient.APIClient {
	c := atclient.NewAPIClient(host)
	c.Client = withCooldown(httpClient, defaultCooldown)
	c.Headers.Set("User-Agent", safehttp.UserAgent)
	return c
}

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

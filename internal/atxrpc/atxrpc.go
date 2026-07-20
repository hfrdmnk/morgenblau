// Package atxrpc constructs indigo XRPC clients with Morgenblau's outbound identity; indigo's default leaks "indigo-sdk" otherwise.
package atxrpc

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"

	"morgenblau/internal/safehttp"
)

// New builds an APIClient for host with Morgenblau's User-Agent; a nil httpClient keeps http.DefaultClient.
func New(host string, httpClient *http.Client) *atclient.APIClient {
	c := atclient.NewAPIClient(host)
	if httpClient != nil {
		c.Client = httpClient
	}
	c.Headers.Set("User-Agent", safehttp.UserAgent)
	return c
}

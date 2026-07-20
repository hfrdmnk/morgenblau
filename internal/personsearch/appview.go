// Package personsearch proxies Bluesky AppView person typeahead and badges the
// results with reader-network presence (SPEC <discovery> People "Search").
package personsearch

import (
	"context"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
)

// Actor is the trimmed shape of one app.bsky.actor.searchActorsTypeahead hit.
type Actor struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"` // AppView CDN URL, passed through verbatim.
}

type typeaheadResp struct {
	Actors []Actor `json:"actors"`
}

// AppView adapts the Bluesky AppView typeahead XRPC query to a []Actor.
type AppView struct {
	client *atclient.APIClient
}

// NewAppView points an AppView at host (env APPVIEW_HOST, default
// https://public.api.bsky.app; resolved by the caller) over httpClient.
func NewAppView(host string, httpClient *http.Client) *AppView {
	return &AppView{client: atxrpc.New(host, httpClient)}
}

// SearchActorsTypeahead runs app.bsky.actor.searchActorsTypeahead for q, capped at limit.
func (a *AppView) SearchActorsTypeahead(ctx context.Context, q string, limit int) ([]Actor, error) {
	var out typeaheadResp
	params := map[string]any{
		"q":     q,
		"limit": limit,
	}
	if err := a.client.Get(ctx, syntax.NSID("app.bsky.actor.searchActorsTypeahead"), params, &out); err != nil {
		return nil, err
	}
	return out.Actors, nil
}

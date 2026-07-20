// Package leafletfeed reads pub.leaflet.publication records from publisher repos via unauthenticated XRPC.
package leafletfeed

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
)

const CollectionPublication = "pub.leaflet.publication"

// Publication is a decoded pub.leaflet.publication record.
// URI is DID-normalized so it matches other feed records regardless of the lookup authority.
type Publication struct {
	URI            string
	DID            string
	Name           string
	BasePath       string // normalized: scheme stripped, slashes trimmed, host lowercased
	URL            string // "https://" + BasePath; empty when the record has no base_path
	FeedURL        string // URL + "/rss"; empty when the record has no base_path
	Description    string
	ShowInDiscover bool
}

// Resolver is the identity lookup the client needs; atid may be a DID or a handle since at-uri authorities allow either.
type Resolver interface {
	Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error)
}

// Client reads records from arbitrary publisher repos.
type Client struct {
	resolver Resolver
	http     *http.Client
}

// NewClient builds a Client; httpClient must be the safehttp client, since PDS
// endpoints come from attacker-controllable DID documents.
func NewClient(resolver Resolver, httpClient *http.Client) *Client {
	return &Client{resolver: resolver, http: httpClient}
}

type getRecordResp struct {
	URI   string         `json:"uri"`
	Value map[string]any `json:"value"`
}

// GetPublication fetches and maps a pub.leaflet.publication record, DID-normalizing its URI.
func (c *Client) GetPublication(ctx context.Context, rawURI string) (*Publication, error) {
	uri, err := syntax.ParseATURI(rawURI)
	if err != nil {
		return nil, fmt.Errorf("leafletfeed: invalid at-uri %q: %w", rawURI, err)
	}
	if uri.Collection().String() != CollectionPublication {
		return nil, fmt.Errorf("leafletfeed: %q is not a %s record", rawURI, CollectionPublication)
	}

	ident, err := c.resolver.Lookup(ctx, uri.Authority())
	if err != nil {
		return nil, fmt.Errorf("leafletfeed: resolve %s: %w", uri.Authority(), err)
	}
	endpoint := ident.PDSEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("leafletfeed: no PDS endpoint for %s", ident.DID)
	}

	var out getRecordResp
	params := map[string]any{
		"repo":       ident.DID.String(),
		"collection": CollectionPublication,
		"rkey":       uri.RecordKey().String(),
	}
	if err := atxrpc.New(endpoint, c.http).Get(ctx, syntax.NSID("com.atproto.repo.getRecord"), params, &out); err != nil {
		return nil, fmt.Errorf("leafletfeed: getRecord %s: %w", rawURI, err)
	}

	name, _ := out.Value["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("leafletfeed: publication %s missing required name", rawURI)
	}
	didURI := out.URI
	if didURI == "" {
		didURI = fmt.Sprintf("at://%s/%s/%s", ident.DID, CollectionPublication, uri.RecordKey())
	}

	pub := &Publication{
		URI:            didURI,
		DID:            ident.DID.String(),
		Name:           name,
		ShowInDiscover: true,
	}
	pub.Description, _ = out.Value["description"].(string)
	if basePath, _ := out.Value["base_path"].(string); basePath != "" {
		pub.BasePath = normalizeBasePath(basePath)
		pub.URL = "https://" + pub.BasePath
		pub.FeedURL = pub.URL + "/rss"
	}
	if prefs, ok := out.Value["preferences"].(map[string]any); ok {
		if show, ok := prefs["showInDiscover"].(bool); ok {
			pub.ShowInDiscover = show
		}
	}
	return pub, nil
}

// normalizeBasePath strips an optional scheme and outer slashes, lowercasing only the host portion so a path segment keeps its case.
func normalizeBasePath(raw string) string {
	s := strings.TrimPrefix(raw, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.Trim(s, "/")
	host, rest, hasPath := strings.Cut(s, "/")
	host = strings.ToLower(host)
	if hasPath {
		return host + "/" + rest
	}
	return host
}

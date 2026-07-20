// Package standardfeed reads site.standard.* records from publisher repos via unauthenticated XRPC.
// SPEC <sync-architecture> Tier-2 kinds.
package standardfeed

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	CollectionPublication  = "site.standard.publication"
	CollectionDocument     = "site.standard.document"
	CollectionSubscription = "site.standard.graph.subscription"
	CollectionRecommend    = "site.standard.graph.recommend"
)

// Publication is the trimmed shape of a site.standard.publication record.
// URI is DID-normalized so it matches document site fields regardless of the lookup authority.
type Publication struct {
	URI            string
	CID            string
	DID            string
	Name           string
	URL            string // base publication URL, trailing slash stripped
	Description    string
	IconURL        string // com.atproto.sync.getBlob URL; empty when no icon
	ShowInDiscover bool
}

// Document is the trimmed shape of a site.standard.document record; Site is the publication at-uri or an https URL for loose documents.
type Document struct {
	URI           string
	CID           string
	Site          string
	Title         string
	Path          string
	Description   string
	TextContent   string
	PublishedAt   string
	UpdatedAt     string
	Tags          []string
	CoverImageURL string
}

// Resolver is the identity lookup the client needs; atid may be a DID or a handle since at-uri authorities allow either.
type Resolver interface {
	Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error)
}

// Client reads records from arbitrary publisher repos. httpClient must be the
// safehttp client: PDS endpoints come from attacker-controllable DID documents.
type Client struct {
	resolver Resolver
	http     *http.Client
}

func NewClient(resolver Resolver, httpClient *http.Client) *Client {
	return &Client{resolver: resolver, http: httpClient}
}

func blobURL(pdsEndpoint string, did syntax.DID, cid string) string {
	u, err := url.Parse(pdsEndpoint)
	if err != nil {
		return ""
	}
	u.Path = "/xrpc/com.atproto.sync.getBlob"
	q := u.Query()
	q.Set("did", did.String())
	q.Set("cid", cid)
	u.RawQuery = q.Encode()
	return u.String()
}

// $link is atproto's blob-ref CID field.
func blobRefCID(v any) string {
	blob, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	ref, ok := blob["ref"].(map[string]any)
	if !ok {
		return ""
	}
	link, _ := ref["$link"].(string)
	return link
}

// stringSlice skips non-string members instead of failing on malformed input.
func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimTrailingSlash(s string) string {
	return strings.TrimRight(s, "/")
}

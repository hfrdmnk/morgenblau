// Package standardfeed reads Standardfeed (site.standard.*) records from
// publisher repos: identity-resolved, unauthenticated XRPC against the
// publisher's PDS. It is the ingestion counterpart to RSS fetching — see
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
// URI is always DID-normalized (at://did:.../site.standard.publication/rkey)
// so it can serve as the Tier-2 catalog key and match document site fields.
type Publication struct {
	URI         string
	CID         string
	DID         string
	Name        string
	URL         string // base publication URL, trailing slash stripped
	Description string
	IconURL     string // com.atproto.sync.getBlob URL; empty when no icon
}

// Document is the trimmed shape of a site.standard.document record. Site is
// either the publication at-uri or an https URL (loose document).
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

// Resolver is the slice of indigo's identity.Directory the client needs. The
// at-uri authority may be a DID or a handle, so Lookup takes either.
type Resolver interface {
	Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error)
}

// Client reads records from arbitrary publisher repos. httpClient should be
// the safehttp client: PDS endpoints come from attacker-controllable DID
// documents, so every request must pass the SSRF guard.
type Client struct {
	resolver Resolver
	http     *http.Client
}

func NewClient(resolver Resolver, httpClient *http.Client) *Client {
	return &Client{resolver: resolver, http: httpClient}
}

// blobURL builds the public com.atproto.sync.getBlob URL for a blob CID.
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

// blobRefCID pulls the $link CID out of a decoded blob field.
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

// stringSlice extracts a []string from a decoded JSON array, skipping
// non-string members. Returns nil when v isn't an array or ends up empty.
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

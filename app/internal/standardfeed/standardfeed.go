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
)

type Publication struct {
	URI         string
	CID         string
	DID         string
	Name        string
	URL         string
	Description string
	IconURL     string
}

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

type Resolver interface {
	Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error)
}

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

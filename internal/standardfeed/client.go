package standardfeed

import (
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type getRecordResp struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

type listRecordsResp struct {
	Records []recordEntry `json:"records"`
	Cursor  string        `json:"cursor"`
}

type recordEntry struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

// repoRef is a resolved at-uri: the repo's DID, its PDS endpoint, and the
// parsed uri itself.
type repoRef struct {
	did      syntax.DID
	endpoint string
	uri      syntax.ATURI
}

func (c *Client) resolveRepo(ctx context.Context, rawURI, wantCollection string) (repoRef, error) {
	uri, err := syntax.ParseATURI(rawURI)
	if err != nil {
		return repoRef{}, fmt.Errorf("standardfeed: invalid at-uri %q: %w", rawURI, err)
	}
	if uri.Collection().String() != wantCollection {
		return repoRef{}, fmt.Errorf("standardfeed: %q is not a %s record", rawURI, wantCollection)
	}
	ident, err := c.resolver.Lookup(ctx, uri.Authority())
	if err != nil {
		return repoRef{}, fmt.Errorf("standardfeed: resolve %s: %w", uri.Authority(), err)
	}
	endpoint := ident.PDSEndpoint()
	if endpoint == "" {
		return repoRef{}, fmt.Errorf("standardfeed: no PDS endpoint for %s", ident.DID)
	}
	return repoRef{did: ident.DID, endpoint: endpoint, uri: uri}, nil
}

func (c *Client) apiClient(endpoint string) *atclient.APIClient {
	client := atclient.NewAPIClient(endpoint)
	client.Client = c.http
	return client
}

// GetPublication fetches and maps a site.standard.publication record. The
// returned URI is DID-normalized so it matches document site fields even when
// the caller passed a handle-authority uri.
func (c *Client) GetPublication(ctx context.Context, rawURI string) (*Publication, error) {
	ref, err := c.resolveRepo(ctx, rawURI, CollectionPublication)
	if err != nil {
		return nil, err
	}
	var out getRecordResp
	params := map[string]any{
		"repo":       ref.did.String(),
		"collection": CollectionPublication,
		"rkey":       ref.uri.RecordKey().String(),
	}
	if err := c.apiClient(ref.endpoint).Get(ctx, syntax.NSID("com.atproto.repo.getRecord"), params, &out); err != nil {
		return nil, fmt.Errorf("standardfeed: getRecord %s: %w", rawURI, err)
	}

	name, _ := out.Value["name"].(string)
	pubURL, _ := out.Value["url"].(string)
	if name == "" || pubURL == "" {
		return nil, fmt.Errorf("standardfeed: publication %s missing required name/url", rawURI)
	}
	uri := out.URI
	if uri == "" {
		uri = fmt.Sprintf("at://%s/%s/%s", ref.did, CollectionPublication, ref.uri.RecordKey())
	}
	pub := &Publication{
		URI:  uri,
		CID:  out.CID,
		DID:  ref.did.String(),
		Name: name,
		URL:  trimTrailingSlash(pubURL),
	}
	pub.Description, _ = out.Value["description"].(string)
	if cid := blobRefCID(out.Value["icon"]); cid != "" {
		pub.IconURL = blobURL(ref.endpoint, ref.did, cid)
	}
	return pub, nil
}

// GetDocument fetches and maps a single site.standard.document record.
func (c *Client) GetDocument(ctx context.Context, rawURI string) (*Document, error) {
	ref, err := c.resolveRepo(ctx, rawURI, CollectionDocument)
	if err != nil {
		return nil, err
	}
	var out getRecordResp
	params := map[string]any{
		"repo":       ref.did.String(),
		"collection": CollectionDocument,
		"rkey":       ref.uri.RecordKey().String(),
	}
	if err := c.apiClient(ref.endpoint).Get(ctx, syntax.NSID("com.atproto.repo.getRecord"), params, &out); err != nil {
		return nil, fmt.Errorf("standardfeed: getRecord %s: %w", rawURI, err)
	}
	uri := out.URI
	if uri == "" {
		uri = fmt.Sprintf("at://%s/%s/%s", ref.did, CollectionDocument, ref.uri.RecordKey())
	}
	doc := toDocument(recordEntry{URI: uri, CID: out.CID, Value: out.Value}, ref)
	return &doc, nil
}

// ListDocuments pages the publisher repo's site.standard.document collection
// and returns the documents belonging to the given publication (site ==
// pubURI). A repo can host several publications and loose documents; those
// are filtered out here. Terminates only on empty cursor — stopping early
// would let the sweep's diff hard-delete entries missing from a partial
// snapshot.
func (c *Client) ListDocuments(ctx context.Context, pubURI string) ([]Document, error) {
	ref, err := c.resolveRepo(ctx, pubURI, CollectionPublication)
	if err != nil {
		return nil, err
	}
	client := c.apiClient(ref.endpoint)

	// Documents reference the publication by its DID-form at-uri. The caller
	// may have passed a handle authority (adopted verbatim from another app's
	// subscription record), so match the site field against both forms.
	didURI := fmt.Sprintf("at://%s/%s/%s", ref.did, CollectionPublication, ref.uri.RecordKey())

	var (
		out    []Document
		cursor string
	)
	for {
		var resp listRecordsResp
		params := map[string]any{
			"repo":       ref.did.String(),
			"collection": CollectionDocument,
			"limit":      100,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &resp); err != nil {
			return nil, fmt.Errorf("standardfeed: listRecords %s: %w", pubURI, err)
		}
		for _, r := range resp.Records {
			if site, _ := r.Value["site"].(string); site != pubURI && site != didURI {
				continue
			}
			out = append(out, toDocument(r, ref))
		}
		if resp.Cursor == "" {
			return out, nil
		}
		cursor = resp.Cursor
	}
}

func toDocument(r recordEntry, ref repoRef) Document {
	doc := Document{URI: r.URI, CID: r.CID}
	doc.Site, _ = r.Value["site"].(string)
	doc.Title, _ = r.Value["title"].(string)
	doc.Path, _ = r.Value["path"].(string)
	doc.Description, _ = r.Value["description"].(string)
	doc.TextContent, _ = r.Value["textContent"].(string)
	doc.PublishedAt, _ = r.Value["publishedAt"].(string)
	doc.UpdatedAt, _ = r.Value["updatedAt"].(string)
	doc.Tags = stringSlice(r.Value["tags"])
	if cid := blobRefCID(r.Value["coverImage"]); cid != "" {
		doc.CoverImageURL = blobURL(ref.endpoint, ref.did, cid)
	}
	return doc
}

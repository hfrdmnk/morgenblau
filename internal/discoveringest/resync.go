package discoveringest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
)

const (
	// resyncTimeout bounds one repo's whole re-crawl: thirteen collections against one PDS.
	resyncTimeout       = 2 * time.Minute
	listRecordsPageSize = 100
	maxResyncPages      = 100
	maxResyncRecords    = 10_000
)

// Resolver turns a DID into its PDS endpoint; production must pass the SSRF-guarded directory (internal/atidentity), never identity.DefaultDirectory.
type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

// RepoFetcher re-reads a whole repo over listRecords, the only way to reconcile a mirror after a #sync divergence: the stream carries ops, not repo state.
type RepoFetcher struct {
	resolver Resolver
	http     *http.Client
	timeout  time.Duration
}

// NewRepoFetcher builds the production fetcher. httpClient must be the SSRF-safe client: PDS endpoints come from DID documents and are attacker-influenceable.
func NewRepoFetcher(resolver Resolver, httpClient *http.Client) *RepoFetcher {
	return &RepoFetcher{resolver: resolver, http: httpClient, timeout: resyncTimeout}
}

// FetchRepoRecords returns every tracked-collection record in the repo. One collection's failure abandons the whole repo, so a partial read can never be mistaken for a shrunken repo.
func (f *RepoFetcher) FetchRepoRecords(ctx context.Context, did string) ([]MirrorRecord, error) {
	parsed, err := syntax.ParseDID(did)
	if err != nil {
		return nil, fmt.Errorf("discoveringest: resync %s: %w", did, err)
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	ident, err := f.resolver.LookupDID(ctx, parsed)
	if err != nil {
		return nil, fmt.Errorf("discoveringest: resolve %s: %w", did, err)
	}
	endpoint := ident.PDSEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("discoveringest: no PDS endpoint for %s", did)
	}

	client := atxrpc.New(endpoint, f.http)
	var out []MirrorRecord
	for _, collection := range Collections {
		records, err := pageRecords(ctx, client, did, collection, len(out))
		if err != nil {
			return nil, fmt.Errorf("discoveringest: list %s for %s: %w", collection, did, err)
		}
		out = append(out, records...)
	}
	return out, nil
}

type listRecordsResp struct {
	Records []struct {
		URI   string          `json:"uri"`
		CID   string          `json:"cid"`
		Value json.RawMessage `json:"value"`
	} `json:"records"`
	Cursor string `json:"cursor"`
}

// recordLister is the seam pageRecords tests against; *atclient.APIClient satisfies it.
type recordLister interface {
	Get(ctx context.Context, endpoint syntax.NSID, params map[string]any, out any) error
}

var _ recordLister = (*atclient.APIClient)(nil)

func pageRecords(ctx context.Context, client recordLister, repo, collection string, already int) ([]MirrorRecord, error) {
	var (
		out    []MirrorRecord
		cursor string
		pages  int
	)
	seen := make(map[string]struct{})
	for {
		if pages >= maxResyncPages {
			return nil, fmt.Errorf("page limit reached")
		}
		params := map[string]any{"repo": repo, "collection": collection, "limit": listRecordsPageSize}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var resp listRecordsResp
		if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &resp); err != nil {
			return nil, err
		}
		pages++
		if already+len(out)+len(resp.Records) > maxResyncRecords {
			return nil, fmt.Errorf("record limit reached")
		}
		for _, r := range resp.Records {
			rkey := rkeyFromURI(r.URI)
			if rkey == "" {
				continue
			}
			record, err := compactJSON(r.Value)
			if err != nil {
				continue
			}
			out = append(out, MirrorRecord{Collection: collection, Rkey: rkey, CID: r.CID, Record: record})
		}
		if resp.Cursor == "" {
			return out, nil
		}
		// A repeated cursor means the PDS is looping us; stopping beats paging forever.
		if _, repeated := seen[resp.Cursor]; repeated {
			return nil, fmt.Errorf("repeated cursor %q", resp.Cursor)
		}
		seen[resp.Cursor] = struct{}{}
		cursor = resp.Cursor
	}
}

func rkeyFromURI(uri string) string {
	if idx := strings.LastIndex(uri, "/"); idx >= 0 && idx+1 < len(uri) {
		return uri[idx+1:]
	}
	return ""
}

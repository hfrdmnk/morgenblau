package profiles

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// PDSFetcher is the production RecordFetcher. It issues an unauthenticated
// com.atproto.repo.getRecord against the subject's PDS for the
// app.bsky.actor.profile/self record. Client is the safehttp client: the PDS
// endpoint comes from an attacker-suppliable DID, so the fetch must be guarded.
type PDSFetcher struct {
	Client *http.Client
}

// getRecordResp is the trimmed shape of com.atproto.repo.getRecord we care
// about. Value is an opaque map so we can pull displayName/avatar by hand.
type getRecordResp struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

// FetchProfile pulls app.bsky.actor.profile/self from the subject's PDS. A
// "record not found" reply (the common case for users who never edited their
// Bluesky profile) returns (nil, nil, nil) — the cache treats that the same
// as a present-but-empty record.
func (f PDSFetcher) FetchProfile(ctx context.Context, did syntax.DID, pdsEndpoint string) (*string, *string, error) {
	if pdsEndpoint == "" {
		return nil, nil, errors.New("empty PDS endpoint")
	}
	client := atclient.NewAPIClient(pdsEndpoint)
	if f.Client != nil {
		client.Client = f.Client
	}
	var out getRecordResp
	params := map[string]any{
		"repo":       did.String(),
		"collection": "app.bsky.actor.profile",
		"rkey":       "self",
	}
	if err := client.Get(ctx, syntax.NSID("com.atproto.repo.getRecord"), params, &out); err != nil {
		// Treat any error (missing record, malformed PDS, transient) as
		// "no profile data available" — handler downgrades to nulls.
		return nil, nil, err
	}

	var displayName, avatar *string
	if v, ok := out.Value["displayName"].(string); ok && v != "" {
		s := v
		displayName = &s
	}
	if blob, ok := out.Value["avatar"].(map[string]any); ok {
		if ref, ok := blob["ref"].(map[string]any); ok {
			if link, ok := ref["$link"].(string); ok && link != "" {
				u := buildBlobURL(pdsEndpoint, did, link)
				avatar = &u
			}
		}
	}
	return displayName, avatar, nil
}

func buildBlobURL(pdsEndpoint string, did syntax.DID, cid string) string {
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

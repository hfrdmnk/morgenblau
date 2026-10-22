package profiles

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
)

// PDSFetcher is the production RecordFetcher. Client must be the safehttp client: the PDS endpoint comes from an attacker-suppliable DID.
type PDSFetcher struct {
	Client *http.Client
}

// getRecordResp is the trimmed shape of com.atproto.repo.getRecord this package needs.
type getRecordResp struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

// FetchProfile pulls app.bsky.actor.profile/self from the subject's PDS. A "record not found" reply (common for users who never edited their Bluesky profile) returns a zero ProfileRecord, treated as present-but-empty.
func (f PDSFetcher) FetchProfile(ctx context.Context, did syntax.DID, pdsEndpoint string) (ProfileRecord, error) {
	if pdsEndpoint == "" {
		return ProfileRecord{}, errors.New("empty PDS endpoint")
	}
	client := atxrpc.New(pdsEndpoint, f.Client)
	var out getRecordResp
	params := map[string]any{
		"repo":       did.String(),
		"collection": "app.bsky.actor.profile",
		"rkey":       "self",
	}
	if err := client.Get(ctx, syntax.NSID("com.atproto.repo.getRecord"), params, &out); err != nil {
		// Any error here (missing record, malformed PDS, transient) means "no profile data available"; caller downgrades to nulls.
		return ProfileRecord{}, err
	}

	var record ProfileRecord
	if v, ok := out.Value["displayName"].(string); ok && v != "" {
		s := v
		record.DisplayName = &s
	}
	if v, ok := out.Value["description"].(string); ok && v != "" {
		s := v
		record.Description = &s
	}
	if blob, ok := out.Value["avatar"].(map[string]any); ok {
		if ref, ok := blob["ref"].(map[string]any); ok {
			if link, ok := ref["$link"].(string); ok && link != "" {
				u := buildBlobURL(pdsEndpoint, did, link)
				record.Avatar = &u
			}
		}
	}
	return record, nil
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

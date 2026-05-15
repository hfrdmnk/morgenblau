package sync

import (
	"context"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const subscriptionCollection = "app.skyreader.feed.subscription"

// SessionPDSLister calls com.atproto.repo.listRecords against the session's
// PDS, paging through cursors until exhausted.
type SessionPDSLister struct{}

type listRecordsResp struct {
	Records []recordEntry `json:"records"`
	Cursor  string        `json:"cursor"`
}

type recordEntry struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid"`
	Value map[string]any `json:"value"`
}

func (SessionPDSLister) ListSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSSubscription, error) {
	client := sess.APIClient()
	var (
		out    []PDSSubscription
		cursor string
	)
	for {
		var resp listRecordsResp
		params := map[string]any{
			"repo":       sess.Data.AccountDID.String(),
			"collection": subscriptionCollection,
			"limit":      100,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Records {
			out = append(out, toPDSSubscription(r))
		}
		if resp.Cursor == "" || len(resp.Records) == 0 {
			return out, nil
		}
		cursor = resp.Cursor
	}
}

func toPDSSubscription(r recordEntry) PDSSubscription {
	feedURL, _ := r.Value["feedUrl"].(string)
	title, _ := r.Value["title"].(string)
	return PDSSubscription{
		URI:     r.URI,
		Rkey:    rkeyFromATURI(r.URI),
		FeedURL: feedURL,
		Title:   title,
	}
}

func rkeyFromATURI(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

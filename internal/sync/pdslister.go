package sync

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

const subscriptionCollection = "blue.morgen.feed.subscription"

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

// listRecordsClient is the slice of *atclient.APIClient pageSubscriptions uses
// — a tiny seam so tests can swap in an httptest-backed fake.
type listRecordsClient interface {
	Get(ctx context.Context, endpoint syntax.NSID, params map[string]any, out any) error
}

func (SessionPDSLister) ListSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSSubscription, error) {
	return pageSubscriptions(ctx, sess.APIClient(), sess.Data.AccountDID.String())
}

func pageSubscriptions(ctx context.Context, client listRecordsClient, repo string) ([]PDSSubscription, error) {
	var (
		out    []PDSSubscription
		cursor string
	)
	for {
		var resp listRecordsResp
		params := map[string]any{
			"repo":       repo,
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
		// Terminate only on empty cursor — an empty page with a non-empty
		// cursor is still a valid continuation, and stopping early would let
		// reconcile delete every local row not in the partial snapshot.
		if resp.Cursor == "" {
			return out, nil
		}
		cursor = resp.Cursor
	}
}

func toPDSSubscription(r recordEntry) PDSSubscription {
	// TODO(blue.morgen lexicon): once published, validate r.Value against
	// blue.morgen.feed.subscription with lexicon.LenientMode (read-path
	// tolerates unknown fields from older producers). Reject malformed records.
	feedURL, _ := r.Value["feedUrl"].(string)
	title, _ := r.Value["title"].(string)
	return PDSSubscription{
		URI:     r.URI,
		Rkey:    atprepo.RkeyFromATURI(r.URI),
		FeedURL: feedURL,
		Title:   title,
	}
}


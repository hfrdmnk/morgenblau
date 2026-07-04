package sync

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

const (
	subscriptionCollection = "blue.morgen.feed.subscription"
	saveCollection         = "blue.morgen.feed.save"
)

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

func (SessionPDSLister) ListSaves(ctx context.Context, sess *oauth.ClientSession) ([]PDSSave, error) {
	records, err := pageRecords(ctx, sess.APIClient(), sess.Data.AccountDID.String(), saveCollection)
	if err != nil {
		return nil, err
	}
	out := make([]PDSSave, 0, len(records))
	for _, r := range records {
		out = append(out, toPDSSave(r))
	}
	return out, nil
}

func pageSubscriptions(ctx context.Context, client listRecordsClient, repo string) ([]PDSSubscription, error) {
	records, err := pageRecords(ctx, client, repo, subscriptionCollection)
	if err != nil {
		return nil, err
	}
	out := make([]PDSSubscription, 0, len(records))
	for _, r := range records {
		out = append(out, toPDSSubscription(r))
	}
	return out, nil
}

// pageRecords pulls every record in a collection from the repo, following
// cursors until exhausted. Terminate only on empty cursor — an empty page with
// a non-empty cursor is still a valid continuation, and stopping early would
// let reconcile delete every local row not in the partial snapshot.
func pageRecords(ctx context.Context, client listRecordsClient, repo, collection string) ([]recordEntry, error) {
	var (
		out    []recordEntry
		cursor string
	)
	for {
		var resp listRecordsResp
		params := map[string]any{
			"repo":       repo,
			"collection": collection,
			"limit":      100,
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Records...)
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
	primary, _ := r.Value["primary"].(bool)
	return PDSSubscription{
		URI:     r.URI,
		Rkey:    atprepo.RkeyFromATURI(r.URI),
		FeedURL: feedURL,
		Title:   title,
		Primary: primary,
		Tags:    stringSlice(r.Value["tags"]),
	}
}

// stringSlice extracts a []string from a decoded JSON array (which arrives as
// []any), skipping any non-string members. Returns nil when v isn't an array.
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

func toPDSSave(r recordEntry) PDSSave {
	// TODO(blue.morgen lexicon): validate r.Value against blue.morgen.feed.save
	// once published. feedUrl is optional on the record.
	itemURL, _ := r.Value["itemUrl"].(string)
	feedURL, _ := r.Value["feedUrl"].(string)
	createdAt, _ := r.Value["createdAt"].(string)
	return PDSSave{
		URI:       r.URI,
		Rkey:      atprepo.RkeyFromATURI(r.URI),
		ItemURL:   itemURL,
		FeedURL:   feedURL,
		CreatedAt: createdAt,
	}
}


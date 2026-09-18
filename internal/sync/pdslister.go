package sync

import (
	"context"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
)

const (
	subscriptionCollection = lexicon.Subscription
	saveCollection         = lexicon.Save
)

// Standardfeed subscriptions hold portable existence records; blue.morgen subscriptions carry optional metadata sidecars.
const standardSubscriptionCollection = standardfeed.CollectionSubscription

// SessionPDSLister pages com.atproto.repo.listRecords against the session's PDS until the cursor is exhausted.
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

// listRecordsClient is the slice of *atclient.APIClient pageSubscriptions uses, a seam for httptest-backed fakes in tests.
type listRecordsClient interface {
	Get(ctx context.Context, endpoint syntax.NSID, params map[string]any, out any) error
}

func (SessionPDSLister) ListSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSSubscription, error) {
	return pageSubscriptions(ctx, sess.APIClient(), sess.Data.AccountDID.String())
}

func (SessionPDSLister) ListStandardSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSStandardSubscription, error) {
	records, err := pageRecords(ctx, sess.APIClient(), sess.Data.AccountDID.String(), standardSubscriptionCollection)
	if err != nil {
		return nil, err
	}
	out := make([]PDSStandardSubscription, 0, len(records))
	for _, r := range records {
		publication, _ := r.Value["publication"].(string)
		if publication == "" {
			slog.Warn("pdslister: skipping standard subscription without publication", "uri", r.URI)
			continue
		}
		createdAt, _ := r.Value["createdAt"].(string) // optional in the standard lexicon
		out = append(out, PDSStandardSubscription{
			URI:         r.URI,
			Rkey:        atprepo.RkeyFromATURI(r.URI),
			Publication: publication,
			CreatedAt:   createdAt,
		})
	}
	return out, nil
}

func (SessionPDSLister) ListSaves(ctx context.Context, sess *oauth.ClientSession) ([]PDSSave, error) {
	records, err := pageRecords(ctx, sess.APIClient(), sess.Data.AccountDID.String(), saveCollection)
	if err != nil {
		return nil, err
	}
	out := make([]PDSSave, 0, len(records))
	for _, r := range records {
		s, ok := toPDSSave(r)
		if !ok {
			continue
		}
		out = append(out, s)
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
		sub, ok := toPDSSubscription(r)
		if !ok {
			// Unknown source variants are skipped and logged, never treated as deletes of something else.
			slog.Warn("pdslister: skipping subscription with unknown source variant", "uri", r.URI)
			continue
		}
		out = append(out, sub)
	}
	return out, nil
}

// pageRecords terminates only on an empty cursor: an empty page with a cursor
// set is still a valid continuation, and stopping early would let reconcile delete rows missing from a partial snapshot.
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

const (
	sourceTypeRSS      = lexicon.SourceRSS
	sourceTypeStandard = lexicon.SourceStandard
)

// toPDSSubscription dispatches on the required source open union; returns false for v1 flat-shape or unrecognized future variants so callers skip them.
func toPDSSubscription(r recordEntry) (PDSSubscription, bool) {
	if err := lexicon.ValidateRecordLenient(subscriptionCollection, r.Value); err != nil {
		slog.Warn("pdslister: skipping subscription that failed lexicon validation", "uri", r.URI, "err", err)
		return PDSSubscription{}, false
	}
	source, ok := r.Value["source"].(map[string]any)
	if !ok {
		return PDSSubscription{}, false
	}
	title, _ := r.Value["title"].(string)
	primary, _ := r.Value["primary"].(bool)
	sub := PDSSubscription{
		URI:     r.URI,
		Rkey:    atprepo.RkeyFromATURI(r.URI),
		Title:   title,
		Primary: primary,
		Tags:    stringSlice(r.Value["tags"]),
	}
	typ, _ := source["$type"].(string)
	switch typ {
	case sourceTypeRSS:
		feedURL, _ := source["feedUrl"].(string)
		if feedURL == "" {
			return PDSSubscription{}, false
		}
		sub.Kind = "rss"
		sub.FeedURL = feedURL
		sub.SiteURL, _ = source["siteUrl"].(string)
	case sourceTypeStandard:
		publication, _ := source["publication"].(string)
		if publication == "" {
			return PDSSubscription{}, false
		}
		sub.Kind = "standardfeed"
		sub.Publication = publication
	default:
		return PDSSubscription{}, false
	}
	return sub, true
}

// stringSlice extracts []string from a decoded JSON array ([]any), skipping non-string members.
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

// toPDSSave maps a blue.morgen.feed.save record; itemUrl and createdAt are lexicon-required, feedUrl is optional.
func toPDSSave(r recordEntry) (PDSSave, bool) {
	if err := lexicon.ValidateRecordLenient(saveCollection, r.Value); err != nil {
		slog.Warn("pdslister: skipping save that failed lexicon validation", "uri", r.URI, "err", err)
		return PDSSave{}, false
	}
	itemURL, _ := r.Value["itemUrl"].(string)
	feedURL, _ := r.Value["feedUrl"].(string)
	createdAt, _ := r.Value["createdAt"].(string)
	return PDSSave{
		URI:       r.URI,
		Rkey:      atprepo.RkeyFromATURI(r.URI),
		ItemURL:   itemURL,
		FeedURL:   feedURL,
		CreatedAt: createdAt,
	}, true
}

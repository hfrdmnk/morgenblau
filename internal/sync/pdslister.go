package sync

import (
	"context"
	"log/slog"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
)

const (
	subscriptionCollection = "blue.morgen.feed.subscription"
	saveCollection         = "blue.morgen.feed.save"
	shareCollection        = "blue.morgen.feed.share"
)

// Standardfeed collections read from the user's own repo. Existence records
// for publication subscriptions and document shares; blue.morgen records are
// their metadata sidecars.
const (
	standardSubscriptionCollection = "site.standard.graph.subscription"
	standardRecommendCollection    = "site.standard.graph.recommend"
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
		out = append(out, toPDSSave(r))
	}
	return out, nil
}

func (SessionPDSLister) ListShares(ctx context.Context, sess *oauth.ClientSession) ([]PDSShare, error) {
	records, err := pageRecords(ctx, sess.APIClient(), sess.Data.AccountDID.String(), shareCollection)
	if err != nil {
		return nil, err
	}
	out := make([]PDSShare, 0, len(records))
	for _, r := range records {
		s, ok := toPDSShare(r)
		if !ok {
			slog.Warn("pdslister: skipping share without itemUrl", "uri", r.URI)
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (SessionPDSLister) ListRecommends(ctx context.Context, sess *oauth.ClientSession) ([]PDSRecommend, error) {
	records, err := pageRecords(ctx, sess.APIClient(), sess.Data.AccountDID.String(), standardRecommendCollection)
	if err != nil {
		return nil, err
	}
	out := make([]PDSRecommend, 0, len(records))
	for _, r := range records {
		rec, ok := toPDSRecommend(r)
		if !ok {
			slog.Warn("pdslister: skipping recommend without document", "uri", r.URI)
			continue
		}
		out = append(out, rec)
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
			// Readers tolerate unknown source variants: skip and log, never
			// treat them as deletes of something else.
			slog.Warn("pdslister: skipping subscription with unknown source variant", "uri", r.URI)
			continue
		}
		out = append(out, sub)
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

const (
	sourceTypeRSS      = subscriptionCollection + "#rssFeed"
	sourceTypeStandard = subscriptionCollection + "#standardPublication"
)

// toPDSSubscription maps a rev-2 record onto the trimmed shape, dispatching
// on the required `source` open union. Returns false for records without a
// recognizable variant (v1 flat shape, future variants) — callers skip those.
func toPDSSubscription(r recordEntry) (PDSSubscription, bool) {
	// TODO(blue.morgen lexicon): once published, validate r.Value against
	// blue.morgen.feed.subscription with lexicon.LenientMode (read-path
	// tolerates unknown fields from older producers). Reject malformed records.
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

// toPDSShare maps a blue.morgen.feed.share record. itemUrl is required by the
// lexicon — a record without it is malformed, so skip and log. A document
// marks the record as a standardfeed comment sidecar; its absence marks an
// rss share. feedUrl/comment/createdAt are optional.
func toPDSShare(r recordEntry) (PDSShare, bool) {
	itemURL, _ := r.Value["itemUrl"].(string)
	if itemURL == "" {
		return PDSShare{}, false
	}
	document, _ := r.Value["document"].(string)
	feedURL, _ := r.Value["feedUrl"].(string)
	comment, _ := r.Value["comment"].(string)
	createdAt, _ := r.Value["createdAt"].(string)
	return PDSShare{
		URI:       r.URI,
		Rkey:      atprepo.RkeyFromATURI(r.URI),
		ItemURL:   itemURL,
		Document:  document,
		FeedURL:   feedURL,
		Comment:   comment,
		CreatedAt: createdAt,
	}, true
}

// toPDSRecommend maps a site.standard.graph.recommend record — the existence
// authority for a standardfeed share. document is required; without it there's
// nothing to key on, so skip and log. createdAt is optional.
func toPDSRecommend(r recordEntry) (PDSRecommend, bool) {
	document, _ := r.Value["document"].(string)
	if document == "" {
		return PDSRecommend{}, false
	}
	createdAt, _ := r.Value["createdAt"].(string)
	return PDSRecommend{
		URI:       r.URI,
		Rkey:      atprepo.RkeyFromATURI(r.URI),
		Document:  document,
		CreatedAt: createdAt,
	}, true
}

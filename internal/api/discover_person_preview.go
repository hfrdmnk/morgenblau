package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/discoverperson"
	"morgenblau/internal/feedkey"
	"morgenblau/internal/sharemeta"
)

// DiscoverPersonInspector is the person-record aggregation seam; *discoverperson.Inspector satisfies it directly.
type DiscoverPersonInspector interface {
	Records(ctx context.Context, did string, viewerKeys map[string]struct{}) discoverperson.Records
	Preview(r discoverperson.Records) discoverperson.Preview
}

// DiscoverPersonSourceWire is one write/read source in a person card or profile page, same kind-based split as DiscoverSourceWire.
type DiscoverPersonSourceWire struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Title       string `json:"title,omitempty"`
	SiteURL     string `json:"siteUrl,omitempty"`
	FeedURL     string `json:"feedUrl,omitempty"`
	Publication string `json:"publication,omitempty"`
	Subscribed  bool   `json:"subscribed"`
}

// DiscoverPersonShareWire is one share item in a person card or profile page.
type DiscoverPersonShareWire struct {
	ItemURL   string `json:"itemUrl,omitempty"`
	Document  string `json:"document,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	Title     string `json:"title,omitempty"`
	TargetURL string `json:"targetUrl,omitempty"`
	EntrySlug string `json:"entrySlug,omitempty"`
}

// DiscoverPersonPreviewWire is the card-sized slice of a person's records. LatestShare is an explicit null when absent, not omitted, so the frontend never has to distinguish "missing field" from "no share".
type DiscoverPersonPreviewWire struct {
	Writes      []DiscoverPersonSourceWire `json:"writes"`
	WritesTotal int                        `json:"writesTotal"`
	Reads       []DiscoverPersonSourceWire `json:"reads"`
	ReadsTotal  int                        `json:"readsTotal"`
	LatestShare *DiscoverPersonShareWire   `json:"latestShare"`
}

// DiscoverPersonPreviewHandler serves a person card's lazy-loaded preview: writes/reads capped by Inspector.Preview, latest share only. A crawl failure or empty repo degrades to empty sections, never an error status, same posture as DiscoverSourcePostsHandler. SPEC <discovery>.
func DiscoverPersonPreviewHandler(inspector DiscoverPersonInspector, subs DiscoverSubscriptionsReader, metadata ShareMetadataResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}

		did := r.URL.Query().Get("did")
		if _, err := syntax.ParseDID(did); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "did is required")
			return
		}

		viewerKeys, ok := loadViewerKeys(w, r, subs, sess.Data.AccountDID.String())
		if !ok {
			return
		}

		records := inspector.Records(r.Context(), did, viewerKeys)
		preview := inspector.Preview(records)

		writeJSON(w, DiscoverPersonPreviewWire{
			Writes:      discoverPersonSourceWires(preview.Writes),
			WritesTotal: len(records.Writes),
			Reads:       discoverPersonSourceWires(preview.Reads),
			ReadsTotal:  len(records.Reads),
			LatestShare: discoverPersonShareWire(r.Context(), metadata, preview.LatestShare),
		})
	})
}

// loadViewerKeys reads the session user's subscriptions and normalizes them into the key set Inspector.Records marks inert against. A read failure writes a 500 and returns ok=false.
func loadViewerKeys(w http.ResponseWriter, r *http.Request, subs DiscoverSubscriptionsReader, viewerDID string) (map[string]struct{}, bool) {
	subRows, err := subs.ListUserSubscriptions(r.Context(), viewerDID)
	if err != nil {
		slog.Warn("loadViewerKeys: list subscriptions failed", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return nil, false
	}
	viewerKeys := make(map[string]struct{}, len(subRows))
	for _, s := range subRows {
		viewerKeys[feedkey.Normalize(s.FeedUrl)] = struct{}{}
	}
	return viewerKeys, true
}

func discoverPersonSourceWire(item discoverperson.SourceItem) DiscoverPersonSourceWire {
	wire := DiscoverPersonSourceWire{
		Key:        item.Key,
		Kind:       item.Kind,
		Title:      item.Title,
		SiteURL:    item.SiteURL,
		Subscribed: item.Subscribed,
	}
	if item.Kind == "standardfeed" {
		wire.Publication = item.Key
	} else {
		wire.FeedURL = item.Key
	}
	return wire
}

func discoverPersonSourceWires(items []discoverperson.SourceItem) []DiscoverPersonSourceWire {
	out := make([]DiscoverPersonSourceWire, 0, len(items))
	for _, item := range items {
		out = append(out, discoverPersonSourceWire(item))
	}
	return out
}

func discoverPersonShareWireFromItem(s discoverperson.ShareItem, metadata sharemeta.Metadata) DiscoverPersonShareWire {
	return DiscoverPersonShareWire{
		ItemURL:   s.ItemURL,
		Document:  s.Document,
		Comment:   s.Comment,
		CreatedAt: formatDiscoverPersonTime(s.CreatedAt),
		Title:     metadata.Title,
		TargetURL: metadata.TargetURL,
		EntrySlug: metadata.EntrySlug,
	}
}

func discoverPersonShareWire(ctx context.Context, resolver ShareMetadataResolver, s *discoverperson.ShareItem) *DiscoverPersonShareWire {
	if s == nil {
		return nil
	}
	metadata := resolveShareMetadata(ctx, resolver, []sharemeta.Target{{
		ItemURL: s.ItemURL, Document: s.Document,
	}})[0]
	wire := discoverPersonShareWireFromItem(*s, metadata)
	return &wire
}

func discoverPersonShareWires(ctx context.Context, resolver ShareMetadataResolver, items []discoverperson.ShareItem) []DiscoverPersonShareWire {
	targets := make([]sharemeta.Target, len(items))
	for i, item := range items {
		targets[i] = sharemeta.Target{ItemURL: item.ItemURL, Document: item.Document}
	}
	metadata := resolveShareMetadata(ctx, resolver, targets)
	out := make([]DiscoverPersonShareWire, 0, len(items))
	for i, item := range items {
		out = append(out, discoverPersonShareWireFromItem(item, metadata[i]))
	}
	return out
}

// formatDiscoverPersonTime formats a share's recency, blank when unknown rather than the zero-time sentinel value.
func formatDiscoverPersonTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

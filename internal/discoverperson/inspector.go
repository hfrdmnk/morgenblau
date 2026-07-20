// Package discoverperson inspects one person's reader-network activity — the writes,
// reads, and shares behind a person card and the profile page. It hides the multi-lexicon
// crawl aggregation, marks already-subscribed sources inert against the viewer's keys, and
// never surfaces saves (save privacy, SPEC <saving-sharing>).
//
// The underlying crawls are TTL-cached and daily-stable (SPEC <discovery>): callers may
// treat a person's records as unchanged within a day.
package discoverperson

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/feedkey"
)

// SubscriptionFetcher is the subscription crawl seam; *discovercrawl.CachedCrawler satisfies it directly.
type SubscriptionFetcher interface {
	FetchSubscriptions(ctx context.Context, did syntax.DID) ([]discovercrawl.Subscription, error)
}

// AuthoredFetcher is the authored-publication crawl seam; *discovercrawl.CachedAuthoredCrawler satisfies it directly.
type AuthoredFetcher interface {
	FetchAuthoredPublications(ctx context.Context, did syntax.DID) ([]discovercrawl.AuthoredPublication, error)
}

// ShareFetcher is the share crawl seam; *discovercrawl.CachedShareCrawler satisfies it directly.
type ShareFetcher interface {
	FetchShares(ctx context.Context, did syntax.DID) ([]discovercrawl.Share, error)
}

// SourceItem is one source a person writes or reads. Subscribed marks a source the viewer
// already has (de-dup rule, SPEC <discovery>): inert, never hidden.
type SourceItem struct {
	Key        string
	Kind       string
	Title      string
	SiteURL    string
	Subscribed bool
}

// ShareItem is one item a person shared.
type ShareItem struct {
	ItemURL   string
	Document  string
	Kind      string
	Comment   string
	CreatedAt time.Time
}

// Records is a person's full inspected activity, newest first within each section.
type Records struct {
	Writes []SourceItem
	Reads  []SourceItem
	Shares []ShareItem
}

// Preview is the card-sized slice of Records: SPEC <discovery> caps of 2 writes, 4 reads, latest share.
type Preview struct {
	Writes      []SourceItem
	Reads       []SourceItem
	LatestShare *ShareItem
}

const (
	previewWrites = 2
	previewReads  = 4
)

// Inspector answers DID → {writes, reads, shares} by aggregating the cached crawls.
type Inspector struct {
	subs     SubscriptionFetcher
	authored AuthoredFetcher
	shares   ShareFetcher
}

// New wires an Inspector over the three crawl seams.
func New(subs SubscriptionFetcher, authored AuthoredFetcher, shares ShareFetcher) *Inspector {
	return &Inspector{subs: subs, authored: authored, shares: shares}
}

// Records collects a person's writes, reads, and shares. viewerKeys are the viewer's canonical
// subscription keys (already feedkey-normalized, matching candidate keys) used to mark inert
// sources. A malformed DID or any single crawl failure degrades that section to empty; Records
// never returns an error (SPEC <discovery>: zero records is an empty result, not an error).
func (in *Inspector) Records(ctx context.Context, did string, viewerKeys map[string]struct{}) Records {
	personDID, err := syntax.ParseDID(did)
	if err != nil {
		slog.Warn("discoverperson: malformed did", "did", did, "err", err)
		return Records{}
	}
	return Records{
		Writes: in.writes(ctx, personDID, viewerKeys),
		Reads:  in.reads(ctx, personDID, viewerKeys),
		Shares: in.shareItems(ctx, personDID),
	}
}

// Preview caps Records to card size: 2 writes, 4 reads, the single latest share.
func (in *Inspector) Preview(r Records) Preview {
	p := Preview{
		Writes: capSources(r.Writes, previewWrites),
		Reads:  capSources(r.Reads, previewReads),
	}
	if len(r.Shares) > 0 {
		latest := r.Shares[0]
		p.LatestShare = &latest
	}
	return p
}

func (in *Inspector) writes(ctx context.Context, did syntax.DID, viewerKeys map[string]struct{}) []SourceItem {
	pubs, err := in.authored.FetchAuthoredPublications(ctx, did)
	if err != nil {
		slog.Warn("discoverperson: authored-publication crawl failed", "did", did, "err", err)
		return nil
	}
	rows := make([]sourceRow, 0, len(pubs))
	for _, p := range pubs {
		rows = append(rows, sourceRow{
			item: SourceItem{
				Key:        p.Key,
				Kind:       kindOf(p.Kind, p.Key),
				Title:      p.Title,
				SiteURL:    p.SiteURL,
				Subscribed: contains(viewerKeys, p.Key),
			},
			at: parseTime(p.LastPublishedAt),
		})
	}
	return dedupSources(rows)
}

func (in *Inspector) reads(ctx context.Context, did syntax.DID, viewerKeys map[string]struct{}) []SourceItem {
	subs, err := in.subs.FetchSubscriptions(ctx, did)
	if err != nil {
		slog.Warn("discoverperson: subscription crawl failed", "did", did, "err", err)
		return nil
	}
	rows := make([]sourceRow, 0, len(subs))
	for _, s := range subs {
		rows = append(rows, sourceRow{
			item: SourceItem{
				Key:        s.Key,
				Kind:       kindOf(s.Kind, s.Key),
				Title:      s.Title,
				SiteURL:    s.SiteURL,
				Subscribed: contains(viewerKeys, s.Key),
			},
			at: parseTime(s.CreatedAt),
		})
	}
	return dedupSources(rows)
}

func (in *Inspector) shareItems(ctx context.Context, did syntax.DID) []ShareItem {
	shares, err := in.shares.FetchShares(ctx, did)
	if err != nil {
		slog.Warn("discoverperson: share crawl failed", "did", did, "err", err)
		return nil
	}
	items := make([]ShareItem, 0, len(shares))
	for _, s := range shares {
		items = append(items, ShareItem{
			ItemURL:   s.ItemURL,
			Document:  s.Document,
			Kind:      s.Kind,
			Comment:   s.Comment,
			CreatedAt: parseTime(s.CreatedAt),
		})
	}
	return dedupShares(items)
}

// sourceRow carries a source's recency alongside the item so dedup and sort see it without exposing it.
type sourceRow struct {
	item SourceItem
	at   time.Time
}

// dedupSources collapses rows by canonical key (keeping the newest), then orders newest first.
func dedupSources(rows []sourceRow) []SourceItem {
	byKey := make(map[string]int, len(rows))
	deduped := make([]sourceRow, 0, len(rows))
	for _, r := range rows {
		if i, ok := byKey[r.item.Key]; ok {
			if r.at.After(deduped[i].at) {
				deduped[i] = r
			}
			continue
		}
		byKey[r.item.Key] = len(deduped)
		deduped = append(deduped, r)
	}
	sort.SliceStable(deduped, func(i, j int) bool { return deduped[i].at.After(deduped[j].at) })
	out := make([]SourceItem, len(deduped))
	for i, r := range deduped {
		out[i] = r.item
	}
	return out
}

// dedupShares collapses shares by itemURL (keeping the newest), then orders newest first.
// Empty itemURLs are never merged: standardfeed shares key on document upstream and would otherwise collide.
func dedupShares(items []ShareItem) []ShareItem {
	byURL := make(map[string]int, len(items))
	out := make([]ShareItem, 0, len(items))
	for _, s := range items {
		if s.ItemURL != "" {
			if i, ok := byURL[s.ItemURL]; ok {
				if s.CreatedAt.After(out[i].CreatedAt) {
					out[i] = s
				}
				continue
			}
			byURL[s.ItemURL] = len(out)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func capSources(items []SourceItem, n int) []SourceItem {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func contains(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

func kindOf(kind, key string) string {
	if kind != "" {
		return kind
	}
	return feedkey.Kind(key)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

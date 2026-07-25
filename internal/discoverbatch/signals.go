package discoverbatch

import (
	"context"
	"time"

	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverrank"
	"morgenblau/internal/feedkey"
)

// EntryResolver resolves a share/save reaction to its canonical source key via Tier-2 provenance. SPEC <discovery>.
type EntryResolver interface {
	GetFeedURLByGuid(ctx context.Context, guid string) (string, error)
	GetFeedURLByItemURL(ctx context.Context, url string) (string, error)
}

// RepoSource is one repo's strongest signal for a canonical source key. SPEC <discovery>: one signal per source, strongest wins.
type RepoSource struct {
	Kind    string
	Title   string
	SiteURL string
	Signal  discoverrank.Signal
}

// ReduceRepoSignals folds one repo's crawl results into its strongest signal per source key; unresolvable reactions drop silently.
func ReduceRepoSignals(
	ctx context.Context,
	subs []discovercrawl.Subscription,
	pubs []discovercrawl.AuthoredPublication,
	shares []discovercrawl.Share,
	saves []discovercrawl.Save,
	entries EntryResolver,
) map[string]RepoSource {
	out := map[string]RepoSource{}
	upsert := func(key, kind, title, siteURL string, signal discoverrank.Signal) {
		if key == "" {
			return
		}
		cur, ok := out[key]
		if !ok {
			out[key] = RepoSource{Kind: kind, Title: title, SiteURL: siteURL, Signal: signal}
			return
		}
		if cur.Kind == "" {
			cur.Kind = kind
		}
		if cur.Title == "" {
			cur.Title = title
		}
		if cur.SiteURL == "" {
			cur.SiteURL = siteURL
		}
		if discoverrank.StrongerSignal(signal, cur.Signal) {
			cur.Signal = signal
		}
		out[key] = cur
	}

	for _, s := range subs {
		upsert(s.Key, feedkey.Kind(s.Key), s.Title, s.SiteURL, discoverrank.Signal{Kind: discoverrank.SignalSubscribe, At: parseTime(s.CreatedAt)})
	}
	for _, p := range pubs {
		upsert(p.Key, feedkey.Kind(p.Key), p.Title, p.SiteURL, discoverrank.Signal{Kind: discoverrank.SignalAuthor, At: parseTime(p.LastPublishedAt)})
	}
	for _, sh := range shares {
		key, ok := resolveReactionKey(ctx, entries, sh.FeedURL, sh.Document, sh.ItemURL)
		if !ok {
			continue
		}
		upsert(key, kindForKey(key), "", "", discoverrank.Signal{Kind: discoverrank.SignalShare, At: parseTime(sh.CreatedAt)})
	}
	for _, sv := range saves {
		key, ok := resolveReactionKey(ctx, entries, sv.FeedURL, "", sv.ItemURL)
		if !ok {
			continue
		}
		upsert(key, kindForKey(key), "", "", discoverrank.Signal{Kind: discoverrank.SignalSave, At: parseTime(sv.CreatedAt)})
	}
	return out
}

// resolveReactionKey mirrors internal/api's version; feedkey.Normalize runs on every branch since Tier-2 lookups return the feed_url unnormalized.
func resolveReactionKey(ctx context.Context, resolver EntryResolver, feedURL, document, itemURL string) (string, bool) {
	if feedURL != "" {
		return feedkey.Normalize(feedURL), true
	}
	if document != "" {
		if fu, err := resolver.GetFeedURLByGuid(ctx, document); err == nil && fu != "" {
			return feedkey.Normalize(fu), true
		}
	}
	if itemURL != "" {
		if fu, err := resolver.GetFeedURLByItemURL(ctx, itemURL); err == nil && fu != "" {
			return feedkey.Normalize(fu), true
		}
	}
	return "", false
}

// kindForKey infers a Tier-2 kind from the canonical key shape.
func kindForKey(key string) string {
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

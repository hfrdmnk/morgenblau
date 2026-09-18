package discoveringest

import (
	"context"
	"time"

	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverrank"
	"morgenblau/internal/feedkey"
)

// EntryResolver resolves a share or save reaction to its canonical source key via Tier-2 provenance. SPEC <discovery>.
type EntryResolver interface {
	GetFeedURLByGuid(ctx context.Context, guid string) (string, error)
	GetFeedURLByItemURL(ctx context.Context, url string) (string, error)
}

// repoSource is one repo's strongest signal for a canonical source key. SPEC <discovery>: one signal per source, strongest wins.
type repoSource struct {
	Kind    string
	Title   string
	SiteURL string
	Signal  discoverrank.Signal
}

// reduceRepoSignals folds one repo's decoded records into its strongest signal per source key; unresolvable reactions drop silently.
// Every timestamp comes from the record's own createdAt: the stream's witness time is when a crawler saw the record, never when its author made it. SPEC <discovery>.
func reduceRepoSignals(
	ctx context.Context,
	subs []discovercrawl.Subscription,
	pubs []discovercrawl.AuthoredPublication,
	shares []discovercrawl.Share,
	saves []discovercrawl.Save,
	entries EntryResolver,
) map[string]repoSource {
	out := map[string]repoSource{}
	upsert := func(key, kind, title, siteURL string, signal discoverrank.Signal) {
		if key == "" {
			return
		}
		cur, ok := out[key]
		if !ok {
			out[key] = repoSource{Kind: kind, Title: title, SiteURL: siteURL, Signal: signal}
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
		key, ok := feedkey.ResolveReactionKey(ctx, entries, sh.FeedURL, sh.Document, sh.ItemURL)
		if !ok {
			continue
		}
		upsert(key, feedkey.Kind(key), "", "", discoverrank.Signal{Kind: discoverrank.SignalShare, At: parseTime(sh.CreatedAt)})
	}
	for _, sv := range saves {
		key, ok := feedkey.ResolveReactionKey(ctx, entries, sv.FeedURL, "", sv.ItemURL)
		if !ok {
			continue
		}
		upsert(key, feedkey.Kind(key), "", "", discoverrank.Signal{Kind: discoverrank.SignalSave, At: parseTime(sv.CreatedAt)})
	}
	return out
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

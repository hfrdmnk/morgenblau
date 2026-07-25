package discoverbatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverrank"
)

type fakeEntryResolver struct {
	byGuid    map[string]string
	byItemURL map[string]string
}

func (f *fakeEntryResolver) GetFeedURLByGuid(_ context.Context, guid string) (string, error) {
	if v, ok := f.byGuid[guid]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func (f *fakeEntryResolver) GetFeedURLByItemURL(_ context.Context, url string) (string, error) {
	if v, ok := f.byItemURL[url]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func TestReduceRepoSignals_OneRowPerSourceKind(t *testing.T) {
	subs := []discovercrawl.Subscription{
		{Key: "https://a.example/feed", Kind: "rss", Title: "A", SiteURL: "https://a.example", CreatedAt: "2026-07-01T00:00:00Z"},
	}
	got := ReduceRepoSignals(context.Background(), subs, nil, nil, nil, &fakeEntryResolver{})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	row, ok := got["https://a.example/feed"]
	if !ok {
		t.Fatalf("missing key, got %+v", got)
	}
	if row.Kind != "rss" || row.Title != "A" || row.SiteURL != "https://a.example" {
		t.Errorf("row = %+v", row)
	}
	if row.Signal.Kind != discoverrank.SignalSubscribe {
		t.Errorf("Signal.Kind = %v, want SignalSubscribe", row.Signal.Kind)
	}
}

func TestReduceRepoSignals_StrongestSignalWinsAcrossKinds(t *testing.T) {
	subs := []discovercrawl.Subscription{
		{Key: "https://a.example/feed", Kind: "rss", CreatedAt: "2026-01-01T00:00:00Z"},
	}
	shares := []discovercrawl.Share{
		{FeedURL: "https://a.example/feed", ItemURL: "https://a.example/post-1", CreatedAt: "2026-07-01T00:00:00Z"},
	}
	got := ReduceRepoSignals(context.Background(), subs, nil, shares, nil, &fakeEntryResolver{})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (subscribe and share resolve to the same source)", len(got))
	}
	if got["https://a.example/feed"].Signal.Kind != discoverrank.SignalSubscribe {
		t.Errorf("Signal.Kind = %v, want SignalSubscribe (subscribe beats share regardless of order)", got["https://a.example/feed"].Signal.Kind)
	}
}

func TestReduceRepoSignals_AuthoredPublicationSignal(t *testing.T) {
	pubs := []discovercrawl.AuthoredPublication{
		{Key: "at://did:plc:author/site.standard.publication/abc", Kind: "standardfeed", Title: "Zine", SiteURL: "https://zine.example", LastPublishedAt: "2026-07-01T00:00:00Z"},
	}
	got := ReduceRepoSignals(context.Background(), nil, pubs, nil, nil, &fakeEntryResolver{})

	row, ok := got["at://did:plc:author/site.standard.publication/abc"]
	if !ok {
		t.Fatalf("missing authored key, got %+v", got)
	}
	if row.Signal.Kind != discoverrank.SignalAuthor {
		t.Errorf("Signal.Kind = %v, want SignalAuthor", row.Signal.Kind)
	}
}

func TestReduceRepoSignals_ShareResolvesViaDocumentProvenance(t *testing.T) {
	shares := []discovercrawl.Share{
		{Kind: "standardfeed", Document: "at://did:plc:pub/site.standard.document/1", CreatedAt: "2026-07-01T00:00:00Z"},
	}
	resolver := &fakeEntryResolver{byGuid: map[string]string{
		"at://did:plc:pub/site.standard.document/1": "at://did:plc:pub/site.standard.publication/pub",
	}}
	got := ReduceRepoSignals(context.Background(), nil, nil, shares, nil, resolver)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	row, ok := got["at://did:plc:pub/site.standard.publication/pub"]
	if !ok {
		t.Fatalf("missing resolved key, got %+v", got)
	}
	if row.Kind != "standardfeed" {
		t.Errorf("Kind = %q, want standardfeed (at:// convention)", row.Kind)
	}
}

func TestReduceRepoSignals_SaveResolvesViaItemURLFallback(t *testing.T) {
	saves := []discovercrawl.Save{
		{Kind: "morgen", ItemURL: "https://a.example/post-1", CreatedAt: "2026-07-01T00:00:00Z"},
	}
	resolver := &fakeEntryResolver{byItemURL: map[string]string{"https://a.example/post-1": "https://a.example/feed"}}
	got := ReduceRepoSignals(context.Background(), nil, nil, nil, saves, resolver)

	row, ok := got["https://a.example/feed"]
	if !ok {
		t.Fatalf("missing resolved key, got %+v", got)
	}
	if row.Signal.Kind != discoverrank.SignalSave {
		t.Errorf("Signal.Kind = %v, want SignalSave", row.Signal.Kind)
	}
}

func TestReduceRepoSignals_UnresolvableReactionDropsSilently(t *testing.T) {
	shares := []discovercrawl.Share{{ItemURL: "https://unresolvable.example/post"}}
	got := ReduceRepoSignals(context.Background(), nil, nil, shares, nil, &fakeEntryResolver{})
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty (unresolvable reaction drops silently)", got)
	}
}

// TestReduceRepoSignals_TwoReposVariantKeysAggregateUnderOneSourceKey proves variant feed URLs across repos still reduce to the identical map key.
func TestReduceRepoSignals_TwoReposVariantKeysAggregateUnderOneSourceKey(t *testing.T) {
	repoASubs := []discovercrawl.Subscription{
		{Key: "https://a.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"},
	}
	repoBShares := []discovercrawl.Share{
		{FeedURL: "https://a.example:443/feed/", ItemURL: "https://a.example/post-1", CreatedAt: "2026-07-02T00:00:00Z"},
	}

	repoA := ReduceRepoSignals(context.Background(), repoASubs, nil, nil, nil, &fakeEntryResolver{})
	repoB := ReduceRepoSignals(context.Background(), nil, nil, repoBShares, nil, &fakeEntryResolver{})

	if len(repoA) != 1 || len(repoB) != 1 {
		t.Fatalf("repoA = %+v, repoB = %+v, want exactly one source key each", repoA, repoB)
	}
	var keyA, keyB string
	for k := range repoA {
		keyA = k
	}
	for k := range repoB {
		keyB = k
	}
	if keyA != keyB {
		t.Errorf("keyA = %q, keyB = %q, want identical source_key so the two repos' rows aggregate", keyA, keyB)
	}
	if keyA != "https://a.example/feed" {
		t.Errorf("keyA = %q, want canonical https://a.example/feed", keyA)
	}
}

func TestReduceRepoSignals_UnknownTimestampParsesToZeroTime(t *testing.T) {
	subs := []discovercrawl.Subscription{{Key: "https://a.example/feed", Kind: "rss", CreatedAt: "not-a-time"}}
	got := ReduceRepoSignals(context.Background(), subs, nil, nil, nil, &fakeEntryResolver{})
	if !got["https://a.example/feed"].Signal.At.Equal(time.Time{}) {
		t.Errorf("At = %v, want zero time for malformed timestamp", got["https://a.example/feed"].Signal.At)
	}
}

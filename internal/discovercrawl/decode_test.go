package discovercrawl

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atclient"

	"morgenblau/internal/feedkey"
	"morgenblau/internal/leafletfeed"
	"morgenblau/internal/standardfeed"
)

// fakePublicationResolver is mutex-guarded so it doubles as the standard resolver in the singleflight test.
type fakePublicationResolver struct {
	mu       sync.Mutex
	calls    int
	seenURIs []string
	byURI    map[string]*standardfeed.Publication
	errByURI map[string]error
}

func (f *fakePublicationResolver) GetPublication(_ context.Context, uri string) (*standardfeed.Publication, error) {
	f.mu.Lock()
	f.calls++
	f.seenURIs = append(f.seenURIs, uri)
	f.mu.Unlock()
	if err, ok := f.errByURI[uri]; ok {
		return nil, err
	}
	if pub, ok := f.byURI[uri]; ok {
		return pub, nil
	}
	return nil, fmt.Errorf("unknown publication %s", uri)
}

type fakeLeafletResolver struct {
	mu       sync.Mutex
	calls    int
	seenURIs []string
	byURI    map[string]*leafletfeed.Publication
}

func (f *fakeLeafletResolver) GetPublication(_ context.Context, uri string) (*leafletfeed.Publication, error) {
	f.mu.Lock()
	f.calls++
	f.seenURIs = append(f.seenURIs, uri)
	f.mu.Unlock()
	if pub, ok := f.byURI[uri]; ok {
		return pub, nil
	}
	return nil, fmt.Errorf("unknown publication %s", uri)
}

func (f *fakeLeafletResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestDecodeFeedURLRecord_TableDriven(t *testing.T) {
	cases := []struct {
		name    string
		value   map[string]any
		wantOK  bool
		wantSub Subscription
	}{
		{
			name: "full record",
			value: map[string]any{
				"feedUrl":   "https://example.com/feed",
				"title":     "Example",
				"siteUrl":   "https://example.com",
				"createdAt": "2026-06-01T00:00:00Z",
			},
			wantOK:  true,
			wantSub: Subscription{Key: "https://example.com/feed", Kind: "rss", Title: "Example", SiteURL: "https://example.com", CreatedAt: "2026-06-01T00:00:00Z"},
		},
		{
			name:    "missing feedUrl skipped",
			value:   map[string]any{"title": "No URL"},
			wantOK:  false,
			wantSub: Subscription{},
		},
		{
			name: "optionals absent",
			value: map[string]any{
				"feedUrl": "https://bare.example/feed",
			},
			wantOK:  true,
			wantSub: Subscription{Key: "https://bare.example/feed", Kind: "rss"},
		},
	}
	c := &Client{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := c.decodeFeedURLRecord(context.Background(), "app.skyreader.feed.subscription", recordEntry{URI: "at://x/c/r", Value: tc.value}, map[string]Subscription{})
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.wantSub {
				t.Errorf("got = %+v, want %+v", got, tc.wantSub)
			}
		})
	}
}

func TestDecodeFeedURLRecord_AtURIFeedURLResolvesAsPublication(t *testing.T) {
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	resolver := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: resolver}

	got, ok := c.decodeFeedURLRecord(context.Background(), "app.skyreader.feed.subscription", recordEntry{
		URI: "at://x/c/r",
		Value: map[string]any{
			"feedUrl":   pubURI,
			"createdAt": "2026-06-01T00:00:00Z",
		},
	}, map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: pubURI, Kind: "standardfeed", Title: "Zine", SiteURL: "https://zine.example", CreatedAt: "2026-06-01T00:00:00Z"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

func TestDecodeMorgen_RSSVariant(t *testing.T) {
	c := &Client{}
	got, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{
				"$type":   "blue.morgen.feed.subscription#rssFeed",
				"feedUrl": "https://example.com/feed",
				"siteUrl": "https://example.com",
			},
			"title":     "Example",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	}, map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: "https://example.com/feed", Kind: "rss", Title: "Example", SiteURL: "https://example.com", CreatedAt: "2026-06-01T00:00:00Z"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

func TestDecodeMorgen_RSSVariantAtURIFeedURLResolvesAsPublication(t *testing.T) {
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	resolver := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: resolver}

	got, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{
				"$type":   "blue.morgen.feed.subscription#rssFeed",
				"feedUrl": pubURI,
			},
			"title":     "Reader Title",
			"createdAt": "2026-06-01T00:00:00Z",
		},
	}, map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: pubURI, Kind: "standardfeed", Title: "Reader Title", SiteURL: "https://zine.example", CreatedAt: "2026-06-01T00:00:00Z"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

func TestDecodeMorgen_RSSVariantMissingFeedURLSkipped(t *testing.T) {
	c := &Client{}
	_, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{"$type": "blue.morgen.feed.subscription#rssFeed"},
		},
	}, map[string]Subscription{})
	if ok {
		t.Error("expected skip: rssFeed variant missing feedUrl")
	}
}

func TestDecodeMorgen_UnknownVariantSkipped(t *testing.T) {
	c := &Client{}
	_, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{"$type": "blue.morgen.feed.subscription#carrierPigeon"},
		},
	}, map[string]Subscription{})
	if ok {
		t.Error("expected skip: unknown source union variant")
	}
}

func TestDecodeMorgen_SourceNotMapSkipped(t *testing.T) {
	c := &Client{}
	_, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI:   "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{"source": "https://example.com/feed"},
	}, map[string]Subscription{})
	if ok {
		t.Error("expected skip: source not a map (v1 flat shape)")
	}
}

func TestDecodeMorgen_StandardVariantDelegatesToPublicationResolver(t *testing.T) {
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	resolver := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: resolver}

	got, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{
				"$type":       "blue.morgen.feed.subscription#standardPublication",
				"publication": pubURI,
			},
			"createdAt": "2026-06-01T00:00:00Z",
		},
	}, map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: pubURI, Kind: "standardfeed", Title: "Zine", SiteURL: "https://zine.example", CreatedAt: "2026-06-01T00:00:00Z"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1", resolver.calls)
	}
}

func TestDecodeMorgen_StandardVariantMissingPublicationSkipped(t *testing.T) {
	c := &Client{}
	_, ok := c.decodeMorgen(context.Background(), recordEntry{
		URI: "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		Value: map[string]any{
			"source": map[string]any{"$type": "blue.morgen.feed.subscription#standardPublication"},
		},
	}, map[string]Subscription{})
	if ok {
		t.Error("expected skip: standardPublication variant missing publication")
	}
}

func TestResolvePublication_TitleFallsBackToPublicationName(t *testing.T) {
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	resolver := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine Name", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: resolver}

	got, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{})
	if !ok || got.Title != "Zine Name" {
		t.Errorf("got = %+v, ok = %v, want title fallback to publication name", got, ok)
	}
}

func TestResolvePublication_MemoizesAcrossCallsInOneCrawl(t *testing.T) {
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	resolver := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: resolver}
	cache := map[string]Subscription{}

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", cache); !ok {
		t.Fatal("first call: expected ok")
	}
	if _, ok := c.resolvePublication(context.Background(), pubURI, "", cache); !ok {
		t.Fatal("second call: expected ok")
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (memoized)", resolver.calls)
	}
}

func TestResolvePublication_FailureIsSkippedAndMemoized(t *testing.T) {
	resolver := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{}}
	c := &Client{standard: resolver}
	cache := map[string]Subscription{}

	if _, ok := c.resolvePublication(context.Background(), "at://did:plc:missing/site.standard.publication/3p", "", cache); ok {
		t.Fatal("expected skip on resolve failure")
	}
	if _, ok := c.resolvePublication(context.Background(), "at://did:plc:missing/site.standard.publication/3p", "", cache); ok {
		t.Fatal("expected skip on repeated call")
	}
	if resolver.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (failure memoized too)", resolver.calls)
	}
}

func TestResolvePublication_NilStandardResolverSkips(t *testing.T) {
	c := &Client{}
	if _, ok := c.resolvePublication(context.Background(), "at://did:plc:pub/site.standard.publication/3p", "", map[string]Subscription{}); ok {
		t.Error("expected skip when no PublicationResolver is configured")
	}
}

// leafletPubURI and its site.standard.publication sibling share authority and rkey; only the collection segment differs (SPEC <lexicons> pub.leaflet.publication).
const (
	leafletPubURI        = "at://did:plc:pub/pub.leaflet.publication/3p"
	leafletPubSiblingURI = "at://did:plc:pub/site.standard.publication/3p"
)

func recordNotFoundErr() error {
	return &atclient.APIError{StatusCode: 400, Name: "RecordNotFound", Message: "could not locate record"}
}

func TestResolvePublication_LeafletSiblingExistsPrefersStandard(t *testing.T) {
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		leafletPubSiblingURI: {URI: leafletPubSiblingURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	leaflet := &fakeLeafletResolver{}
	c := &Client{standard: standard, leaflet: leaflet}

	got, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: leafletPubSiblingURI, Kind: "standardfeed", Title: "Zine", SiteURL: "https://zine.example"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
	if leaflet.callCount() != 0 {
		t.Errorf("leaflet resolver calls = %d, want 0", leaflet.callCount())
	}
	if standard.calls != 1 || len(standard.seenURIs) != 1 || standard.seenURIs[0] != leafletPubSiblingURI {
		t.Errorf("standard resolver seenURIs = %+v, want exactly [%s] (swapped collection, same rkey)", standard.seenURIs, leafletPubSiblingURI)
	}
}

func TestResolvePublication_LeafletSiblingNotFoundFallsBackToRSS(t *testing.T) {
	standard := &fakePublicationResolver{errByURI: map[string]error{leafletPubSiblingURI: recordNotFoundErr()}}
	leaflet := &fakeLeafletResolver{byURI: map[string]*leafletfeed.Publication{
		leafletPubURI: {URI: leafletPubURI, Name: "Zine", URL: "https://zine.example", FeedURL: "https://zine.example/rss", ShowInDiscover: true},
	}}
	c := &Client{standard: standard, leaflet: leaflet}

	got, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: feedkey.Normalize("https://zine.example/rss"), Kind: "rss", Title: "Zine", SiteURL: "https://zine.example"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
	if leaflet.callCount() != 1 || leaflet.seenURIs[0] != leafletPubURI {
		t.Errorf("leaflet resolver seenURIs = %+v, want exactly [%s] (RAW uri)", leaflet.seenURIs, leafletPubURI)
	}
}

func TestResolvePublication_LeafletSiblingTransientErrorNeverFallsBack(t *testing.T) {
	standard := &fakePublicationResolver{errByURI: map[string]error{leafletPubSiblingURI: &atclient.APIError{StatusCode: 500}}}
	leaflet := &fakeLeafletResolver{}
	c := &Client{standard: standard, leaflet: leaflet}

	if _, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected failure on a transient sibling error")
	}
	if leaflet.callCount() != 0 {
		t.Errorf("leaflet resolver calls = %d, want 0 (never fall back on a transient error)", leaflet.callCount())
	}
}

func TestResolvePublication_LeafletOptOutSkipped(t *testing.T) {
	standard := &fakePublicationResolver{errByURI: map[string]error{leafletPubSiblingURI: recordNotFoundErr()}}
	leaflet := &fakeLeafletResolver{byURI: map[string]*leafletfeed.Publication{
		leafletPubURI: {URI: leafletPubURI, Name: "Zine", FeedURL: "https://zine.example/rss", ShowInDiscover: false},
	}}
	c := &Client{standard: standard, leaflet: leaflet}

	if _, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{}); ok {
		t.Error("expected skip: showInDiscover false")
	}
}

func TestResolveLeafletSibling_SiblingOptedOut_SkipsWithoutRSSFallback(t *testing.T) {
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		leafletPubSiblingURI: {URI: leafletPubSiblingURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: false},
	}}
	leaflet := &fakeLeafletResolver{byURI: map[string]*leafletfeed.Publication{
		leafletPubURI: {URI: leafletPubURI, Name: "Zine", URL: "https://zine.example", FeedURL: "https://zine.example/rss", ShowInDiscover: true},
	}}
	c := &Client{standard: standard, leaflet: leaflet}

	if _, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{}); ok {
		t.Error("expected skip: sibling opted out")
	}
	if leaflet.callCount() != 0 {
		t.Errorf("leaflet resolver calls = %d, want 0 (sibling is authoritative, no RSS fallback)", leaflet.callCount())
	}
}

func TestResolvePublication_LeafletNoBasePathSkipped(t *testing.T) {
	standard := &fakePublicationResolver{errByURI: map[string]error{leafletPubSiblingURI: recordNotFoundErr()}}
	leaflet := &fakeLeafletResolver{byURI: map[string]*leafletfeed.Publication{
		leafletPubURI: {URI: leafletPubURI, Name: "Zine", ShowInDiscover: true}, // no base_path: FeedURL empty
	}}
	c := &Client{standard: standard, leaflet: leaflet}

	if _, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{}); ok {
		t.Error("expected skip: no base_path means no feed to fall back to")
	}
}

func TestResolvePublication_UnknownCollectionSkipped(t *testing.T) {
	standard := &fakePublicationResolver{}
	leaflet := &fakeLeafletResolver{}
	c := &Client{standard: standard, leaflet: leaflet}

	if _, ok := c.resolvePublication(context.Background(), "at://did:plc:x/com.example.zine/1", "", map[string]Subscription{}); ok {
		t.Error("expected skip: unknown collection")
	}
	if standard.calls != 0 || leaflet.callCount() != 0 {
		t.Errorf("resolvers called for unknown collection: standard=%d leaflet=%d, want 0 and 0", standard.calls, leaflet.callCount())
	}
}

func TestResolvePublication_InvalidURISkipped(t *testing.T) {
	c := &Client{}
	if _, ok := c.resolvePublication(context.Background(), "not-a-uri", "", map[string]Subscription{}); ok {
		t.Error("expected skip: unparseable at-uri")
	}
}

func TestDecode_StandardSubscriptionWithLeafletPublicationCarriesCreatedAt(t *testing.T) {
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		leafletPubSiblingURI: {URI: leafletPubSiblingURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: standard}

	got, ok := c.decode(context.Background(), standardSubscriptionCollection, recordEntry{
		URI: "at://did:plc:alice/site.standard.graph.subscription/3la",
		Value: map[string]any{
			"publication": leafletPubURI,
			"createdAt":   "2026-06-01T00:00:00Z",
		},
	}, map[string]Subscription{})
	if !ok {
		t.Fatal("expected ok")
	}
	want := Subscription{Key: leafletPubSiblingURI, Kind: "standardfeed", Title: "Zine", SiteURL: "https://zine.example", CreatedAt: "2026-06-01T00:00:00Z"}
	if got != want {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

// Pins the bug fix: the canonical resolution is what gets memoized in L1, not the first caller's fallbackTitle.
func TestResolvePublication_FallbackTitleNotMemoizedAcrossCallers(t *testing.T) {
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Canonical Name", URL: "https://zine.example", ShowInDiscover: true},
	}}
	c := &Client{standard: standard}
	cache := map[string]Subscription{}

	got1, ok := c.resolvePublication(context.Background(), pubURI, "Reader One's Title", cache)
	if !ok || got1.Title != "Reader One's Title" {
		t.Fatalf("first call: got = %+v, ok = %v", got1, ok)
	}
	got2, ok := c.resolvePublication(context.Background(), pubURI, "Reader Two's Title", cache)
	if !ok || got2.Title != "Reader Two's Title" {
		t.Fatalf("second call: got = %+v, ok = %v", got2, ok)
	}
	if standard.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (memoized)", standard.calls)
	}
}

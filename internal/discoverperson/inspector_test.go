package discoverperson

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/discovercrawl"
)

const testDID = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"

type fakeSubs struct {
	subs []discovercrawl.Subscription
	err  error
}

func (f fakeSubs) FetchSubscriptions(context.Context, syntax.DID) ([]discovercrawl.Subscription, error) {
	return f.subs, f.err
}

type fakeAuthored struct {
	pubs []discovercrawl.AuthoredPublication
	err  error
}

func (f fakeAuthored) FetchAuthoredPublications(context.Context, syntax.DID) ([]discovercrawl.AuthoredPublication, error) {
	return f.pubs, f.err
}

type fakeShares struct {
	shares []discovercrawl.Share
	err    error
}

func (f fakeShares) FetchShares(context.Context, syntax.DID) ([]discovercrawl.Share, error) {
	return f.shares, f.err
}

func sourceKeys(items []SourceItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}

func shareURLs(items []ShareItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ItemURL
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRecordsSortsNewestFirst(t *testing.T) {
	insp := New(
		fakeSubs{subs: []discovercrawl.Subscription{
			{Key: "https://a.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"},
			{Key: "https://b.example/feed", Kind: "rss", CreatedAt: "2026-07-10T00:00:00Z"},
		}},
		fakeAuthored{pubs: []discovercrawl.AuthoredPublication{
			{Key: "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/old", Kind: "standardfeed", LastPublishedAt: "2026-07-01T00:00:00Z"},
			{Key: "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/new", Kind: "standardfeed", LastPublishedAt: "2026-07-15T00:00:00Z"},
			{Key: "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/mid", Kind: "standardfeed", LastPublishedAt: "2026-07-08T00:00:00Z"},
		}},
		fakeShares{shares: []discovercrawl.Share{
			{Kind: "rss", ItemURL: "https://a.example/post1", CreatedAt: "2026-07-01T00:00:00Z"},
			{Kind: "rss", ItemURL: "https://b.example/post2", CreatedAt: "2026-07-05T00:00:00Z"},
		}},
	)

	got := insp.Records(context.Background(), testDID, nil)

	wantWrites := []string{
		"at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/new",
		"at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/mid",
		"at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/old",
	}
	if !equalStrings(sourceKeys(got.Writes), wantWrites) {
		t.Errorf("writes order = %v, want %v", sourceKeys(got.Writes), wantWrites)
	}
	wantReads := []string{"https://b.example/feed", "https://a.example/feed"}
	if !equalStrings(sourceKeys(got.Reads), wantReads) {
		t.Errorf("reads order = %v, want %v", sourceKeys(got.Reads), wantReads)
	}
	wantShares := []string{"https://b.example/post2", "https://a.example/post1"}
	if !equalStrings(shareURLs(got.Shares), wantShares) {
		t.Errorf("shares order = %v, want %v", shareURLs(got.Shares), wantShares)
	}
}

func TestRecordsDedupKeepsNewest(t *testing.T) {
	const writeKey = "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/x"
	const readKey = "https://c.example/feed"
	const shareURL = "https://c.example/post"

	insp := New(
		fakeSubs{subs: []discovercrawl.Subscription{
			{Key: readKey, Kind: "rss", Title: "Old Title", CreatedAt: "2026-07-01T00:00:00Z"},
			{Key: readKey, Kind: "rss", Title: "New Title", CreatedAt: "2026-07-10T00:00:00Z"},
		}},
		fakeAuthored{pubs: []discovercrawl.AuthoredPublication{
			{Key: writeKey, Kind: "standardfeed", Title: "Old Pub", LastPublishedAt: "2026-07-01T00:00:00Z"},
			{Key: writeKey, Kind: "standardfeed", Title: "New Pub", LastPublishedAt: "2026-07-10T00:00:00Z"},
		}},
		fakeShares{shares: []discovercrawl.Share{
			{Kind: "rss", ItemURL: shareURL, Comment: "old", CreatedAt: "2026-07-01T00:00:00Z"},
			{Kind: "rss", ItemURL: shareURL, Comment: "new", CreatedAt: "2026-07-10T00:00:00Z"},
		}},
	)

	got := insp.Records(context.Background(), testDID, nil)

	if len(got.Writes) != 1 || got.Writes[0].Title != "New Pub" {
		t.Errorf("writes dedup = %+v, want single New Pub", got.Writes)
	}
	if len(got.Reads) != 1 || got.Reads[0].Title != "New Title" {
		t.Errorf("reads dedup = %+v, want single New Title", got.Reads)
	}
	if len(got.Shares) != 1 || got.Shares[0].Comment != "new" {
		t.Errorf("shares dedup = %+v, want single new", got.Shares)
	}
}

func TestRecordsKeepsStandardfeedShareDocument(t *testing.T) {
	const document = "at://did:plc:cccccccccccccccccccccccc/site.standard.document/3example"
	insp := New(
		fakeSubs{},
		fakeAuthored{},
		fakeShares{shares: []discovercrawl.Share{{
			Kind:      "standardfeed",
			Document:  document,
			CreatedAt: "2026-07-10T00:00:00Z",
		}}},
	)

	got := insp.Records(context.Background(), testDID, nil)

	if len(got.Shares) != 1 {
		t.Fatalf("shares = %+v, want one", got.Shares)
	}
	field := reflect.ValueOf(got.Shares[0]).FieldByName("Document")
	if !field.IsValid() || field.String() != document {
		t.Errorf("share document was not preserved: %+v", got.Shares[0])
	}
}

func TestRecordsMarksInertFromViewerKeys(t *testing.T) {
	const subscribedWrite = "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/known"
	const subscribedRead = "https://known.example/feed"

	insp := New(
		fakeSubs{subs: []discovercrawl.Subscription{
			{Key: subscribedRead, Kind: "rss", CreatedAt: "2026-07-02T00:00:00Z"},
			{Key: "https://novel.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"},
		}},
		fakeAuthored{pubs: []discovercrawl.AuthoredPublication{
			{Key: subscribedWrite, Kind: "standardfeed", LastPublishedAt: "2026-07-02T00:00:00Z"},
			{Key: "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/novel", Kind: "standardfeed", LastPublishedAt: "2026-07-01T00:00:00Z"},
		}},
		fakeShares{},
	)

	viewerKeys := map[string]struct{}{subscribedWrite: {}, subscribedRead: {}}
	got := insp.Records(context.Background(), testDID, viewerKeys)

	assertSubscribed := func(section string, items []SourceItem, key string, want bool) {
		for _, it := range items {
			if it.Key == key {
				if it.Subscribed != want {
					t.Errorf("%s key %q Subscribed = %v, want %v", section, key, it.Subscribed, want)
				}
				return
			}
		}
		t.Errorf("%s key %q not found", section, key)
	}
	assertSubscribed("writes", got.Writes, subscribedWrite, true)
	assertSubscribed("writes", got.Writes, "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/novel", false)
	assertSubscribed("reads", got.Reads, subscribedRead, true)
	assertSubscribed("reads", got.Reads, "https://novel.example/feed", false)
}

func TestRecordsDegradesPerSection(t *testing.T) {
	goodSubs := []discovercrawl.Subscription{{Key: "https://a.example/feed", Kind: "rss", CreatedAt: "2026-07-01T00:00:00Z"}}
	goodPubs := []discovercrawl.AuthoredPublication{{Key: "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/pub/a", Kind: "standardfeed", LastPublishedAt: "2026-07-01T00:00:00Z"}}
	goodShares := []discovercrawl.Share{{Kind: "rss", ItemURL: "https://a.example/post", CreatedAt: "2026-07-01T00:00:00Z"}}
	boom := errContext("pds unreachable")

	t.Run("authored errors", func(t *testing.T) {
		insp := New(fakeSubs{subs: goodSubs}, fakeAuthored{err: boom}, fakeShares{shares: goodShares})
		got := insp.Records(context.Background(), testDID, nil)
		if len(got.Writes) != 0 {
			t.Errorf("writes = %v, want empty", got.Writes)
		}
		if len(got.Reads) != 1 || len(got.Shares) != 1 {
			t.Errorf("other sections degraded: reads=%v shares=%v", got.Reads, got.Shares)
		}
	})
	t.Run("subscriptions error", func(t *testing.T) {
		insp := New(fakeSubs{err: boom}, fakeAuthored{pubs: goodPubs}, fakeShares{shares: goodShares})
		got := insp.Records(context.Background(), testDID, nil)
		if len(got.Reads) != 0 {
			t.Errorf("reads = %v, want empty", got.Reads)
		}
		if len(got.Writes) != 1 || len(got.Shares) != 1 {
			t.Errorf("other sections degraded: writes=%v shares=%v", got.Writes, got.Shares)
		}
	})
	t.Run("shares error", func(t *testing.T) {
		insp := New(fakeSubs{subs: goodSubs}, fakeAuthored{pubs: goodPubs}, fakeShares{err: boom})
		got := insp.Records(context.Background(), testDID, nil)
		if len(got.Shares) != 0 {
			t.Errorf("shares = %v, want empty", got.Shares)
		}
		if len(got.Writes) != 1 || len(got.Reads) != 1 {
			t.Errorf("other sections degraded: writes=%v reads=%v", got.Writes, got.Reads)
		}
	})
}

func TestRecordsMalformedDIDIsEmpty(t *testing.T) {
	insp := New(
		fakeSubs{subs: []discovercrawl.Subscription{{Key: "https://a.example/feed", Kind: "rss"}}},
		fakeAuthored{pubs: []discovercrawl.AuthoredPublication{{Key: "at://x/pub/a", Kind: "standardfeed"}}},
		fakeShares{shares: []discovercrawl.Share{{Kind: "rss", ItemURL: "https://a.example/post"}}},
	)
	got := insp.Records(context.Background(), "not-a-did", nil)
	if len(got.Writes) != 0 || len(got.Reads) != 0 || len(got.Shares) != 0 {
		t.Errorf("malformed did produced records: %+v", got)
	}
}

func TestRecordsZeroRecordsIsEmptyNotError(t *testing.T) {
	insp := New(fakeSubs{}, fakeAuthored{}, fakeShares{})
	got := insp.Records(context.Background(), testDID, nil)
	if len(got.Writes) != 0 || len(got.Reads) != 0 || len(got.Shares) != 0 {
		t.Errorf("expected empty records, got %+v", got)
	}
}

func TestPreviewCaps(t *testing.T) {
	mk := func(n int) []SourceItem {
		out := make([]SourceItem, n)
		for i := range out {
			out[i] = SourceItem{Key: string(rune('a' + i))}
		}
		return out
	}
	t1 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	r := Records{
		Writes: mk(3),
		Reads:  mk(5),
		Shares: []ShareItem{{ItemURL: "u1", CreatedAt: t1}, {ItemURL: "u2", CreatedAt: t2}},
	}

	p := New(nil, nil, nil).Preview(r)
	if len(p.Writes) != 2 {
		t.Errorf("preview writes = %d, want 2", len(p.Writes))
	}
	if len(p.Reads) != 4 {
		t.Errorf("preview reads = %d, want 4", len(p.Reads))
	}
	if p.LatestShare == nil || p.LatestShare.ItemURL != "u1" {
		t.Errorf("preview latest share = %+v, want u1", p.LatestShare)
	}
}

func TestPreviewNoShares(t *testing.T) {
	p := New(nil, nil, nil).Preview(Records{Writes: []SourceItem{{Key: "a"}}})
	if p.LatestShare != nil {
		t.Errorf("latest share = %+v, want nil", p.LatestShare)
	}
	if len(p.Writes) != 1 {
		t.Errorf("preview writes = %d, want 1", len(p.Writes))
	}
}

type errContext string

func (e errContext) Error() string { return string(e) }

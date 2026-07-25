package discovercrawl

import (
	"context"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func mirrorEntry(t *testing.T, collection, rkey string, value map[string]any) RecordEntry {
	t.Helper()
	return NewRecordEntry(followedDID, collection, rkey, "bafyreiaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", value)
}

func TestNewRecordEntry_SynthesizesATURIFromMirrorColumns(t *testing.T) {
	got := NewRecordEntry(followedDID, morgenSaveCollection, "3abc", "bafyreiaaa", map[string]any{"itemUrl": "https://a.example/post"})
	want := "at://" + followedDID + "/" + morgenSaveCollection + "/3abc"
	if got.URI != want {
		t.Errorf("URI = %q, want %q", got.URI, want)
	}
	if got.CID != "bafyreiaaa" {
		t.Errorf("CID = %q", got.CID)
	}
	if got.Value["itemUrl"] != "https://a.example/post" {
		t.Errorf("Value = %+v", got.Value)
	}
}

func TestDecodeSave_PerCollectionShapes(t *testing.T) {
	cases := []struct {
		name       string
		collection string
		value      map[string]any
		wantOK     bool
		want       Save
	}{
		{
			name:       "morgen carries feed url provenance",
			collection: morgenSaveCollection,
			value:      map[string]any{"itemUrl": "https://a.example/post", "feedUrl": "https://a.example/feed", "createdAt": "2026-07-01T00:00:00Z"},
			wantOK:     true,
			want:       Save{Kind: "morgen", ItemURL: "https://a.example/post", FeedURL: "https://a.example/feed", CreatedAt: "2026-07-01T00:00:00Z"},
		},
		{
			name:       "skyreader uses url and savedAt",
			collection: skyreaderSaveCollection,
			value:      map[string]any{"url": "https://b.example/post", "savedAt": "2026-07-02T00:00:00Z"},
			wantOK:     true,
			want:       Save{Kind: "skyreader", ItemURL: "https://b.example/post", CreatedAt: "2026-07-02T00:00:00Z"},
		},
		{
			name:       "glean uses articleUrl",
			collection: gleanSaveCollection,
			value:      map[string]any{"articleUrl": "https://c.example/post", "feedUrl": "https://c.example/feed", "createdAt": "2026-07-03T00:00:00Z"},
			wantOK:     true,
			want:       Save{Kind: "glean", ItemURL: "https://c.example/post", FeedURL: "https://c.example/feed", CreatedAt: "2026-07-03T00:00:00Z"},
		},
		{
			name:       "missing url field is skipped",
			collection: morgenSaveCollection,
			value:      map[string]any{"createdAt": "2026-07-01T00:00:00Z"},
		},
		{
			name:       "unknown collection is skipped",
			collection: "com.example.unknown",
			value:      map[string]any{"itemUrl": "https://a.example/post"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DecodeSave(tc.collection, mirrorEntry(t, tc.collection, "1", tc.value))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("got = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDecodeSaves_DedupesByItemURLAcrossCollections(t *testing.T) {
	byCollection := map[string][]RecordEntry{
		morgenSaveCollection: {
			mirrorEntry(t, morgenSaveCollection, "1", map[string]any{"itemUrl": "https://shared.example/post", "createdAt": "2026-07-01T00:00:00Z"}),
		},
		gleanSaveCollection: {
			mirrorEntry(t, gleanSaveCollection, "1", map[string]any{"articleUrl": "https://shared.example/post", "createdAt": "2026-07-03T00:00:00Z"}),
		},
	}

	got := DecodeSaves(byCollection)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != "morgen" {
		t.Errorf("Kind = %q, want morgen (lexicon precedence order wins the dedupe)", got[0].Kind)
	}
}

func TestMergeShares_JoinsRecommendWithSidecarComment(t *testing.T) {
	const doc = "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/site.standard.document/3doc"
	byCollection := map[string][]RecordEntry{
		standardRecommendCollection: {
			mirrorEntry(t, standardRecommendCollection, "3rec", map[string]any{"document": doc, "createdAt": "2026-07-02T00:00:00Z"}),
		},
		morgenShareCollection: {
			mirrorEntry(t, morgenShareCollection, "3side", map[string]any{
				"itemUrl": "https://pub.example/post", "document": doc, "comment": "worth a read", "createdAt": "2026-07-02T00:00:01Z",
			}),
		},
	}

	got := MergeShares(byCollection)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != "standardfeed" || got[0].Document != doc {
		t.Fatalf("got = %+v", got[0])
	}
	if got[0].Comment != "worth a read" || got[0].ItemURL != "https://pub.example/post" {
		t.Errorf("sidecar not merged: %+v", got[0])
	}
	if got[0].CreatedAt != "2026-07-02T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want the recommend's stamp", got[0].CreatedAt)
	}
}

func TestMergeShares_StandaloneRSSAndSkyreaderShares(t *testing.T) {
	byCollection := map[string][]RecordEntry{
		morgenShareCollection: {
			mirrorEntry(t, morgenShareCollection, "1", map[string]any{"itemUrl": "https://a.example/post", "feedUrl": "https://a.example/feed", "createdAt": "2026-07-01T00:00:00Z"}),
		},
		skyreaderShareCollection: {
			mirrorEntry(t, skyreaderShareCollection, "1", map[string]any{"itemUrl": "https://c.example/post", "createdAt": "2026-07-03T00:00:00Z"}),
		},
	}

	got := MergeShares(byCollection)
	kinds := map[string]int{}
	for _, s := range got {
		kinds[s.Kind]++
	}
	if kinds["rss"] != 1 || kinds["skyreader"] != 1 || len(got) != 2 {
		t.Errorf("kinds = %+v (len %d), want one rss and one skyreader", kinds, len(got))
	}
}

func TestDecodeShare_RequiresItemURL(t *testing.T) {
	if _, ok := DecodeShare(mirrorEntry(t, morgenShareCollection, "1", map[string]any{"comment": "no url"})); ok {
		t.Error("ok = true, want false for a share without itemUrl")
	}
	got, ok := DecodeShare(mirrorEntry(t, morgenShareCollection, "3abc", map[string]any{"itemUrl": "https://a.example/post"}))
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got.Rkey != "3abc" {
		t.Errorf("Rkey = %q, want 3abc (read back off the synthesized at-uri)", got.Rkey)
	}
}

func TestDecodeRecommend_RequiresDocument(t *testing.T) {
	if _, ok := DecodeRecommend(mirrorEntry(t, standardRecommendCollection, "1", map[string]any{"createdAt": "2026-07-02T00:00:00Z"})); ok {
		t.Error("ok = true, want false for a recommend without document")
	}
}

func TestDecodeFollows_DedupesAndDropsSelf(t *testing.T) {
	const other = "did:plc:bbbbbbbbbbbbbbbbbbbbbbbb"
	byCollection := map[string][]RecordEntry{
		morgenFollowCollection: {
			mirrorEntry(t, morgenFollowCollection, "1", map[string]any{"subject": other}),
			mirrorEntry(t, morgenFollowCollection, "2", map[string]any{"subject": followedDID}),
		},
		skyreaderFollowCollection: {
			mirrorEntry(t, skyreaderFollowCollection, "1", map[string]any{"subject": other}),
		},
	}

	got := DecodeFollows(followedDID, byCollection)
	if len(got) != 1 || got[0].DID != other {
		t.Fatalf("got = %+v, want one %s row", got, other)
	}
}

func TestDecodeFollowSubject_RequiresSubject(t *testing.T) {
	if _, ok := DecodeFollowSubject(mirrorEntry(t, morgenFollowCollection, "1", map[string]any{})); ok {
		t.Error("ok = true, want false for a follow without subject")
	}
}

func TestClient_DecodeAuthoredPublications_VerifiedHandleFormWellKnown(t *testing.T) {
	client := verifyingClient(t, nil, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedHandle + "/" + authoredPublicationCollection + "/3p",
	}})
	byCollection := map[string][]RecordEntry{
		authoredPublicationCollection: {
			mirrorEntry(t, authoredPublicationCollection, "3p", map[string]any{"name": "Example Zine", "url": "https://zine.example"}),
		},
		standardDocumentCollectionForLatest: {
			mirrorEntry(t, standardDocumentCollectionForLatest, "9a", map[string]any{"publishedAt": "2026-07-01T00:00:00Z"}),
			mirrorEntry(t, standardDocumentCollectionForLatest, "9b", map[string]any{"publishedAt": "2026-07-08T00:00:00Z"}),
		},
	}

	did, _ := syntax.ParseDID(followedDID)
	got := client.DecodeAuthoredPublications(context.Background(), byCollection, did, syntax.Handle(followedHandle))
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(got), got)
	}
	if got[0].Key != "at://"+followedDID+"/"+authoredPublicationCollection+"/3p" {
		t.Errorf("Key = %q", got[0].Key)
	}
	if got[0].Title != "Example Zine" || got[0].Verification != verifiedOutcome {
		t.Errorf("got = %+v", got[0])
	}
	if got[0].LastPublishedAt != "2026-07-08T00:00:00Z" {
		t.Errorf("LastPublishedAt = %q, want the newest mirrored document", got[0].LastPublishedAt)
	}
}

func TestClient_DecodeAuthoredPublications_UnverifiedClaimIsDropped(t *testing.T) {
	client := verifyingClient(t, nil, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://did:plc:cccccccccccccccccccccccc/" + authoredPublicationCollection + "/3p",
	}})
	byCollection := map[string][]RecordEntry{
		authoredPublicationCollection: {
			mirrorEntry(t, authoredPublicationCollection, "3p", map[string]any{"name": "Example Zine", "url": "https://zine.example"}),
		},
	}

	did, _ := syntax.ParseDID(followedDID)
	if got := client.DecodeAuthoredPublications(context.Background(), byCollection, did, syntax.Handle(followedHandle)); len(got) != 0 {
		t.Errorf("got = %+v, want none (well-known names another repo)", got)
	}
}

func TestClient_DecodeSubscriptions_DecodesRSSVariantsAcrossLexicons(t *testing.T) {
	client, _ := newTestClient(t, nil)
	byCollection := map[string][]RecordEntry{
		morgenSubscriptionCollection: {
			mirrorEntry(t, morgenSubscriptionCollection, "1", map[string]any{
				"source":    map[string]any{"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://a.example/feed"},
				"title":     "Example Publication",
				"createdAt": "2026-07-01T00:00:00Z",
			}),
		},
		gleanSubscriptionCollection: {
			mirrorEntry(t, gleanSubscriptionCollection, "1", map[string]any{"feedUrl": "https://b.example/feed", "createdAt": "2026-07-02T00:00:00Z"}),
		},
	}

	got := client.DecodeSubscriptions(context.Background(), byCollection)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	byKey := map[string]Subscription{}
	for _, s := range got {
		byKey[s.Key] = s
	}
	if byKey["https://a.example/feed"].Title != "Example Publication" {
		t.Errorf("morgen subscription = %+v", byKey["https://a.example/feed"])
	}
	if byKey["https://b.example/feed"].CreatedAt != "2026-07-02T00:00:00Z" {
		t.Errorf("glean subscription = %+v", byKey["https://b.example/feed"])
	}
}

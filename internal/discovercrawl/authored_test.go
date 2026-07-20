package discovercrawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// fakeWellKnownFetcher canned-responds per siteURL, avoiding real network calls in the verification tests.
type fakeWellKnownFetcher struct {
	byURL map[string]string
	err   map[string]error
	delay time.Duration
}

func (f *fakeWellKnownFetcher) FetchWellKnown(ctx context.Context, siteURL string) (string, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err, ok := f.err[siteURL]; ok {
		return "", err
	}
	return f.byURL[siteURL], nil
}

// verifyingClient wires a fake well-known fetcher onto a test client's private verifier field, the seam production code fills with *standardfeed.Client.
func verifyingClient(t *testing.T, byCollection map[string][]map[string]any, verifier *fakeWellKnownFetcher) *Client {
	t.Helper()
	client, _ := newTestClient(t, authoredHandler(t, byCollection))
	client.verifier = verifier
	return client
}

func authoredHandler(t *testing.T, byCollection map[string][]map[string]any) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": byCollection[coll]})
	})
}

func TestClient_CrawlAuthoredPublications_DecodesOwnedPublication(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(got), got)
	}
	want := "at://" + followedDID + "/" + authoredPublicationCollection + "/3p"
	if got[0].Key != want || got[0].Kind != "standardfeed" || got[0].Title != "Zine" || got[0].SiteURL != "https://zine.example" {
		t.Errorf("got = %+v", got[0])
	}
}

func TestClient_CrawlAuthoredPublications_NoneOwnedSkipsLatestDocumentLookup(t *testing.T) {
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		if coll == standardDocumentCollectionForLatest {
			calls++
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want none", got)
	}
	if calls != 0 {
		t.Errorf("latest-document lookup calls = %d, want 0 (no owned publication)", calls)
	}
}

func TestClient_CrawlAuthoredPublications_CarriesLatestDocumentRecencyProxy(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
		standardDocumentCollectionForLatest: {
			{"uri": "at://" + followedDID + "/site.standard.document/9z", "value": map[string]any{
				"publishedAt": "2026-07-08T00:00:00Z",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 1 || got[0].LastPublishedAt != "2026-07-08T00:00:00Z" {
		t.Fatalf("got = %+v, want the newest document's publishedAt", got)
	}
}

func TestClient_CrawlAuthoredPublications_RecencyProxyIsNewestNotOldestDocument(t *testing.T) {
	// This fake honors listRecords ordering (newest-first, or oldest-first under reverse=true) to catch a reverse-ordered limit-1 lookup returning the wrong document.
	oldest := map[string]any{
		"uri":   "at://" + followedDID + "/site.standard.document/3aaaaaaaaaa2a",
		"value": map[string]any{"publishedAt": "2024-01-01T00:00:00Z"},
	}
	newest := map[string]any{
		"uri":   "at://" + followedDID + "/site.standard.document/3zzzzzzzzzz2z",
		"value": map[string]any{"publishedAt": "2026-07-01T00:00:00Z"},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != standardDocumentCollectionForLatest {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
				"uri":   "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
				"value": map[string]any{"name": "Zine", "url": "https://zine.example"},
			}}})
			return
		}
		records := []any{newest, oldest}
		if r.URL.Query().Get("reverse") == "true" {
			records = []any{oldest, newest}
		}
		json.NewEncoder(w).Encode(map[string]any{"records": records[:1]})
	})
	client, _ := newTestClient(t, handler)
	client.verifier = &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
	}}

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 1 || got[0].LastPublishedAt != "2026-07-01T00:00:00Z" {
		t.Fatalf("LastPublishedAt = %+v, want the newest document's publishedAt, not the oldest", got)
	}
}

func TestClient_CrawlAuthoredPublications_LatestDocumentLookupFailureDegradesToEmpty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll == standardDocumentCollectionForLatest {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
			"uri":   "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
			"value": map[string]any{"name": "Zine", "url": "https://zine.example"},
		}}})
	})
	client, _ := newTestClient(t, handler)
	client.verifier = &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
	}}

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications should not fail on a recency-lookup error: %v", err)
	}
	if len(got) != 1 || got[0].LastPublishedAt != "" {
		t.Fatalf("got = %+v, want the publication with no recency proxy", got)
	}
}

func TestClient_CrawlAuthoredPublications_MalformedRecordSkippedNotFatal(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/1", "value": map[string]any{
				"name": "No URL",
			}},
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/2", "value": map[string]any{
				"name": "Good", "url": "https://good.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://good.example": "at://" + followedDID + "/" + authoredPublicationCollection + "/2",
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications should not fail on malformed records: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Good" {
		t.Fatalf("got = %+v, want only the well-formed record", got)
	}
}

func TestClient_CrawlAuthoredPublications_VerifiesAgainstDIDFormWellKnown(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p",
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Zine" {
		t.Fatalf("got = %+v, want the DID-form-verified publication kept", got)
	}
}

func TestClient_CrawlAuthoredPublications_VerifiesAgainstHandleFormWellKnown(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://" + followedHandle + "/" + authoredPublicationCollection + "/3p",
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	want := "at://" + followedDID + "/" + authoredPublicationCollection + "/3p"
	if len(got) != 1 || got[0].Key != want {
		t.Fatalf("got = %+v, want the handle-form-verified publication kept, keyed by DID", got)
	}
}

func TestClient_CrawlAuthoredPublications_DropsRecordWhenWellKnownNamesADifferentPublication(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{byURL: map[string]string{
		"https://zine.example": "at://did:plc:someoneelse/" + authoredPublicationCollection + "/other",
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want the mismatched authorship claim dropped", got)
	}
}

func TestClient_CrawlAuthoredPublications_DropsRecordWhenSiteHasNoWellKnown(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	// FetchWellKnown's contract for a miss (no well-known, non-200, bad body) is ("", nil); no entry in byURL/err reproduces that.
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want the unverifiable claim dropped", got)
	}
}

func TestClient_CrawlAuthoredPublications_DropsRecordOnProbeTransportError(t *testing.T) {
	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{err: map[string]error{
		"https://zine.example": errors.New("connection refused"),
	}})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want the probe-error claim dropped", got)
	}
}

func TestClient_CrawlAuthoredPublications_ProbeTimeoutCountsAsDropped(t *testing.T) {
	orig := verificationTimeout
	verificationTimeout = 5 * time.Millisecond
	t.Cleanup(func() { verificationTimeout = orig })

	byCollection := map[string][]map[string]any{
		authoredPublicationCollection: {
			{"uri": "at://" + followedDID + "/" + authoredPublicationCollection + "/3p", "value": map[string]any{
				"name": "Zine", "url": "https://zine.example",
			}},
		},
	}
	client := verifyingClient(t, byCollection, &fakeWellKnownFetcher{delay: 50 * time.Millisecond})

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.CrawlAuthoredPublications(context.Background(), did)
	if err != nil {
		t.Fatalf("CrawlAuthoredPublications: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want a timed-out probe dropped like any other probe error", got)
	}
}

func TestClient_CrawlAuthoredPublications_UnknownPDSEndpointErrors(t *testing.T) {
	c := NewClient(&fakeResolver{}, http.DefaultClient, nil, nil, nil)
	did, _ := syntax.ParseDID("did:plc:nobody")
	if _, err := c.CrawlAuthoredPublications(context.Background(), did); err == nil {
		t.Fatal("expected error for unresolvable did")
	}
}

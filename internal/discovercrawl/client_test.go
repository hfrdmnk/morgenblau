package discovercrawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

type fakeResolver struct {
	byDID map[string]*identity.Identity
}

func (f *fakeResolver) LookupDID(_ context.Context, did syntax.DID) (*identity.Identity, error) {
	if ident, ok := f.byDID[did.String()]; ok {
		return ident, nil
	}
	return nil, fmt.Errorf("unknown did %s", did)
}

const followedDID = "did:plc:followed"
const followedHandle = "followed.example"

func TestClient_Crawl_AggregatesAllFourCollections(t *testing.T) {
	records := map[string][]map[string]any{
		morgenSubscriptionCollection: {
			{"uri": "at://" + followedDID + "/" + morgenSubscriptionCollection + "/1", "value": map[string]any{
				"source": map[string]any{"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://a.example/feed"},
			}},
		},
		skyreaderSubscriptionCollection: {
			{"uri": "at://" + followedDID + "/" + skyreaderSubscriptionCollection + "/1", "value": map[string]any{
				"feedUrl": "https://b.example/feed",
			}},
		},
		gleanSubscriptionCollection: {
			{"uri": "at://" + followedDID + "/" + gleanSubscriptionCollection + "/1", "value": map[string]any{
				"feedUrl": "https://c.example/feed",
			}},
		},
		standardSubscriptionCollection: {},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": records[coll]})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.Crawl(context.Background(), did)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	keys := map[string]bool{}
	for _, s := range got {
		keys[s.Key] = true
	}
	for _, want := range []string{"https://a.example/feed", "https://b.example/feed", "https://c.example/feed"} {
		if !keys[want] {
			t.Errorf("missing key %q in %+v", want, got)
		}
	}
}

func TestClient_Crawl_PagesUntilCursorEmpty(t *testing.T) {
	page := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != morgenSubscriptionCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		cursor := r.URL.Query().Get("cursor")
		page++
		switch cursor {
		case "":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{map[string]any{
					"uri": "at://" + followedDID + "/x/1",
					"value": map[string]any{"source": map[string]any{
						"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://a.example/feed",
					}},
				}},
				"cursor": "page2",
			})
		case "page2":
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{map[string]any{
					"uri": "at://" + followedDID + "/x/2",
					"value": map[string]any{"source": map[string]any{
						"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://b.example/feed",
					}},
				}},
			})
		default:
			t.Fatalf("unexpected cursor %q", cursor)
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.Crawl(context.Background(), did)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if page != 2 {
		t.Errorf("pages fetched = %d, want 2", page)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

type cursorPagingClient struct {
	calls    int
	repeated bool
}

func (f *cursorPagingClient) Get(_ context.Context, _ syntax.NSID, _ map[string]any, out any) error {
	f.calls++
	if f.calls > 110 {
		return errors.New("test paging guard reached")
	}
	resp := out.(*listRecordsResp)
	if f.repeated {
		resp.Cursor = "same"
	} else {
		resp.Cursor = fmt.Sprintf("page-%d", f.calls)
	}
	return nil
}

func TestPageRecords_RejectsRepeatedCursor(t *testing.T) {
	client := &cursorPagingClient{repeated: true}

	_, err := pageRecords(context.Background(), client, followedDID, morgenSubscriptionCollection)

	if err == nil || !strings.Contains(err.Error(), "repeated cursor") {
		t.Fatalf("err = %v, want repeated cursor error", err)
	}
}

func TestPageRecords_StopsAtPageLimit(t *testing.T) {
	client := &cursorPagingClient{}

	_, err := pageRecords(context.Background(), client, followedDID, morgenSubscriptionCollection)

	if err == nil || !strings.Contains(err.Error(), "page limit") {
		t.Fatalf("err = %v, want page limit error", err)
	}
	if client.calls >= 110 {
		t.Errorf("calls = %d, want pagination stopped before test guard", client.calls)
	}
}

type recordFloodClient struct{}

func (recordFloodClient) Get(_ context.Context, _ syntax.NSID, _ map[string]any, out any) error {
	out.(*listRecordsResp).Records = make([]recordEntry, 10_001)
	return nil
}

func TestPageRecords_StopsAtRecordLimit(t *testing.T) {
	_, err := pageRecords(context.Background(), recordFloodClient{}, followedDID, morgenSubscriptionCollection)

	if err == nil || !strings.Contains(err.Error(), "record limit") {
		t.Fatalf("err = %v, want record limit error", err)
	}
}

func TestClient_Crawl_RejectsOversizedListRecordsResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"records": []any{},
			"padding": strings.Repeat("x", 2<<20),
		})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	if _, err := client.Crawl(context.Background(), did); err == nil {
		t.Fatal("Crawl succeeded on an oversized listRecords response")
	}
}

func TestClient_Crawl_TimesOutWholeCrawl(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
		}
	})
	client, _ := newTestClient(t, handler)
	client.crawlTimeout = 20 * time.Millisecond

	did, _ := syntax.ParseDID(followedDID)
	started := time.Now()
	if _, err := client.Crawl(context.Background(), did); err == nil {
		t.Fatal("Crawl succeeded after exceeding its total deadline")
	}
	if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
		t.Errorf("crawl took %s, want total deadline to stop it promptly", elapsed)
	}
}

func TestClient_Crawl_UnknownVariantSkippedNotFatal(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		if coll != morgenSubscriptionCollection {
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"records": []any{
			map[string]any{
				"uri":   "at://" + followedDID + "/x/1",
				"value": map[string]any{"source": map[string]any{"$type": "blue.morgen.feed.subscription#carrierPigeon"}},
			},
			map[string]any{
				"uri": "at://" + followedDID + "/x/2",
				"value": map[string]any{"source": map[string]any{
					"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://good.example/feed",
				}},
			},
		}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.Crawl(context.Background(), did)
	if err != nil {
		t.Fatalf("Crawl should not fail on an unknown variant: %v", err)
	}
	if len(got) != 1 || got[0].Key != "https://good.example/feed" {
		t.Fatalf("got = %+v, want only the good record", got)
	}
}

func TestClient_Crawl_DedupesCrossLexiconSameCanonicalKey(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case morgenSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
				"uri": "at://" + followedDID + "/x/1",
				"value": map[string]any{"source": map[string]any{
					"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://same.example/feed",
				}},
			}}})
		case skyreaderSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
				"uri":   "at://" + followedDID + "/y/1",
				"value": map[string]any{"feedUrl": "https://same.example/feed"},
			}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.Crawl(context.Background(), did)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (deduped across lexicons): %+v", len(got), got)
	}
}

// TestClient_Crawl_DedupesCanonicalizedURLVariant proves a Skyreader feedUrl with a default port and trailing slash collapses against the bare canonical form (SPEC <discovery>).
func TestClient_Crawl_DedupesCanonicalizedURLVariant(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		coll := r.URL.Query().Get("collection")
		w.Header().Set("Content-Type", "application/json")
		switch coll {
		case morgenSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
				"uri": "at://" + followedDID + "/x/1",
				"value": map[string]any{"source": map[string]any{
					"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": "https://variant.example/feed",
				}},
			}}})
		case skyreaderSubscriptionCollection:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{map[string]any{
				"uri":   "at://" + followedDID + "/y/1",
				"value": map[string]any{"feedUrl": "https://variant.example:443/feed/"},
			}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
		}
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	got, err := client.Crawl(context.Background(), did)
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (default-port/trailing-slash variant deduped): %+v", len(got), got)
	}
	if got[0].Key != "https://variant.example/feed" {
		t.Errorf("Key = %q, want canonical form https://variant.example/feed", got[0].Key)
	}
}

func TestCrawl_SendsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	})
	client, _ := newTestClient(t, handler)

	did, _ := syntax.ParseDID(followedDID)
	if _, err := client.Crawl(context.Background(), did); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}
}

func TestClient_Crawl_UnknownPDSEndpointErrors(t *testing.T) {
	c := NewClient(&fakeResolver{}, http.DefaultClient, nil, nil, nil)
	did, _ := syntax.ParseDID("did:plc:nobody")
	if _, err := c.Crawl(context.Background(), did); err == nil {
		t.Fatal("expected error for unresolvable did")
	}
}

// newTestClient builds a Client with a nil PublicationResolver, fine for tests that never touch standardfeed records.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	did, err := syntax.ParseDID(followedDID)
	if err != nil {
		t.Fatalf("ParseDID: %v", err)
	}
	resolver := &fakeResolver{byDID: map[string]*identity.Identity{
		followedDID: {
			DID:    did,
			Handle: syntax.Handle(followedHandle),
			Services: map[string]identity.ServiceEndpoint{
				"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: srv.URL},
			},
		},
	}}
	return NewClient(resolver, srv.Client(), nil, nil, nil), srv
}

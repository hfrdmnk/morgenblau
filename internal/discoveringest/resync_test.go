package discoveringest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

// fakePDS answers listRecords for whichever collections a test gives it.
type fakePDS struct {
	*httptest.Server

	mu       sync.Mutex
	byColl   map[string][]map[string]any
	requests []string
	pages    map[string]int
}

func newFakePDS(t *testing.T, byColl map[string][]map[string]any) *fakePDS {
	t.Helper()
	p := &fakePDS{byColl: byColl, pages: map[string]int{}}
	p.Server = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.Close)
	return p
}

func (p *fakePDS) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/xrpc/com.atproto.repo.listRecords" {
		http.NotFound(w, r)
		return
	}
	coll := r.URL.Query().Get("collection")
	p.mu.Lock()
	p.requests = append(p.requests, coll)
	records := p.byColl[coll]
	p.mu.Unlock()

	out := map[string]any{"records": records}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (p *fakePDS) collectionsAsked() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.requests...)
}

type fakeResolver struct {
	endpoint string
	err      error
}

func (f fakeResolver) LookupDID(_ context.Context, did syntax.DID) (*identity.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &identity.Identity{
		DID: did,
		Services: map[string]identity.ServiceEndpoint{
			"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: f.endpoint},
		},
	}, nil
}

func TestRepoFetcher_ReadsEveryTrackedCollection(t *testing.T) {
	pds := newFakePDS(t, map[string][]map[string]any{
		"blue.morgen.feed.subscription": {{
			"uri":   fmt.Sprintf("at://%s/blue.morgen.feed.subscription/3aaaaaaaaaaa2", testDID),
			"cid":   testCID,
			"value": map[string]any{"$type": "blue.morgen.feed.subscription", "title": "Example Publication"},
		}},
	})
	fetcher := NewRepoFetcher(fakeResolver{endpoint: pds.URL}, pds.Client())

	got, err := fetcher.FetchRepoRecords(context.Background(), testDID)
	if err != nil {
		t.Fatalf("FetchRepoRecords: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("records = %+v, want 1", got)
	}
	if got[0].Collection != "blue.morgen.feed.subscription" || got[0].Rkey != "3aaaaaaaaaaa2" || got[0].CID != testCID {
		t.Errorf("record = %+v", got[0])
	}
	if !strings.Contains(got[0].Record, `"title":"Example Publication"`) {
		t.Errorf("record body = %s", got[0].Record)
	}
	if asked := pds.collectionsAsked(); len(asked) != len(Collections) {
		t.Errorf("asked for %d collections, want %d", len(asked), len(Collections))
	}
}

func TestRepoFetcher_UnparseableDIDFails(t *testing.T) {
	pds := newFakePDS(t, nil)
	fetcher := NewRepoFetcher(fakeResolver{endpoint: pds.URL}, pds.Client())

	if _, err := fetcher.FetchRepoRecords(context.Background(), "not-a-did"); err == nil {
		t.Fatal("want an error for an unparseable DID")
	}
}

func TestRepoFetcher_MissingPDSEndpointFails(t *testing.T) {
	pds := newFakePDS(t, nil)
	fetcher := NewRepoFetcher(fakeResolver{}, pds.Client())

	if _, err := fetcher.FetchRepoRecords(context.Background(), testDID); err == nil {
		t.Fatal("want an error when the identity carries no PDS")
	}
}

package discoverbatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/discovercrawl"
)

// TestBatch_Run_ThrottlesRealCrawlerPerPDSHost wires the real discovercrawl.Client against a shared fake PDS host and checks the bound holds.
func TestBatch_Run_ThrottlesRealCrawlerPerPDSHost(t *testing.T) {
	var inFlight, maxInFlight int64
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&inFlight, 1)
		defer atomic.AddInt64(&inFlight, -1)
		for {
			max := atomic.LoadInt64(&maxInFlight)
			if cur <= max || atomic.CompareAndSwapInt64(&maxInFlight, max, cur) {
				break
			}
		}
		time.Sleep(3 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"records": []any{}})
	}))
	defer pds.Close()

	const repoCount = 6
	dids := make([]string, repoCount)
	hostByDID := map[string]string{}
	for i := range dids {
		did := "did:plc:host-shared-" + string(rune('a'+i))
		dids[i] = did
		hostByDID[did] = pds.URL // every repo resolves to the same PDS host
	}

	relay := fakeRelay(t, dids)
	resolver := &fakeBatchResolver{hostByDID: hostByDID}
	realCrawler := discovercrawl.NewClient(resolver, pds.Client(), nil, nil, nil)

	b := New(relay.URL, relay.Client(), resolver, realCrawler, fakeEntries{})
	b.collections = []string{"site.standard.publication"}
	b.perHostConcurrency = 2
	w := newFakeWriter()
	withFakeTx(b, w)

	n, err := b.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != repoCount {
		t.Errorf("n = %d, want %d", n, repoCount)
	}
	if got := atomic.LoadInt64(&maxInFlight); got > 2 {
		t.Errorf("max concurrent requests to the shared PDS host = %d, want <= 2", got)
	}
	if got := atomic.LoadInt64(&maxInFlight); got < 2 {
		t.Errorf("max concurrent requests = %d, want the test to actually exercise concurrency (>=2)", got)
	}
}

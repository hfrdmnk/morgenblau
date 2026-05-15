package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type fakeLister struct {
	got     syntax.DID
	gotColl string
	records []map[string]any
	err     error
}

func (f *fakeLister) ListRecords(_ context.Context, did syntax.DID, coll string, _ *oauth.ClientSession) ([]map[string]any, error) {
	f.got = did
	f.gotColl = coll
	if f.err != nil {
		return nil, f.err
	}
	return f.records, nil
}

func TestSubscriptions_HappyPath(t *testing.T) {
	lister := &fakeLister{
		records: []map[string]any{
			{
				"uri":   "at://did:plc:alice/app.skyreader.feed.subscription/3la",
				"cid":   "bafyrei111",
				"value": map[string]any{"title": "Daring Fireball", "feedUrl": "https://daringfireball.net/feeds/main"},
			},
			{
				"uri":   "at://did:plc:alice/app.skyreader.feed.subscription/3lb",
				"cid":   "bafyrei222",
				"value": map[string]any{"feedUrl": "https://example.com/feed.xml"},
			},
		},
	}
	h := SubscriptionsHandler(lister)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if lister.got.String() != "did:plc:alice" {
		t.Errorf("ListRecords called with did = %q", lister.got)
	}
	if lister.gotColl != "app.skyreader.feed.subscription" {
		t.Errorf("ListRecords called with coll = %q", lister.gotColl)
	}

	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[0]["uri"] != lister.records[0]["uri"] {
		t.Errorf("first record uri mismatch")
	}
	val, _ := got[0]["value"].(map[string]any)
	if val["title"] != "Daring Fireball" {
		t.Errorf("value pass-through failed: %v", val)
	}
}

func TestSubscriptions_EmptyArray(t *testing.T) {
	lister := &fakeLister{records: []map[string]any{}}
	h := SubscriptionsHandler(lister)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d", rr.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty array, got %v", got)
	}
}

func TestSubscriptions_PDSError_502(t *testing.T) {
	lister := &fakeLister{err: fmt.Errorf("pds boom")}
	h := SubscriptionsHandler(lister)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestSubscriptions_NoSessionInContext_500(t *testing.T) {
	lister := &fakeLister{}
	h := SubscriptionsHandler(lister)
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

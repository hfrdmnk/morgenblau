package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/database/db"
)

// rkeyIndex wraps fakeIndex with the rkey-keyed lookup PATCH/DELETE need.
type rkeyIndex struct {
	*fakeIndex
	byRkey map[string]map[string]db.UserSubscription // did → rkey → row
	delMu  sync.Mutex
	deleted []string
}

func newRkeyIndex() *rkeyIndex {
	return &rkeyIndex{fakeIndex: newFakeIndex(), byRkey: map[string]map[string]db.UserSubscription{}}
}

func (r *rkeyIndex) GetUserSubscription(_ context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error) {
	if rows, ok := r.byRkey[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			return row, nil
		}
	}
	return db.UserSubscription{}, sql.ErrNoRows
}

func (r *rkeyIndex) DeleteUserSubscription(_ context.Context, arg db.DeleteUserSubscriptionParams) error {
	r.delMu.Lock()
	defer r.delMu.Unlock()
	r.deleted = append(r.deleted, arg.Did+":"+arg.Rkey)
	if rows, ok := r.byRkey[arg.Did]; ok {
		delete(rows, arg.Rkey)
	}
	return nil
}

func (r *rkeyIndex) seed(did, rkey, feedURL string) {
	if _, ok := r.byRkey[did]; !ok {
		r.byRkey[did] = map[string]db.UserSubscription{}
	}
	row := db.UserSubscription{
		Did:       did,
		Rkey:      rkey,
		AtUri:     "at://" + did + "/app.skyreader.feed.subscription/" + rkey,
		FeedUrl:   feedURL,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	}
	r.byRkey[did][rkey] = row
}

func TestSubscriptionsPatch_NoDiff_NoOp(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")

	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("PDS create called on no-diff: %d", pds.creates)
	}
}

func TestSubscriptionsPatch_OtherUserRkey_403(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:bob", "3la", "https://x")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"new"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestSubscriptionsDelete_HappyPath_204(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if len(idx.deleted) != 1 {
		t.Errorf("deleted = %v", idx.deleted)
	}
}

func TestSubscriptionsDelete_OtherUserRkey_403(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:bob", "3la", "https://x")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

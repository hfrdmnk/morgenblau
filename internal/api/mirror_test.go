package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
)

const mirrorDID = "did:plc:alice"

var errMirrorDown = errors.New("mirror write failed")

// recordingRepair counts repair dispatches so tests can assert exactly-once, and can fail the dispatch itself.
type recordingRepair struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (d *recordingRepair) StartManualRefresh(_ context.Context, did syntax.DID, sessionID string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, did.String()+":"+sessionID)
	if d.err != nil {
		return "", d.err
	}
	return "sync-1", nil
}

func (d *recordingRepair) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

type failingSaveMirror struct{ *fakeSavesIndex }

func (failingSaveMirror) UpsertUserSave(context.Context, db.UpsertUserSaveParams) error {
	return errMirrorDown
}

func (failingSaveMirror) DeleteUserSave(context.Context, db.DeleteUserSaveParams) error {
	return errMirrorDown
}

type failingShareMirror struct{ *fakeShareIndex }

func (failingShareMirror) UpsertUserShare(context.Context, db.UpsertUserShareParams) error {
	return errMirrorDown
}

func (failingShareMirror) DeleteUserShare(context.Context, db.DeleteUserShareParams) error {
	return errMirrorDown
}

type failingFollowMirror struct{ *fakeFollowsIndex }

func (failingFollowMirror) UpsertUserFollow(context.Context, db.UpsertUserFollowParams) error {
	return errMirrorDown
}

func (failingFollowMirror) DeleteUserFollow(context.Context, db.DeleteUserFollowParams) error {
	return errMirrorDown
}

type failingSubscriptionMirror struct{ *rkeyIndex }

func (failingSubscriptionMirror) DeleteUserSubscription(context.Context, db.DeleteUserSubscriptionParams) error {
	return errMirrorDown
}

// --- saves ---

func TestSavesCreate_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newFakeSavesIndex()
	repair := &recordingRepair{}
	h := SavesCreateHandler(idx, failingSaveMirror{idx}, &fakePDS{}, repair)

	body := `{"itemUrl":"https://example.test/post"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(body)), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; the PDS write succeeded so a mirror failure must not fail the request. body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1", got)
	}
}

func TestSavesCreate_MirrorOK_NoRepair(t *testing.T) {
	idx := newFakeSavesIndex()
	repair := &recordingRepair{}
	h := SavesCreateHandler(idx, idx, &fakePDS{}, repair)

	body := `{"itemUrl":"https://example.test/post"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(body)), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 0 {
		t.Errorf("repair dispatches = %d, want 0 on the happy path", got)
	}
}

func TestSavesCreate_MirrorAndRepairBothFail_StillSucceeds(t *testing.T) {
	idx := newFakeSavesIndex()
	repair := &recordingRepair{err: errors.New("dispatch refused")}
	h := SavesCreateHandler(idx, failingSaveMirror{idx}, &fakePDS{}, repair)

	body := `{"itemUrl":"https://example.test/post"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(body)), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 even when the repair dispatch also fails; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1 attempt", got)
	}
}

func TestSavesDelete_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newFakeSavesIndex()
	idx.seed(db.UserSave{Did: mirrorDID, Rkey: "3la", ItemUrl: "https://example.test/post"})
	repair := &recordingRepair{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, failingSaveMirror{idx}, &fakePDS{}, repair))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/saves/3la", nil), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1", got)
	}
}

// --- shares ---

func TestSharesCreateRSS_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newShareIndex()
	idx.entry = rssEntry()
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "rss", FeedUrl: shareRSSFeed}
	repair := &recordingRepair{}

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/shares", strings.NewReader(`{"entrySlug":"post"}`)), shareDID, "sid-1")
	rr := httptest.NewRecorder()
	SharesCreateHandler(idx, failingShareMirror{idx}, &fakePDS{}, repair).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1", got)
	}
}

func TestSharesDelete_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newShareIndex()
	idx.byRkey["3la"] = db.UserShare{Did: shareDID, Rkey: "3la", Kind: "rss"}
	repair := &recordingRepair{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/shares/{rkey}", SharesDeleteHandler(idx, failingShareMirror{idx}, &fakePDS{}, repair))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/shares/3la", nil), shareDID, "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1", got)
	}
}

// --- follows ---

func TestFollowsCreate_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newFakeFollowsIndex()
	repair := &recordingRepair{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		syntax.Handle("bob.example"): syntax.DID("did:plc:bob"),
	}}
	h := FollowsCreateHandler(idx, failingFollowMirror{idx}, &fakePDS{}, resolver, repair, nil)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"bob.example"}`)), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1", got)
	}
}

func TestFollowsCreate_MirrorOK_NoRepair(t *testing.T) {
	idx := newFakeFollowsIndex()
	repair := &recordingRepair{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		syntax.Handle("bob.example"): syntax.DID("did:plc:bob"),
	}}
	h := FollowsCreateHandler(idx, idx, &fakePDS{}, resolver, repair, nil)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"bob.example"}`)), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 0 {
		t.Errorf("repair dispatches = %d, want 0 on the happy path", got)
	}
}

// --- subscriptions ---

func TestSubscriptionsDelete_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed(mirrorDID, "3la", "https://blog.example/feed.xml")
	repair := &recordingRepair{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, failingSubscriptionMirror{idx}, &fakePDS{}, repair, nil))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), mirrorDID, "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rr.Code, rr.Body.String())
	}
	if got := repair.count(); got != 1 {
		t.Errorf("repair dispatches = %d, want 1", got)
	}
}

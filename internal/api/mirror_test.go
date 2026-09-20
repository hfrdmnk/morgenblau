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

// --- subscriptions ---

func TestSubscriptionsDelete_MirrorFailure_SucceedsAndDispatchesRepair(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed(mirrorDID, "3la", "https://blog.example/feed.xml")
	repair := &recordingRepair{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, failingSubscriptionMirror{idx}, &fakePDS{}, repair))

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

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
)

type fakeProfileSource struct {
	getCalls     int
	refreshCalls int
	profiles     map[syntax.DID]profiles.Profile
	err          error
}

func (f *fakeProfileSource) Get(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	f.getCalls++
	if f.err != nil {
		return profiles.Profile{}, f.err
	}
	if p, ok := f.profiles[did]; ok {
		return p, nil
	}
	return profiles.Profile{}, fmt.Errorf("not found")
}

func (f *fakeProfileSource) Refresh(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	f.refreshCalls++
	if f.err != nil {
		return profiles.Profile{}, f.err
	}
	if p, ok := f.profiles[did]; ok {
		return p, nil
	}
	return profiles.Profile{}, fmt.Errorf("not found")
}

func sptr(s string) *string { return &s }

func TestMeProfile_HappyPath(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "alice.example", DisplayName: sptr("Alice")},
	}}
	h := MeProfileHandler(src)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if src.refreshCalls != 1 {
		t.Errorf("Refresh calls = %d, want 1 (self-bypass)", src.refreshCalls)
	}
	if src.getCalls != 0 {
		t.Errorf("Get calls = %d, want 0", src.getCalls)
	}
	var got profiles.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DID != "did:plc:alice" || got.Handle != "alice.example" {
		t.Errorf("got = %+v", got)
	}
}

func TestMeProfile_NoSession_500(t *testing.T) {
	h := MeProfileHandler(&fakeProfileSource{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestProfileByDID_OtherUser_UsesCache(t *testing.T) {
	otherDID, _ := syntax.ParseDID("did:plc:bob")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		otherDID: {DID: "did:plc:bob", Handle: "bob.example"},
	}}
	h := ProfileByDIDHandler(src)

	mux := http.NewServeMux()
	mux.Handle("GET /api/profiles/{did}", h)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/did:plc:bob", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if src.getCalls != 1 || src.refreshCalls != 0 {
		t.Errorf("get=%d refresh=%d, want get=1 refresh=0", src.getCalls, src.refreshCalls)
	}
}

func TestProfileByDID_SelfDID_TakesBypass(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "alice.example"},
	}}
	h := ProfileByDIDHandler(src)

	mux := http.NewServeMux()
	mux.Handle("GET /api/profiles/{did}", h)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/did:plc:alice", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if src.refreshCalls != 1 || src.getCalls != 0 {
		t.Errorf("refresh=%d get=%d, want refresh=1 get=0", src.refreshCalls, src.getCalls)
	}
}

func TestProfileByDID_InvalidDID_400(t *testing.T) {
	h := ProfileByDIDHandler(&fakeProfileSource{})
	mux := http.NewServeMux()
	mux.Handle("GET /api/profiles/{did}", h)

	req := httptest.NewRequest(http.MethodGet, "/api/profiles/not-a-did", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

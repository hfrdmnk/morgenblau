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
	profiles        map[syntax.DID]profiles.Profile
	refreshProfiles map[syntax.DID]profiles.Profile
	err             error
}

func (f *fakeProfileSource) Get(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	if f.err != nil {
		return profiles.Profile{}, f.err
	}
	if p, ok := f.profiles[did]; ok {
		return p, nil
	}
	return profiles.Profile{}, fmt.Errorf("not found")
}

func (f *fakeProfileSource) Refresh(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	if f.err != nil {
		return profiles.Profile{}, f.err
	}
	if p, ok := f.refreshProfiles[did]; ok {
		return p, nil
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
		did: {DID: "did:plc:alice", Handle: "user.example.com", DisplayName: sptr("Alice")},
	}, refreshProfiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "user.example.com", DisplayName: sptr("Alice Fresh")},
	}}
	h := MeProfileHandler(src)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got profiles.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DID != "did:plc:alice" || got.Handle != "user.example.com" {
		t.Errorf("got = %+v", got)
	}
	if got.DisplayName == nil || *got.DisplayName != "Alice Fresh" {
		t.Errorf("DisplayName = %v, want fresh profile", got.DisplayName)
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
		otherDID: {DID: "did:plc:bob", Handle: "other-user.example.com", DisplayName: sptr("Bob Cached")},
	}, refreshProfiles: map[syntax.DID]profiles.Profile{
		otherDID: {DID: "did:plc:bob", Handle: "other-user.example.com", DisplayName: sptr("Bob Fresh")},
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
	var got profiles.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DisplayName == nil || *got.DisplayName != "Bob Cached" {
		t.Errorf("DisplayName = %v, want cached profile", got.DisplayName)
	}
}

func TestProfileByDID_SelfDID_TakesBypass(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "user.example.com", DisplayName: sptr("Alice Cached")},
	}, refreshProfiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "user.example.com", DisplayName: sptr("Alice Fresh")},
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
	var got profiles.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DisplayName == nil || *got.DisplayName != "Alice Fresh" {
		t.Errorf("DisplayName = %v, want fresh profile", got.DisplayName)
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

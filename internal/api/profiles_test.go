package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
)

type fakeProfileSource struct {
	profiles map[syntax.DID]profiles.Profile
	err      error
	getCalls atomic.Int64
}

func (f *fakeProfileSource) Get(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	f.getCalls.Add(1)
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
		did: {DID: "did:plc:alice", Handle: "user.example.com", DisplayName: sptr("Alice")},
	}}
	h := MeProfileHandler(src)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got meResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.DID != "did:plc:alice" || got.Handle != "user.example.com" {
		t.Errorf("got = %+v", got)
	}
	if got.DisplayName == nil || *got.DisplayName != "Alice" {
		t.Errorf("DisplayName = %v, want cached profile", got.DisplayName)
	}
	if !got.NeedsReauth {
		t.Error("NeedsReauth = false without standard subscription scope")
	}
}

func TestMeProfile_StandardSubscriptionScopeNeedsNoReauth(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		did: {DID: did.String(), Handle: "user.example.com"},
	}}
	rr := httptest.NewRecorder()
	MeProfileHandler(src).ServeHTTP(rr, withStandardWriteSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), did.String(), "sid-1"))

	var got meResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.NeedsReauth {
		t.Error("NeedsReauth = true with standard subscription scope")
	}
}

func TestMeProfile_UsesCache(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		did: {DID: did.String(), Handle: "user.example.com"},
	}}
	rr := httptest.NewRecorder()
	MeProfileHandler(src).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), did.String(), "sid-1"))
	if got := src.getCalls.Load(); got != 1 {
		t.Errorf("Get calls = %d, want 1", got)
	}
}

func TestMeProfile_NoSession_500(t *testing.T) {
	rr := httptest.NewRecorder()
	MeProfileHandler(&fakeProfileSource{}).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

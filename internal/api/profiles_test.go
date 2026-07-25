package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
)

type fakeProfileSource struct {
	profiles        map[syntax.DID]profiles.Profile
	refreshProfiles map[syntax.DID]profiles.Profile
	err             error

	getCalls     atomic.Int64
	refreshCalls atomic.Int64
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

func (f *fakeProfileSource) Refresh(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	f.refreshCalls.Add(1)
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
	if got.DisplayName == nil || *got.DisplayName != "Alice" {
		t.Errorf("DisplayName = %v, want cached profile", got.DisplayName)
	}
}

func TestMeProfile_UsesCacheNeverRefreshes(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "user.example.com"},
	}, refreshProfiles: map[syntax.DID]profiles.Profile{
		did: {DID: "did:plc:alice", Handle: "user.example.com"},
	}}
	h := MeProfileHandler(src)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := src.refreshCalls.Load(); got != 0 {
		t.Errorf("Refresh calls = %d, want 0: a hard page load must not block on a PDS round-trip", got)
	}
	if got := src.getCalls.Load(); got != 1 {
		t.Errorf("Get calls = %d, want 1", got)
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

// batchDID builds a syntactically valid placeholder DID that stays distinct across a batch.
func batchDID(n int) string { return fmt.Sprintf("did:plc:aaaaaaaaaaaaaaaaaaaa%04d", n) }

// batchGet issues a /api/profiles request for the given DIDs, percent-encoding each one the way the frontend does.
func batchGet(t *testing.T, src ProfileSource, dids ...string) *httptest.ResponseRecorder {
	t.Helper()
	encoded := make([]string, len(dids))
	for i, did := range dids {
		encoded[i] = url.QueryEscape(did)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/profiles?dids="+strings.Join(encoded, ","), nil)
	rr := httptest.NewRecorder()
	ProfilesBatchHandler(src).ServeHTTP(rr, req)
	return rr
}

func decodeBatch(t *testing.T, rr *httptest.ResponseRecorder) map[string]*profiles.Profile {
	t.Helper()
	var body struct {
		Profiles map[string]*profiles.Profile `json:"profiles"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	return body.Profiles
}

func TestProfilesBatch_ReturnsProfilesKeyedByDID(t *testing.T) {
	first, _ := syntax.ParseDID(batchDID(1))
	second, _ := syntax.ParseDID(batchDID(2))
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		first:  {DID: first.String(), Handle: "reader.example", DisplayName: sptr("First Reader"), Avatar: sptr("https://cdn.example.com/a.jpg"), Description: sptr("Reads things")},
		second: {DID: second.String(), Handle: "writer.example"},
	}}

	rr := batchGet(t, src, first.String(), second.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if src.refreshCalls.Load() != 0 {
		t.Errorf("Refresh calls = %d, want 0: the batch must stay cache-first", src.refreshCalls.Load())
	}

	got := decodeBatch(t, rr)
	if len(got) != 2 {
		t.Fatalf("got %d keys, want 2: %s", len(got), rr.Body.String())
	}
	p := got[first.String()]
	if p == nil || p.Handle != "reader.example" || p.DisplayName == nil || *p.DisplayName != "First Reader" {
		t.Fatalf("first profile = %+v", p)
	}
	if p.Avatar == nil || *p.Avatar != "https://cdn.example.com/a.jpg" || p.Description == nil || *p.Description != "Reads things" {
		t.Errorf("avatar/description not carried through: %+v", p)
	}
	if q := got[second.String()]; q == nil || q.Handle != "writer.example" {
		t.Errorf("second profile = %+v", q)
	}
}

func TestProfilesBatch_FailedLookupIsNull(t *testing.T) {
	ok, _ := syntax.ParseDID(batchDID(1))
	missing := batchDID(2)
	src := &fakeProfileSource{profiles: map[syntax.DID]profiles.Profile{
		ok: {DID: ok.String(), Handle: "reader.example"},
	}}

	rr := batchGet(t, src, ok.String(), missing)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	got := decodeBatch(t, rr)
	if p, present := got[missing]; !present || p != nil {
		t.Errorf("missing DID = %+v (present=%v), want the key present and null", p, present)
	}
	if p := got[ok.String()]; p == nil || p.Handle != "reader.example" {
		t.Errorf("resolvable DID = %+v, want a real profile alongside the failure", p)
	}
}

func TestProfilesBatch_RejectsBadInput(t *testing.T) {
	fifty := make([]string, 50)
	for i := range fifty {
		fifty[i] = batchDID(i)
	}

	tests := []struct {
		name string
		dids []string
		want int
	}{
		{"no dids", nil, http.StatusBadRequest},
		{"empty entry", []string{""}, http.StatusBadRequest},
		{"malformed did", []string{batchDID(1), "not-a-did"}, http.StatusBadRequest},
		{"at the cap", fifty, http.StatusOK},
		{"over the cap", append(append([]string{}, fifty...), batchDID(50)), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := batchGet(t, &fakeProfileSource{}, tt.dids...)
			if rr.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tt.want, rr.Body.String())
			}
			if tt.want == http.StatusBadRequest && !strings.Contains(rr.Body.String(), codeInvalidRequest) {
				t.Errorf("body = %s, want the %s code", rr.Body.String(), codeInvalidRequest)
			}
		})
	}
}

// gatedProfileSource holds every Get open until release closes, so the batch's peak concurrency is observable.
type gatedProfileSource struct {
	gate    int
	release chan struct{}
	reached chan struct{}
	once    sync.Once

	mu       sync.Mutex
	inFlight int
	peak     int
}

func (g *gatedProfileSource) Get(_ context.Context, did syntax.DID) (profiles.Profile, error) {
	g.mu.Lock()
	g.inFlight++
	if g.inFlight > g.peak {
		g.peak = g.inFlight
	}
	atGate := g.inFlight >= g.gate
	g.mu.Unlock()

	if atGate {
		g.once.Do(func() { close(g.reached) })
	}
	<-g.release

	g.mu.Lock()
	g.inFlight--
	g.mu.Unlock()
	return profiles.Profile{DID: did.String(), Handle: "reader.example"}, nil
}

func (g *gatedProfileSource) Refresh(ctx context.Context, did syntax.DID) (profiles.Profile, error) {
	return g.Get(ctx, did)
}

func (g *gatedProfileSource) peakConcurrency() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.peak
}

func TestProfilesBatch_ConcurrencyStaysAtLimit(t *testing.T) {
	const want = 8
	src := &gatedProfileSource{gate: want, release: make(chan struct{}), reached: make(chan struct{})}
	dids := make([]string, 24)
	for i := range dids {
		dids[i] = batchDID(i)
	}

	var rr *httptest.ResponseRecorder
	done := make(chan struct{})
	go func() {
		defer close(done)
		rr = batchGet(t, src, dids...)
	}()

	select {
	case <-src.reached:
	case <-time.After(10 * time.Second):
		close(src.release)
		t.Fatalf("only %d concurrent lookups after 10s, want %d", src.peakConcurrency(), want)
	}
	close(src.release)
	<-done

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := src.peakConcurrency(); got != want {
		t.Errorf("peak concurrency = %d, want %d", got, want)
	}
	if got := len(decodeBatch(t, rr)); got != len(dids) {
		t.Errorf("got %d keys, want %d", got, len(dids))
	}
}

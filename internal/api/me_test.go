package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/middleware/auth"
)

type fakeResolver struct {
	byDID map[syntax.DID]*identity.Identity
	err   error
}

func (f *fakeResolver) LookupDID(_ context.Context, did syntax.DID) (*identity.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	if ident, ok := f.byDID[did]; ok {
		return ident, nil
	}
	return nil, fmt.Errorf("not found")
}

func withSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &oauth.ClientSession{
		Data: &oauth.ClientSessionData{AccountDID: d, SessionID: sid},
	}
	ctx := auth.WithSession(req.Context(), sess)
	return req.WithContext(ctx)
}

func TestMe_HappyPath(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	handle, _ := syntax.ParseHandle("alice.bsky.social")
	r := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{
		did: {DID: did, Handle: handle},
	}}
	h := MeHandler(r)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["did"] != "did:plc:alice" {
		t.Errorf("did = %q", got["did"])
	}
	if got["handle"] != "alice.bsky.social" {
		t.Errorf("handle = %q", got["handle"])
	}
}

func TestMe_ResolverError_500(t *testing.T) {
	r := &fakeResolver{err: fmt.Errorf("DNS down")}
	h := MeHandler(r)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// If indigo's bidirectional verification failed, Handle is the sentinel
// "handle.invalid". Don't fall back to displaying it — the contract is
// "real handle present or fail".
func TestMe_HandleInvalid_500(t *testing.T) {
	did, _ := syntax.ParseDID("did:plc:alice")
	r := &fakeResolver{byDID: map[syntax.DID]*identity.Identity{
		did: {DID: did, Handle: syntax.HandleInvalid},
	}}
	h := MeHandler(r)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/me", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestMe_NoSessionInContext_500(t *testing.T) {
	// This shouldn't happen behind the middleware, but verify the handler
	// fails closed rather than panicking on nil session.
	r := &fakeResolver{}
	h := MeHandler(r)
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

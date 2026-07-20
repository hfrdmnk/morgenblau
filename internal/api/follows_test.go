package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
)

// fakeFollowsIndex covers both the reader and writer slices for follows handlers.
type fakeFollowsIndex struct {
	mu           sync.Mutex
	byRkey       map[string]map[string]db.UserFollow // did → rkey → row
	bySubject    map[string]map[string]db.UserFollow // did → subjectDid → row
	deleted      []string
	upserts      int
	getSubjectEr error
}

func newFakeFollowsIndex() *fakeFollowsIndex {
	return &fakeFollowsIndex{
		byRkey:    map[string]map[string]db.UserFollow{},
		bySubject: map[string]map[string]db.UserFollow{},
	}
}

func (f *fakeFollowsIndex) seed(row db.UserFollow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byRkey[row.Did]; !ok {
		f.byRkey[row.Did] = map[string]db.UserFollow{}
	}
	if _, ok := f.bySubject[row.Did]; !ok {
		f.bySubject[row.Did] = map[string]db.UserFollow{}
	}
	f.byRkey[row.Did][row.Rkey] = row
	f.bySubject[row.Did][row.SubjectDid] = row
}

func (f *fakeFollowsIndex) GetUserFollow(_ context.Context, arg db.GetUserFollowParams) (db.UserFollow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rows, ok := f.byRkey[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			return row, nil
		}
	}
	return db.UserFollow{}, sql.ErrNoRows
}

func (f *fakeFollowsIndex) GetUserFollowBySubjectDID(_ context.Context, arg db.GetUserFollowBySubjectDIDParams) (db.UserFollow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getSubjectEr != nil {
		return db.UserFollow{}, f.getSubjectEr
	}
	if rows, ok := f.bySubject[arg.Did]; ok {
		if row, ok := rows[arg.SubjectDid]; ok {
			return row, nil
		}
	}
	return db.UserFollow{}, sql.ErrNoRows
}

func (f *fakeFollowsIndex) ListUserFollows(_ context.Context, did string) ([]db.UserFollow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]db.UserFollow, 0)
	for _, r := range f.byRkey[did] {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeFollowsIndex) UpsertUserFollow(_ context.Context, arg db.UpsertUserFollowParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	row := db.UserFollow{
		Did:        arg.Did,
		Rkey:       arg.Rkey,
		AtUri:      arg.AtUri,
		SubjectDid: arg.SubjectDid,
		CreatedAt:  arg.CreatedAt,
		UpdatedAt:  arg.UpdatedAt,
	}
	if _, ok := f.byRkey[arg.Did]; !ok {
		f.byRkey[arg.Did] = map[string]db.UserFollow{}
	}
	if _, ok := f.bySubject[arg.Did]; !ok {
		f.bySubject[arg.Did] = map[string]db.UserFollow{}
	}
	f.byRkey[arg.Did][arg.Rkey] = row
	f.bySubject[arg.Did][arg.SubjectDid] = row
	return nil
}

func (f *fakeFollowsIndex) DeleteUserFollow(_ context.Context, arg db.DeleteUserFollowParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, arg.Did+":"+arg.Rkey)
	if rows, ok := f.byRkey[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			delete(f.bySubject[arg.Did], row.SubjectDid)
			delete(rows, arg.Rkey)
		}
	}
	return nil
}

// fakeHandleResolver stubs handle→DID resolution without a network identity directory.
type fakeHandleResolver struct {
	dids map[syntax.Handle]syntax.DID
	err  error
}

func (f *fakeHandleResolver) LookupHandle(_ context.Context, handle syntax.Handle) (*identity.Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	did, ok := f.dids[handle]
	if !ok {
		return nil, errors.New("handle not found")
	}
	return &identity.Identity{DID: did, Handle: handle}, nil
}

// --- POST /api/follows ---

func TestFollowsCreate_HappyPath_201(t *testing.T) {
	idx := newFakeFollowsIndex()
	pds := &fakePDS{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"alice.test": mustDID2(t, "did:plc:alice"),
	}}
	h := FollowsCreateHandler(idx, idx, pds, resolver)

	body := `{"handle":"alice.test"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got FollowWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SubjectDID != "did:plc:alice" {
		t.Errorf("subjectDid = %q", got.SubjectDID)
	}
	if pds.creates != 1 {
		t.Errorf("PDS creates = %d", pds.creates)
	}
	if pds.created[0].collection != "blue.morgen.graph.follow" {
		t.Errorf("collection = %q", pds.created[0].collection)
	}
	if pds.created[0].record["subject"] != "did:plc:alice" {
		t.Errorf("record subject = %v", pds.created[0].record["subject"])
	}
	if idx.upserts != 1 {
		t.Errorf("Tier-1 upserts = %d", idx.upserts)
	}
}

func TestFollowsCreate_StripsLeadingAt(t *testing.T) {
	idx := newFakeFollowsIndex()
	pds := &fakePDS{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"alice.test": mustDID2(t, "did:plc:alice"),
	}}
	h := FollowsCreateHandler(idx, idx, pds, resolver)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"@alice.test"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestFollowsCreate_DedupeGuard_Idempotent(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{
		Did:        "did:plc:me",
		Rkey:       "3faOLD",
		AtUri:      "at://did:plc:me/blue.morgen.graph.follow/3faOLD",
		SubjectDid: "did:plc:alice",
		CreatedAt:  "2026-05-15T10:00:00Z",
		UpdatedAt:  "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"alice.test": mustDID2(t, "did:plc:alice"),
	}}
	h := FollowsCreateHandler(idx, idx, pds, resolver)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"alice.test"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (existing record); body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS create called on dedupe path: %d", pds.creates)
	}
	var got FollowWire
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Rkey != "3faOLD" {
		t.Errorf("rkey = %q, want existing 3faOLD", got.Rkey)
	}
}

func TestFollowsCreate_MissingHandle_400(t *testing.T) {
	idx := newFakeFollowsIndex()
	h := FollowsCreateHandler(idx, idx, &fakePDS{}, &fakeHandleResolver{})
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"  "}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestFollowsCreate_InvalidHandleSyntax_400_NoWrite(t *testing.T) {
	idx := newFakeFollowsIndex()
	pds := &fakePDS{}
	h := FollowsCreateHandler(idx, idx, pds, &fakeHandleResolver{})
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"not a handle!"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if pds.creates != 0 || idx.upserts != 0 {
		t.Errorf("write happened on invalid handle: creates=%d upserts=%d", pds.creates, idx.upserts)
	}
}

func TestFollowsCreate_UnresolvableHandle_422_NoWrite(t *testing.T) {
	idx := newFakeFollowsIndex()
	pds := &fakePDS{}
	resolver := &fakeHandleResolver{err: errors.New("not found")}
	h := FollowsCreateHandler(idx, idx, pds, resolver)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"nobody.test"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if pds.creates != 0 || idx.upserts != 0 {
		t.Errorf("write happened on unresolvable handle: creates=%d upserts=%d", pds.creates, idx.upserts)
	}
}

func TestFollowsCreate_SelfFollow_422_NoWrite(t *testing.T) {
	idx := newFakeFollowsIndex()
	pds := &fakePDS{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"reader.example": mustDID2(t, "did:plc:reader"),
	}}
	h := FollowsCreateHandler(idx, idx, pds, resolver)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"reader.example"}`)), "did:plc:reader", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", rr.Code, rr.Body.String())
	}
	var body errorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != codeUnprocessable || body.Message != "you can't follow yourself" {
		t.Errorf("body = %+v", body)
	}
	if pds.creates != 0 || idx.upserts != 0 {
		t.Errorf("write happened on self-follow: creates=%d upserts=%d", pds.creates, idx.upserts)
	}
}

// A type-converted DID bypasses syntax.ParseDID to model a subject the lexicon rejects; validation must fail before the PDS write, not after.
func TestFollowsCreate_InvalidRecord_500_NoWrite(t *testing.T) {
	idx := newFakeFollowsIndex()
	pds := &fakePDS{}
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"alice.test": syntax.DID("banana"),
	}}
	h := FollowsCreateHandler(idx, idx, pds, resolver)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"alice.test"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_record") {
		t.Errorf("body = %s, want code invalid_record", rr.Body.String())
	}
	if pds.creates != 0 || idx.upserts != 0 {
		t.Errorf("write happened on invalid record: creates=%d upserts=%d", pds.creates, idx.upserts)
	}
}

func TestFollowsCreate_PDSFailure_502(t *testing.T) {
	idx := newFakeFollowsIndex()
	resolver := &fakeHandleResolver{dids: map[syntax.Handle]syntax.DID{
		"alice.test": mustDID2(t, "did:plc:alice"),
	}}
	h := FollowsCreateHandler(idx, idx, failingPDS{}, resolver)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/follows", strings.NewReader(`{"handle":"alice.test"}`)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if idx.upserts != 0 {
		t.Errorf("Tier-1 upsert fired despite PDS failure: %d", idx.upserts)
	}
}

// --- DELETE /api/follows/{rkey} ---

func TestFollowsDelete_HappyPath_204(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:me", Rkey: "3fa", SubjectDid: "did:plc:alice"})
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{
		"blue.morgen.graph.follow": {
			{URI: "at://did:plc:me/blue.morgen.graph.follow/3fa", Value: map[string]any{"subject": "did:plc:alice"}},
		},
	}}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/follows/{rkey}", FollowsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/follows/3fa", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if len(idx.deleted) != 1 {
		t.Errorf("deleted = %v", idx.deleted)
	}
	if len(pds.deleted) != 1 || pds.deleted[0] != "blue.morgen.graph.follow/3fa" {
		t.Errorf("PDS deletes = %v", pds.deleted)
	}
}

// Guards against a second device leaving a duplicate follow record: unfollowing must tombstone both PDS records or the survivor resurrects the follow on reconcile.
func TestFollowsDelete_DuplicateOnPDS_DeletesBothRecords(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:me", Rkey: "3faOLD", SubjectDid: "did:plc:alice"})
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{
		"blue.morgen.graph.follow": {
			{URI: "at://did:plc:me/blue.morgen.graph.follow/3faOLD", Value: map[string]any{"subject": "did:plc:alice"}},
			{URI: "at://did:plc:me/blue.morgen.graph.follow/3faNEW", Value: map[string]any{"subject": "did:plc:alice"}},
			{URI: "at://did:plc:me/blue.morgen.graph.follow/3fb", Value: map[string]any{"subject": "did:plc:bob"}},
		},
	}}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/follows/{rkey}", FollowsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/follows/3faOLD", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rr.Code, rr.Body.String())
	}

	wantDeletes := map[string]bool{
		"blue.morgen.graph.follow/3faOLD": true,
		"blue.morgen.graph.follow/3faNEW": true,
	}
	if len(pds.deleted) != len(wantDeletes) {
		t.Fatalf("PDS deletes = %v, want exactly %v", pds.deleted, wantDeletes)
	}
	for _, d := range pds.deleted {
		if !wantDeletes[d] {
			t.Errorf("unexpected PDS delete %q", d)
		}
	}
	for _, d := range pds.deleted {
		if d == "blue.morgen.graph.follow/3fb" {
			t.Error("deleted a different subject's follow record")
		}
	}
}

func TestFollowsDelete_ListFailure_502_NoLocalDelete(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:me", Rkey: "3fa", SubjectDid: "did:plc:alice"})
	pds := &fakePDS{listErr: errors.New("pds down")}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/follows/{rkey}", FollowsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/follows/3fa", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if len(idx.deleted) != 0 {
		t.Errorf("Tier-1 delete fired despite PDS list failure: %v", idx.deleted)
	}
}

func TestFollowsDelete_OtherUserRkey_404(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:bob", Rkey: "3fa", SubjectDid: "did:plc:x"})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/follows/{rkey}", FollowsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/follows/3fa", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (collapsed with not-owned)", rr.Code)
	}
}

func TestFollowsDelete_PDSFailure_502_NoLocalDelete(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:me", Rkey: "3fa", SubjectDid: "did:plc:alice"})
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/follows/{rkey}", FollowsDeleteHandler(idx, idx, failingPDS{}))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/follows/3fa", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if len(idx.deleted) != 0 {
		t.Errorf("Tier-1 delete fired despite PDS failure: %v", idx.deleted)
	}
}

// --- GET /api/follows ---

func TestFollowsList_ScopedToOwner(t *testing.T) {
	idx := newFakeFollowsIndex()
	idx.seed(db.UserFollow{Did: "did:plc:me", Rkey: "3fa", SubjectDid: "did:plc:alice", CreatedAt: "2026-06-01T00:00:00Z"})
	idx.seed(db.UserFollow{Did: "did:plc:bob", Rkey: "3fb", SubjectDid: "did:plc:carol", CreatedAt: "2026-06-01T00:00:00Z"})
	h := FollowsListHandler(idx)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/follows", nil), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []FollowWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SubjectDID != "did:plc:alice" {
		t.Errorf("got = %+v, want only did:plc:me's own follow", got)
	}
}

func mustDID2(t *testing.T, s string) syntax.DID {
	t.Helper()
	d, err := syntax.ParseDID(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

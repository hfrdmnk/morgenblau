package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/database/db"
)

// fakeSavesIndex covers both the reader and writer slices for saves handlers.
type fakeSavesIndex struct {
	mu           sync.Mutex
	byRkey       map[string]map[string]db.UserSave // did → rkey → row
	byItemURL    map[string]map[string]db.UserSave // did → itemUrl → row
	deleted      []string
	upserts      int
	getItemURLEr error
}

func newFakeSavesIndex() *fakeSavesIndex {
	return &fakeSavesIndex{
		byRkey:    map[string]map[string]db.UserSave{},
		byItemURL: map[string]map[string]db.UserSave{},
	}
}

func (f *fakeSavesIndex) seed(row db.UserSave) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byRkey[row.Did]; !ok {
		f.byRkey[row.Did] = map[string]db.UserSave{}
	}
	if _, ok := f.byItemURL[row.Did]; !ok {
		f.byItemURL[row.Did] = map[string]db.UserSave{}
	}
	f.byRkey[row.Did][row.Rkey] = row
	f.byItemURL[row.Did][row.ItemUrl] = row
}

func (f *fakeSavesIndex) GetUserSave(_ context.Context, arg db.GetUserSaveParams) (db.UserSave, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rows, ok := f.byRkey[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			return row, nil
		}
	}
	return db.UserSave{}, sql.ErrNoRows
}

func (f *fakeSavesIndex) GetUserSaveByItemURL(_ context.Context, arg db.GetUserSaveByItemURLParams) (db.UserSave, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getItemURLEr != nil {
		return db.UserSave{}, f.getItemURLEr
	}
	if rows, ok := f.byItemURL[arg.Did]; ok {
		if row, ok := rows[arg.ItemUrl]; ok {
			return row, nil
		}
	}
	return db.UserSave{}, sql.ErrNoRows
}

func (f *fakeSavesIndex) UpsertUserSave(_ context.Context, arg db.UpsertUserSaveParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	row := db.UserSave{
		Did:       arg.Did,
		Rkey:      arg.Rkey,
		AtUri:     arg.AtUri,
		ItemUrl:   arg.ItemUrl,
		FeedUrl:   arg.FeedUrl,
		CreatedAt: arg.CreatedAt,
		UpdatedAt: arg.UpdatedAt,
	}
	if _, ok := f.byRkey[arg.Did]; !ok {
		f.byRkey[arg.Did] = map[string]db.UserSave{}
	}
	if _, ok := f.byItemURL[arg.Did]; !ok {
		f.byItemURL[arg.Did] = map[string]db.UserSave{}
	}
	f.byRkey[arg.Did][arg.Rkey] = row
	f.byItemURL[arg.Did][arg.ItemUrl] = row
	return nil
}

func (f *fakeSavesIndex) DeleteUserSave(_ context.Context, arg db.DeleteUserSaveParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, arg.Did+":"+arg.Rkey)
	if rows, ok := f.byRkey[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			delete(f.byItemURL[arg.Did], row.ItemUrl)
			delete(rows, arg.Rkey)
		}
	}
	return nil
}

// --- POST /api/saves ---

func TestSavesCreate_HappyPath_201(t *testing.T) {
	idx := newFakeSavesIndex()
	pds := &fakePDS{}
	h := SavesCreateHandler(idx, idx, pds)

	body := `{"itemUrl":"https://example.test/post","feedUrl":"https://example.test/feed.xml"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got SaveWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ItemURL != "https://example.test/post" {
		t.Errorf("itemUrl = %q", got.ItemURL)
	}
	if pds.creates != 1 {
		t.Errorf("PDS creates = %d", pds.creates)
	}
	if idx.upserts != 1 {
		t.Errorf("Tier-1 upserts = %d", idx.upserts)
	}
}

func TestSavesCreate_DedupeGuard_Idempotent(t *testing.T) {
	idx := newFakeSavesIndex()
	idx.seed(db.UserSave{
		Did:       "did:plc:alice",
		Rkey:      "3laOLD",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.save/3laOLD",
		ItemUrl:   "https://example.test/post",
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	h := SavesCreateHandler(idx, idx, pds)

	body := `{"itemUrl":"https://example.test/post"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(body)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (existing record); body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS create called on dedupe path: %d", pds.creates)
	}
	var got SaveWire
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Rkey != "3laOLD" {
		t.Errorf("rkey = %q, want existing 3laOLD", got.Rkey)
	}
}

func TestSavesCreate_MissingItemURL_400(t *testing.T) {
	idx := newFakeSavesIndex()
	h := SavesCreateHandler(idx, idx, &fakePDS{})
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(`{"itemUrl":"  "}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSavesCreate_PDSFailure_502(t *testing.T) {
	idx := newFakeSavesIndex()
	h := SavesCreateHandler(idx, idx, failingPDS{})
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(`{"itemUrl":"https://example.test/post"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

// --- DELETE /api/saves/{rkey} ---

func TestSavesDelete_HappyPath_204(t *testing.T) {
	idx := newFakeSavesIndex()
	idx.seed(db.UserSave{
		Did: "did:plc:alice", Rkey: "3la", ItemUrl: "https://example.test/post",
	})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/saves/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if len(idx.deleted) != 1 {
		t.Errorf("deleted = %v", idx.deleted)
	}
}

func TestSavesDelete_OtherUserRkey_404(t *testing.T) {
	idx := newFakeSavesIndex()
	idx.seed(db.UserSave{Did: "did:plc:bob", Rkey: "3la", ItemUrl: "https://x"})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/saves/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (collapsed with not-owned)", rr.Code)
	}
}

func TestSavesDelete_PDSFailure_502_NoLocalDelete(t *testing.T) {
	idx := newFakeSavesIndex()
	idx.seed(db.UserSave{Did: "did:plc:alice", Rkey: "3la", ItemUrl: "https://example.test/post"})
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, idx, failingPDS{}))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/saves/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if len(idx.deleted) != 0 {
		t.Errorf("Tier-1 delete fired despite PDS failure: %v", idx.deleted)
	}
}

func TestSavesCreate_InvalidRecord_500_NoWrite(t *testing.T) {
	idx := newFakeSavesIndex()
	pds := &fakePDS{}
	h := SavesCreateHandler(idx, idx, pds)

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(`{"itemUrl":"not-a-url"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_record") {
		t.Errorf("body = %q, want invalid_record code", rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0 (validation must run before the write)", pds.creates)
	}
	if idx.upserts != 0 {
		t.Errorf("Tier-1 upserts = %d, want 0", idx.upserts)
	}
}

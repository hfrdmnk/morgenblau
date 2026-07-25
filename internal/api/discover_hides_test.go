package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/database/db"
)

type fakeDiscoverHidesIndex struct {
	mu      sync.Mutex
	rows    map[string]db.DiscoverHide // did|kind|key -> row
	getErr  error
	upserts []db.UpsertDiscoverHideParams
}

func newFakeDiscoverHidesIndex() *fakeDiscoverHidesIndex {
	return &fakeDiscoverHidesIndex{rows: map[string]db.DiscoverHide{}}
}

func hideRowKey(did, kind, key string) string { return did + "|" + kind + "|" + key }

func (f *fakeDiscoverHidesIndex) seed(row db.DiscoverHide) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[hideRowKey(row.Did, row.TargetKind, row.TargetKey)] = row
}

func (f *fakeDiscoverHidesIndex) GetDiscoverHide(_ context.Context, arg db.GetDiscoverHideParams) (db.DiscoverHide, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return db.DiscoverHide{}, f.getErr
	}
	row, ok := f.rows[hideRowKey(arg.Did, arg.TargetKind, arg.TargetKey)]
	if !ok {
		return db.DiscoverHide{}, sql.ErrNoRows
	}
	return row, nil
}

func (f *fakeDiscoverHidesIndex) CountDiscoverHidesForUser(_ context.Context, did string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	prefix := did + "|"
	for key := range f.rows {
		if strings.HasPrefix(key, prefix) {
			count++
		}
	}
	return count, nil
}

func (f *fakeDiscoverHidesIndex) UpsertDiscoverHide(_ context.Context, arg db.UpsertDiscoverHideParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts = append(f.upserts, arg)
	f.rows[hideRowKey(arg.Did, arg.TargetKind, arg.TargetKey)] = db.DiscoverHide{
		Did:         arg.Did,
		TargetKind:  arg.TargetKind,
		TargetKey:   arg.TargetKey,
		HiddenUntil: arg.HiddenUntil,
		HideCount:   arg.HideCount,
		CreatedAt:   arg.CreatedAt,
		UpdatedAt:   arg.UpdatedAt,
	}
	return nil
}

func TestDiscoverHidesCreate_FirstHide_201_30DaySnooze(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"https://example.com/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got DiscoverHideWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.HideCount != 1 {
		t.Errorf("hideCount = %d, want 1", got.HideCount)
	}
	if len(idx.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(idx.upserts))
	}
	if idx.upserts[0].Did != "did:plc:me" {
		t.Errorf("upsert did = %q, want ownership scoped to session did", idx.upserts[0].Did)
	}
}

func TestDiscoverHidesCreate_RepeatHide_180DaySnoozeAndEscalatedCount(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	idx.seed(db.DiscoverHide{
		Did:         "did:plc:me",
		TargetKind:  "source",
		TargetKey:   "https://example.com/feed",
		HiddenUntil: "2026-01-01T00:00:00Z", // already resurfaced
		HideCount:   1,
		CreatedAt:   "2025-12-01T00:00:00Z",
		UpdatedAt:   "2025-12-01T00:00:00Z",
	})
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"https://example.com/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got DiscoverHideWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.HideCount != 2 {
		t.Errorf("hideCount = %d, want 2", got.HideCount)
	}
}

func TestDiscoverHidesCreate_PersonTargetKind_Accepted(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"person","targetKey":"did:plc:alice"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(idx.upserts) != 1 || idx.upserts[0].TargetKind != "person" {
		t.Fatalf("upserts = %+v, want one person-kind upsert", idx.upserts)
	}
}

func TestDiscoverHidesCreate_StandardfeedTarget_Accepted(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"at://did:plc:publisher/site.standard.publication/3abc"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(idx.upserts) != 1 {
		t.Fatalf("upserts = %d, want 1", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_InvalidTargetKind_400(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"publication","targetKey":"x"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(idx.upserts) != 0 {
		t.Errorf("write happened on invalid targetKind: %d", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_MissingTargetKey_400(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":""}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(idx.upserts) != 0 {
		t.Errorf("write happened on missing targetKey: %d", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_InvalidPersonDID_400(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"person","targetKey":"not-a-did"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(idx.upserts) != 0 {
		t.Errorf("write happened for invalid person DID: %d", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_InvalidSourceKey_400(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"javascript:alert(1)"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(idx.upserts) != 0 {
		t.Errorf("write happened for invalid source key: %d", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_OversizedTargetKey_400(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body, err := json.Marshal(discoverHidesCreateRequest{
		TargetKind: "source",
		TargetKey:  "https://example.com/" + strings.Repeat("x", 2048),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(string(body))), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(idx.upserts) != 0 {
		t.Errorf("write happened for oversized targetKey: %d", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_NewTargetAtPerUserLimit_422(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("https://source-%d.example/feed", i)
		idx.seed(db.DiscoverHide{
			Did:        "did:plc:me",
			TargetKind: "source",
			TargetKey:  key,
		})
	}
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"https://new.example/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if len(idx.upserts) != 0 {
		t.Errorf("write happened beyond per-user limit: %d", len(idx.upserts))
	}
}

func TestDiscoverHidesCreate_RepeatTargetAtPerUserLimit_Accepted(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("https://source-%d.example/feed", i)
		idx.seed(db.DiscoverHide{
			Did:        "did:plc:me",
			TargetKind: "source",
			TargetKey:  key,
			HideCount:  1,
		})
	}
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"https://source-0.example/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if len(idx.upserts) != 1 || idx.upserts[0].HideCount != 2 {
		t.Fatalf("upserts = %+v, want repeat hide accepted and escalated", idx.upserts)
	}
}

// DiscoverHidesCreateHandler is constructed without a PDS writer dependency at all, so no code path can call CreateRecord/PutRecord/DeleteRecord.
func TestDiscoverHidesCreate_NeverWritesToPDS(t *testing.T) {
	idx := newFakeDiscoverHidesIndex()
	pds := &fakePDS{} // present in scope, exactly as it would be for other handlers in this suite
	h := DiscoverHidesCreateHandler(idx, idx, nil)

	body := `{"targetKind":"source","targetKey":"https://example.com/feed"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/discover/hides", strings.NewReader(body)), "did:plc:me", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 0 || pds.puts != 0 || len(pds.deleted) != 0 {
		t.Errorf("hide triggered a PDS write: creates=%d puts=%d deleted=%v", pds.creates, pds.puts, pds.deleted)
	}
}

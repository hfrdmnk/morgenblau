package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// fakeSavesIndex covers both the reader and writer slices for saves handlers.
type fakeSavesIndex struct {
	mu           sync.Mutex
	byRkey       map[string]map[string]db.UserSave // did → rkey → row
	byItemURL    map[string]map[string]db.UserSave // did → itemUrl → row
	list         []db.ListUserSavesRow
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

func (f *fakeSavesIndex) ListUserSaves(_ context.Context, _ string) ([]db.ListUserSavesRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, nil
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
	h := SavesCreateHandler(idx, idx, pds, &recordingRepair{})

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
	h := SavesCreateHandler(idx, idx, pds, &recordingRepair{})

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
	h := SavesCreateHandler(idx, idx, &fakePDS{}, &recordingRepair{})
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/saves", strings.NewReader(`{"itemUrl":"  "}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSavesCreate_PDSFailure_502(t *testing.T) {
	idx := newFakeSavesIndex()
	h := SavesCreateHandler(idx, idx, failingPDS{}, &recordingRepair{})
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
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, idx, pds, &recordingRepair{}))

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
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, idx, pds, &recordingRepair{}))

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
	mux.Handle("DELETE /api/saves/{rkey}", SavesDeleteHandler(idx, idx, failingPDS{}, &recordingRepair{}))

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
	h := SavesCreateHandler(idx, idx, pds, &recordingRepair{})

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

// --- GET /api/saves ---

// Mirrors the columns ListUserSaves touches from the user_saves and feed_entries migrations; the join semantics are the point, so the rest is omitted.
const savesListSchema = `
CREATE TABLE user_saves (
    did         TEXT NOT NULL,
    rkey        TEXT NOT NULL,
    at_uri      TEXT NOT NULL,
    item_url    TEXT NOT NULL,
    feed_url    TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (did, rkey)
);
CREATE TABLE feed_entries (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_url     TEXT NOT NULL,
    entry_slug   TEXT NOT NULL,
    url          TEXT NOT NULL,
    title        TEXT,
    published_at TEXT NOT NULL
);
`

func openSavesTestDB(t *testing.T) *database.DB {
	t.Helper()
	// A real file (not :memory:): the two pools must share one database.
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), savesListSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

func seedSave(t *testing.T, dbs *database.DB, did, rkey, itemURL string, feedURL *string, createdAt string) {
	t.Helper()
	_, err := dbs.Writer.ExecContext(context.Background(),
		`INSERT INTO user_saves (did, rkey, at_uri, item_url, feed_url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		did, rkey, "at://"+did+"/blue.morgen.feed.save/"+rkey, itemURL, feedURL, createdAt, createdAt)
	if err != nil {
		t.Fatalf("seed save: %v", err)
	}
}

func seedEntry(t *testing.T, dbs *database.DB, feedURL, slug, url, title, publishedAt string) {
	t.Helper()
	_, err := dbs.Writer.ExecContext(context.Background(),
		`INSERT INTO feed_entries (feed_url, entry_slug, url, title, published_at) VALUES (?, ?, ?, ?, ?)`,
		feedURL, slug, url, title, publishedAt)
	if err != nil {
		t.Fatalf("seed entry: %v", err)
	}
}

func listSaves(t *testing.T, dbs *database.DB, did string) []SaveWire {
	t.Helper()
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/saves", nil), did, "sid-1")
	rr := httptest.NewRecorder()
	SavesListHandler(db.New(dbs.Reader)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var got []SaveWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rr.Body.String())
	}
	return got
}

func TestSavesList_NewestFirst_JoinsCachedEntry(t *testing.T) {
	dbs := openSavesTestDB(t)
	feed := "https://blog.example.com/feed.xml"
	seedSave(t, dbs, "did:plc:alice", "3older", "https://blog.example.com/one", &feed, "2026-06-01T00:00:00Z")
	seedSave(t, dbs, "did:plc:alice", "3newer", "https://blog.example.com/two", nil, "2026-06-02T00:00:00Z")
	seedSave(t, dbs, "did:plc:bob", "3bob", "https://blog.example.com/one", nil, "2026-06-03T00:00:00Z")
	seedEntry(t, dbs, feed, "two", "https://blog.example.com/two", "The Second Post", "2026-06-02T00:00:00Z")

	got := listSaves(t, dbs, "did:plc:alice")

	if len(got) != 2 {
		t.Fatalf("saves = %+v, want 2 (bob's save must not leak)", got)
	}
	if got[0].Rkey != "3newer" || got[1].Rkey != "3older" {
		t.Errorf("order = %q, %q; want createdAt DESC", got[0].Rkey, got[1].Rkey)
	}
	if got[0].Title != "The Second Post" || got[0].EntrySlug != "two" || got[0].TargetURL != "https://blog.example.com/two" {
		t.Errorf("joined entry fields = %+v", got[0])
	}
	if got[0].FeedURL != feed {
		t.Errorf("feedUrl = %q, want the entry's feed as fallback for a save without one", got[0].FeedURL)
	}
	if got[1].Title != "" || got[1].EntrySlug != "" || got[1].TargetURL != "" {
		t.Errorf("uncached entry should leave title/slug/target empty: %+v", got[1])
	}
	if got[1].FeedURL != feed {
		t.Errorf("feedUrl = %q, want the save's own feed URL", got[1].FeedURL)
	}
	if got[0].CID != "" {
		t.Errorf("cid = %q, want empty; user_saves has no cid column", got[0].CID)
	}
}

func TestSavesList_DuplicateEntryURLs_OneRowNewestWins(t *testing.T) {
	dbs := openSavesTestDB(t)
	seedSave(t, dbs, "did:plc:alice", "3la", "https://blog.example.com/shared", nil, "2026-06-01T00:00:00Z")
	seedEntry(t, dbs, "https://a.example.com/feed.xml", "shared-old", "https://blog.example.com/shared", "Older Copy", "2026-05-01T00:00:00Z")
	seedEntry(t, dbs, "https://b.example.com/feed.xml", "shared-new", "https://blog.example.com/shared", "Newer Copy", "2026-05-09T00:00:00Z")

	got := listSaves(t, dbs, "did:plc:alice")

	if len(got) != 1 {
		t.Fatalf("saves = %+v, want 1; two entries sharing a url must not fan the save out", got)
	}
	if got[0].Title != "Newer Copy" || got[0].EntrySlug != "shared-new" {
		t.Errorf("joined entry = %+v, want the newest by published_at", got[0])
	}
}

func TestSavesList_NoSaves_EmptyArray(t *testing.T) {
	dbs := openSavesTestDB(t)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/saves", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	SavesListHandler(db.New(dbs.Reader)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if body := strings.TrimSpace(rr.Body.String()); body != "[]" {
		t.Errorf("body = %q, want [] rather than null", body)
	}
}

package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
)

const (
	shareDID     = "did:plc:sharealice"
	docGuid      = "at://did:plc:pub/site.standard.document/3doc"
	shareRSSFeed = "https://blog.example/feed.xml"
	stdPubURI    = "at://did:plc:pub/site.standard.publication/3pub"
)

// fakeShareIndex stubs the reads + writes the shares handlers depend on.
type fakeShareIndex struct {
	entry      db.FeedEntry
	entryErr   error
	sub        db.UserSubscription
	subErr     error
	byItemURL  map[string]db.UserShare
	byDocument map[string]db.UserShare
	byRkey     map[string]db.UserShare
	list       []db.ListUserSharesRow

	upserts []db.UpsertUserShareParams
	deletes []string
}

func newShareIndex() *fakeShareIndex {
	return &fakeShareIndex{
		byItemURL:  map[string]db.UserShare{},
		byDocument: map[string]db.UserShare{},
		byRkey:     map[string]db.UserShare{},
	}
}

func (f *fakeShareIndex) GetFeedEntryBySlug(_ context.Context, _ string) (db.FeedEntry, error) {
	return f.entry, f.entryErr
}

func (f *fakeShareIndex) GetUserSubscriptionByFeedURL(_ context.Context, _ db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error) {
	return f.sub, f.subErr
}

func (f *fakeShareIndex) GetUserShareByItemURL(_ context.Context, arg db.GetUserShareByItemURLParams) (db.UserShare, error) {
	key := ""
	if arg.ItemUrl != nil {
		key = *arg.ItemUrl
	}
	if s, ok := f.byItemURL[key]; ok {
		return s, nil
	}
	return db.UserShare{}, sql.ErrNoRows
}

func (f *fakeShareIndex) GetUserShareByDocument(_ context.Context, arg db.GetUserShareByDocumentParams) (db.UserShare, error) {
	key := ""
	if arg.Document != nil {
		key = *arg.Document
	}
	if s, ok := f.byDocument[key]; ok {
		return s, nil
	}
	return db.UserShare{}, sql.ErrNoRows
}

func (f *fakeShareIndex) GetUserShare(_ context.Context, arg db.GetUserShareParams) (db.UserShare, error) {
	if s, ok := f.byRkey[arg.Rkey]; ok {
		return s, nil
	}
	return db.UserShare{}, sql.ErrNoRows
}

func (f *fakeShareIndex) ListUserShares(_ context.Context, _ string) ([]db.ListUserSharesRow, error) {
	return f.list, nil
}

func (f *fakeShareIndex) UpsertUserShare(_ context.Context, arg db.UpsertUserShareParams) error {
	f.upserts = append(f.upserts, arg)
	return nil
}

func (f *fakeShareIndex) DeleteUserShare(_ context.Context, arg db.DeleteUserShareParams) error {
	f.deletes = append(f.deletes, arg.Rkey)
	return nil
}

func postShare(t *testing.T, idx *fakeShareIndex, pds *fakePDS, standardScope bool, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/shares", strings.NewReader(body))
	if standardScope {
		req = withStandardWriteSession(req, shareDID, "sid-1")
	} else {
		req = withSession(req, shareDID, "sid-1")
	}
	rr := httptest.NewRecorder()
	SharesCreateHandler(idx, idx, pds).ServeHTTP(rr, req)
	return rr
}

func rssEntry() db.FeedEntry {
	return db.FeedEntry{ID: 1, FeedUrl: shareRSSFeed, Guid: "guid-1", EntrySlug: "post", Url: "https://blog.example/post"}
}

func stdEntry(url string) db.FeedEntry {
	return db.FeedEntry{ID: 2, FeedUrl: stdPubURI, Guid: docGuid, EntrySlug: "std-post", Url: url}
}

// --- POST ---

func TestSharesCreate_RSS_SingleRecord(t *testing.T) {
	idx := newShareIndex()
	idx.entry = rssEntry()
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "rss", FeedUrl: shareRSSFeed}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, false, `{"entrySlug":"post"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body)
	}
	if pds.creates != 1 {
		t.Fatalf("creates = %d, want 1", pds.creates)
	}
	if pds.created[0].collection != shareCollection {
		t.Errorf("collection = %q, want %q", pds.created[0].collection, shareCollection)
	}
	if pds.created[0].record["itemUrl"] != "https://blog.example/post" {
		t.Errorf("itemUrl = %v", pds.created[0].record["itemUrl"])
	}
	if len(idx.upserts) != 1 || idx.upserts[0].Kind != "rss" {
		t.Fatalf("upserts = %+v", idx.upserts)
	}
	if idx.upserts[0].Document != nil {
		t.Errorf("rss row must not carry a document: %v", idx.upserts[0].Document)
	}
}

func TestSharesCreate_Standardfeed_RecommendOnly(t *testing.T) {
	idx := newShareIndex()
	idx.entry = stdEntry("https://pub.example/doc")
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "standardfeed", FeedUrl: stdPubURI}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, true, `{"entrySlug":"std-post"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body)
	}
	if pds.creates != 1 {
		t.Fatalf("creates = %d, want 1 (recommend only, no comment)", pds.creates)
	}
	if pds.created[0].collection != recommendCollection {
		t.Errorf("collection = %q, want %q", pds.created[0].collection, recommendCollection)
	}
	if pds.created[0].record["document"] != docGuid {
		t.Errorf("document = %v, want %q", pds.created[0].record["document"], docGuid)
	}
	if len(idx.upserts) != 1 {
		t.Fatalf("upserts = %+v", idx.upserts)
	}
	up := idx.upserts[0]
	if up.Kind != "standardfeed" || up.Document == nil || *up.Document != docGuid {
		t.Errorf("row = %+v", up)
	}
	if up.SidecarRkey != nil {
		t.Errorf("no comment ⇒ no sidecar: %v", up.SidecarRkey)
	}
}

func TestSharesCreate_Standardfeed_RecommendPlusComment(t *testing.T) {
	idx := newShareIndex()
	idx.entry = stdEntry("https://pub.example/doc")
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "standardfeed", FeedUrl: stdPubURI}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, true, `{"entrySlug":"std-post","comment":"loved this"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body)
	}
	if pds.creates != 2 {
		t.Fatalf("creates = %d, want 2 (recommend + sidecar)", pds.creates)
	}
	// Recommend FIRST — its rkey is the local row's rkey.
	if pds.created[0].collection != recommendCollection {
		t.Errorf("first create = %q, want recommend", pds.created[0].collection)
	}
	if pds.created[1].collection != shareCollection {
		t.Errorf("second create = %q, want share sidecar", pds.created[1].collection)
	}
	if pds.created[1].record["comment"] != "loved this" || pds.created[1].record["document"] != docGuid {
		t.Errorf("sidecar record = %+v", pds.created[1].record)
	}
	up := idx.upserts[0]
	if up.Rkey != "3la1" {
		t.Errorf("local rkey = %q, want the recommend rkey 3la1", up.Rkey)
	}
	if up.SidecarRkey == nil || *up.SidecarRkey != "3la2" {
		t.Errorf("sidecarRkey = %v, want the share rkey 3la2", up.SidecarRkey)
	}
	if up.Comment == nil || *up.Comment != "loved this" {
		t.Errorf("comment = %v", up.Comment)
	}
}

func TestSharesCreate_Standardfeed_PathlessWithComment_422(t *testing.T) {
	idx := newShareIndex()
	idx.entry = stdEntry("") // path-less document
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "standardfeed", FeedUrl: stdPubURI}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, true, `{"entrySlug":"std-post","comment":"cannot comment"}`)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rr.Code)
	}
	if pds.creates != 0 || len(idx.upserts) != 0 {
		t.Errorf("path-less+comment must write nothing: creates=%d upserts=%d", pds.creates, len(idx.upserts))
	}
}

func TestSharesCreate_Standardfeed_PathlessNoComment_RecommendOK(t *testing.T) {
	idx := newShareIndex()
	idx.entry = stdEntry("") // path-less, but no comment ⇒ recommend allowed
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "standardfeed", FeedUrl: stdPubURI}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, true, `{"entrySlug":"std-post"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body)
	}
	if pds.creates != 1 || pds.created[0].collection != recommendCollection {
		t.Fatalf("want a single recommend create: %+v", pds.created)
	}
	if idx.upserts[0].ItemUrl != nil {
		t.Errorf("path-less row itemUrl must be nil: %v", idx.upserts[0].ItemUrl)
	}
}

func TestSharesCreate_Standardfeed_StaleScope_403(t *testing.T) {
	idx := newShareIndex()
	idx.entry = stdEntry("https://pub.example/doc")
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "standardfeed", FeedUrl: stdPubURI}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, false, `{"entrySlug":"std-post"}`) // no standard scope

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "reauth_required") {
		t.Errorf("body = %q, want reauth_required", rr.Body.String())
	}
	if pds.creates != 0 || len(idx.upserts) != 0 {
		t.Errorf("stale scope must write nothing: creates=%d upserts=%d", pds.creates, len(idx.upserts))
	}
}

func TestSharesCreate_Idempotent_NoPDSHit(t *testing.T) {
	idx := newShareIndex()
	idx.entry = rssEntry()
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "rss", FeedUrl: shareRSSFeed}
	idx.byItemURL["https://blog.example/post"] = db.UserShare{
		Did: shareDID, Rkey: "existing", Kind: "rss", ItemUrl: ptrString("https://blog.example/post"),
	}
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, false, `{"entrySlug":"post"}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (idempotent)", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("idempotent re-share hit the PDS: creates=%d", pds.creates)
	}
	if !strings.Contains(rr.Body.String(), "existing") {
		t.Errorf("body = %q, want existing rkey", rr.Body.String())
	}
}

func TestSharesCreate_NotSubscribed_403(t *testing.T) {
	idx := newShareIndex()
	idx.entry = rssEntry()
	idx.subErr = sql.ErrNoRows
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, false, `{"entrySlug":"post"}`)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("unauthorized share hit the PDS")
	}
}

func TestSharesCreate_UnknownEntry_404(t *testing.T) {
	idx := newShareIndex()
	idx.entryErr = sql.ErrNoRows
	pds := &fakePDS{}

	rr := postShare(t, idx, pds, false, `{"entrySlug":"missing"}`)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestSharesCreate_CommentTooLong_400(t *testing.T) {
	idx := newShareIndex()
	idx.entry = stdEntry("https://pub.example/doc")
	idx.sub = db.UserSubscription{Did: shareDID, Kind: "standardfeed", FeedUrl: stdPubURI}
	pds := &fakePDS{}

	long := strings.Repeat("x", 3001)
	rr := postShare(t, idx, pds, true, `{"entrySlug":"std-post","comment":"`+long+`"}`)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("over-long comment hit the PDS")
	}
}

// --- DELETE ---

func deleteShare(t *testing.T, idx *fakeShareIndex, pds *fakePDS, standardScope bool, rkey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/shares/"+rkey, nil)
	if standardScope {
		req = withStandardWriteSession(req, shareDID, "sid-1")
	} else {
		req = withSession(req, shareDID, "sid-1")
	}
	req.SetPathValue("rkey", rkey)
	rr := httptest.NewRecorder()
	SharesDeleteHandler(idx, idx, pds).ServeHTTP(rr, req)
	return rr
}

func TestSharesDelete_RSS(t *testing.T) {
	idx := newShareIndex()
	idx.byRkey["3rss"] = db.UserShare{Did: shareDID, Rkey: "3rss", Kind: "rss", ItemUrl: ptrString("https://x/post")}
	pds := &fakePDS{}

	rr := deleteShare(t, idx, pds, false, "3rss")

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if len(pds.deleted) != 1 || pds.deleted[0] != shareCollection+"/3rss" {
		t.Errorf("PDS deletes = %v", pds.deleted)
	}
	if len(idx.deletes) != 1 || idx.deletes[0] != "3rss" {
		t.Errorf("local deletes = %v", idx.deletes)
	}
}

func TestSharesDelete_Standardfeed_RecommendAndSidecar(t *testing.T) {
	idx := newShareIndex()
	idx.byRkey["3rec"] = db.UserShare{
		Did: shareDID, Rkey: "3rec", Kind: "standardfeed",
		Document: ptrString(docGuid), SidecarRkey: ptrString("3sc"),
	}
	pds := &fakePDS{}

	rr := deleteShare(t, idx, pds, true, "3rec")

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	// Recommend deleted, plus the comment sidecar.
	if len(pds.deleted) != 2 {
		t.Fatalf("PDS deletes = %v, want recommend + sidecar", pds.deleted)
	}
	if pds.deleted[0] != recommendCollection+"/3rec" {
		t.Errorf("first delete = %q, want the recommend", pds.deleted[0])
	}
	if pds.deleted[1] != shareCollection+"/3sc" {
		t.Errorf("second delete = %q, want the sidecar", pds.deleted[1])
	}
	if len(idx.deletes) != 1 || idx.deletes[0] != "3rec" {
		t.Errorf("local deletes = %v", idx.deletes)
	}
}

func TestSharesDelete_Standardfeed_StaleScope_403(t *testing.T) {
	idx := newShareIndex()
	idx.byRkey["3rec"] = db.UserShare{Did: shareDID, Rkey: "3rec", Kind: "standardfeed", Document: ptrString(docGuid)}
	pds := &fakePDS{}

	rr := deleteShare(t, idx, pds, false, "3rec") // no standard scope

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if len(pds.deleted) != 0 || len(idx.deletes) != 0 {
		t.Errorf("stale scope must delete nothing")
	}
}

func TestSharesDelete_NotFound_403(t *testing.T) {
	idx := newShareIndex()
	pds := &fakePDS{}

	rr := deleteShare(t, idx, pds, false, "ghost")

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (collapsed with not-yours)", rr.Code)
	}
}

// --- GET ---

func TestSharesList(t *testing.T) {
	idx := newShareIndex()
	idx.list = []db.ListUserSharesRow{
		{Rkey: "3a", Kind: "standardfeed", Document: ptrString(docGuid), Comment: ptrString("nice"),
			CreatedAt: "2026-06-02T00:00:00Z", EntryTitle: ptrString("A Title"), EntrySlug: ptrString("a-title")},
		{Rkey: "3b", Kind: "rss", ItemUrl: ptrString("https://x/post"), CreatedAt: "2026-06-01T00:00:00Z"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/shares", nil)
	req = withSession(req, shareDID, "sid-1")
	rr := httptest.NewRecorder()
	SharesListHandler(idx).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "A Title") || !strings.Contains(body, "a-title") {
		t.Errorf("body missing entry join fields: %q", body)
	}
	if !strings.Contains(body, "https://x/post") {
		t.Errorf("body missing rss itemUrl: %q", body)
	}
}

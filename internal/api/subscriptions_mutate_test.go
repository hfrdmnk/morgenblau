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

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
)

// rkeyIndex wraps fakeIndex with the rkey-keyed lookup PATCH/DELETE need.
type rkeyIndex struct {
	*fakeIndex
	byRkey  map[string]map[string]db.UserSubscription // did → rkey → row
	delMu   sync.Mutex
	deleted []string
}

func newRkeyIndex() *rkeyIndex {
	return &rkeyIndex{fakeIndex: newFakeIndex(), byRkey: map[string]map[string]db.UserSubscription{}}
}

func (r *rkeyIndex) GetUserSubscription(_ context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error) {
	if rows, ok := r.byRkey[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			return row, nil
		}
	}
	return db.UserSubscription{}, sql.ErrNoRows
}

func (r *rkeyIndex) DeleteUserSubscription(_ context.Context, arg db.DeleteUserSubscriptionParams) error {
	r.delMu.Lock()
	defer r.delMu.Unlock()
	r.deleted = append(r.deleted, arg.Did+":"+arg.Rkey)
	if rows, ok := r.byRkey[arg.Did]; ok {
		delete(rows, arg.Rkey)
	}
	return nil
}

func (r *rkeyIndex) seed(did, rkey, feedURL string) {
	if _, ok := r.byRkey[did]; !ok {
		r.byRkey[did] = map[string]db.UserSubscription{}
	}
	row := db.UserSubscription{
		Did:       did,
		Rkey:      rkey,
		AtUri:     "at://" + did + "/blue.morgen.feed.subscription/" + rkey,
		FeedUrl:   feedURL,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	}
	r.byRkey[did][rkey] = row
}

func (r *rkeyIndex) seedRow(row db.UserSubscription) {
	if _, ok := r.byRkey[row.Did]; !ok {
		r.byRkey[row.Did] = map[string]db.UserSubscription{}
	}
	r.byRkey[row.Did][row.Rkey] = row
}

func TestSubscriptionsPatch_NoDiff_NoOp(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")

	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.creates != 0 {
		t.Errorf("PDS create called on no-diff: %d", pds.creates)
	}
}

func TestSubscriptionsPatch_OtherUserRkey_404(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:bob", "3la", "https://x")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"new"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (collapsed with not-owned)", rr.Code)
	}
}

func TestSubscriptionsPatch_Title_Applied(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   "https://example.test/feed.xml",
		Title:     ptrString("Old Title"),
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"New Title"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "New Title" {
		t.Errorf("response title = %q", got.Title)
	}
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did:     "did:plc:alice",
		FeedUrl: "https://example.test/feed.xml",
	})
	if err != nil {
		t.Fatalf("GetUserSubscriptionByFeedURL: %v", err)
	}
	if row.Title == nil || *row.Title != "New Title" {
		t.Errorf("stored title = %v", row.Title)
	}
}

func TestSubscriptionsPatch_RSS_PreservesSiteURL(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   "https://example.test/feed.xml",
		Title:     ptrString("Old Title"),
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	idx.siteURLs["https://example.test/feed.xml"] = ptrString("https://example.test")

	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"New Title"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	source, ok := pds.lastPut["source"].(map[string]any)
	if !ok {
		t.Fatalf("no source in put record: %+v", pds.lastPut)
	}
	if source["siteUrl"] != "https://example.test" {
		t.Errorf("PATCH dropped siteUrl: source = %+v", source)
	}
}

func TestSubscriptionsPatch_PrimaryAndTags_Applied(t *testing.T) {
	idx := newRkeyIndex()
	oldTags := `["News"]`
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   "https://example.test/feed.xml",
		IsPrimary: 1,
		Tags:      &oldTags,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	// primary true→false and tags edited.
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"primary":false,"tags":["Tech","Design"]}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.puts != 1 {
		t.Fatalf("PDS puts = %d, want 1", pds.puts)
	}
	// primary==false must be absent from the record map (minimal PDS shape).
	if _, ok := pds.lastPut["primary"]; ok {
		t.Errorf("primary should be omitted when false: %v", pds.lastPut["primary"])
	}
	if tags, ok := pds.lastPut["tags"].([]string); !ok || len(tags) != 2 || tags[0] != "Tech" {
		t.Errorf("put tags = %v", pds.lastPut["tags"])
	}

	var got SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Primary {
		t.Errorf("response primary = true, want false")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "Tech" {
		t.Errorf("response tags = %v", got.Tags)
	}
}

func TestSubscriptionsPatch_PrimaryTrue_WrittenToRecord(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   "https://example.test/feed.xml",
		IsPrimary: 0,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"primary":true}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.lastPut["primary"] != true {
		t.Errorf("put primary = %v, want true", pds.lastPut["primary"])
	}
}

func TestSubscriptionsPatch_SamePrimaryAndTags_NoOp(t *testing.T) {
	idx := newRkeyIndex()
	tags := `["News","Tech"]`
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   "https://example.test/feed.xml",
		IsPrimary: 1,
		Tags:      &tags,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	// Tags are order-sensitive, so this resubmits the same order to assert a no-op.
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"primary":true,"tags":["News","Tech"]}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.puts != 0 {
		t.Errorf("PDS put fired on no-diff primary+tags: %d", pds.puts)
	}
}

func TestSubscriptionsDelete_HappyPath_204(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if len(idx.deleted) != 1 {
		t.Errorf("deleted = %v", idx.deleted)
	}
	// The rss path never lists the standard collection.
	if pds.listCalls != 0 {
		t.Errorf("ListRecords called %d times on rss delete", pds.listCalls)
	}
}

func TestSubscriptionsDelete_OtherUserRkey_404(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:bob", "3la", "https://x")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (collapsed with not-owned)", rr.Code)
	}
}

func TestSubscriptionsDelete_PDSFailure_502(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, failingPDS{}))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if len(idx.deleted) != 0 {
		t.Errorf("deleted despite PDS failure: %v", idx.deleted)
	}
}

// --- standardfeed PATCH/DELETE ---

// seedStandardRow returns a Tier-1 row for a standardfeed subscription; the rkey and at-uri are the standard record's, not a sidecar's.
func seedStandardRow(sidecarRkey *string) db.UserSubscription {
	return db.UserSubscription{
		Did:         "did:plc:alice",
		Rkey:        "3std",
		AtUri:       "at://did:plc:alice/" + standardSubCollection + "/3std",
		FeedUrl:     testPublication,
		Kind:        "standardfeed",
		SidecarRkey: sidecarRkey,
		CreatedAt:   "2026-06-01T10:00:00Z",
		UpdatedAt:   "2026-06-01T10:00:00Z",
	}
}

func TestSubscriptionsPatch_Standardfeed_RejectsFeedURL_400(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(seedStandardRow(nil))
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3std",
		strings.NewReader(`{"feedUrl":"https://example.test/feed.xml"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
	if pds.puts != 0 || pds.creates != 0 {
		t.Errorf("PDS touched on rejected patch: puts=%d creates=%d", pds.puts, pds.creates)
	}
}

// A metadata edit lazily creates the blue.morgen sidecar under the old grant, no new scope needed, so this runs on a scope-less session on purpose.
func TestSubscriptionsPatch_Standardfeed_FirstEdit_CreatesSidecar(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(seedStandardRow(nil))
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3std",
		strings.NewReader(`{"title":"Custom"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.puts != 0 {
		t.Errorf("PDS puts = %d, want 0 (no sidecar existed)", pds.puts)
	}
	if pds.creates != 1 {
		t.Fatalf("PDS creates = %d, want 1 sidecar", pds.creates)
	}
	if pds.created[0].collection != "blue.morgen.feed.subscription" {
		t.Errorf("sidecar collection = %q", pds.created[0].collection)
	}
	rec := pds.created[0].record
	source, ok := rec["source"].(map[string]any)
	if !ok || source["$type"] != "blue.morgen.feed.subscription#standardPublication" || source["publication"] != testPublication {
		t.Errorf("sidecar source = %v", rec["source"])
	}
	if rec["title"] != "Custom" {
		t.Errorf("sidecar title = %v", rec["title"])
	}

	// The Tier-1 row keeps the standard identity and gains the sidecar rkey.
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did: "did:plc:alice", FeedUrl: testPublication,
	})
	if err != nil {
		t.Fatalf("row lookup: %v", err)
	}
	if row.Rkey != "3std" || row.AtUri != "at://did:plc:alice/"+standardSubCollection+"/3std" {
		t.Errorf("standard identity changed: rkey=%q atUri=%q", row.Rkey, row.AtUri)
	}
	if row.SidecarRkey == nil || *row.SidecarRkey != "3la1" {
		t.Errorf("sidecar_rkey = %v, want 3la1", row.SidecarRkey)
	}
	if row.Kind != "standardfeed" {
		t.Errorf("kind = %q", row.Kind)
	}

	var got SubscriptionWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "standardfeed" || got.Title != "Custom" {
		t.Errorf("wire = %+v", got)
	}
}

func TestSubscriptionsPatch_Standardfeed_SecondEdit_PutsExistingSidecar(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(seedStandardRow(ptrString("3sc")))
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3std",
		strings.NewReader(`{"title":"Renamed"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.creates != 0 {
		t.Errorf("PDS creates = %d, want 0 (sidecar already exists)", pds.creates)
	}
	if pds.puts != 1 || pds.lastPutRkey != "3sc" {
		t.Fatalf("PDS puts = %d rkey %q, want 1 put at 3sc", pds.puts, pds.lastPutRkey)
	}
	if pds.lastPut["title"] != "Renamed" {
		t.Errorf("put title = %v", pds.lastPut["title"])
	}

	row, _ := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did: "did:plc:alice", FeedUrl: testPublication,
	})
	if row.SidecarRkey == nil || *row.SidecarRkey != "3sc" {
		t.Errorf("sidecar_rkey = %v, want preserved 3sc", row.SidecarRkey)
	}
}

func TestSubscriptionsDelete_Standardfeed_SweepsDuplicatesAndSidecar(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(seedStandardRow(ptrString("3sc")))
	pds := &fakePDS{listed: map[string][]atprepo.ListedRecord{
		standardSubCollection: {
			{URI: "at://did:plc:alice/" + standardSubCollection + "/3std", Value: map[string]any{"publication": testPublication}},
			// A duplicate written by another app; must be swept too.
			{URI: "at://did:plc:alice/" + standardSubCollection + "/3dup", Value: map[string]any{"publication": testPublication}},
			// A different publication; must survive.
			{URI: "at://did:plc:alice/" + standardSubCollection + "/3other", Value: map[string]any{"publication": "at://did:plc:other/site.standard.publication/3x"}},
		},
	}}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withStandardWriteSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3std", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	want := []string{
		standardSubCollection + "/3std",
		standardSubCollection + "/3dup",
		"blue.morgen.feed.subscription/3sc",
	}
	if len(pds.deleted) != len(want) {
		t.Fatalf("deleted = %v, want %v", pds.deleted, want)
	}
	for i := range want {
		if pds.deleted[i] != want[i] {
			t.Errorf("deleted[%d] = %q, want %q", i, pds.deleted[i], want[i])
		}
	}
	if len(idx.deleted) != 1 {
		t.Errorf("local deletes = %v", idx.deleted)
	}
}

func TestSubscriptionsDelete_Standardfeed_StaleScope_403(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(seedStandardRow(nil))
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3std", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "reauth_required") {
		t.Errorf("body = %q, want reauth_required code", rr.Body.String())
	}
	if pds.listCalls != 0 || len(pds.deleted) != 0 || len(idx.deleted) != 0 {
		t.Errorf("stale-scope delete touched state: lists=%d pds=%v local=%v", pds.listCalls, pds.deleted, idx.deleted)
	}
}

func TestSubscriptionsDelete_Standardfeed_ListFailure_502(t *testing.T) {
	idx := newRkeyIndex()
	idx.seedRow(seedStandardRow(nil))
	pds := &fakePDS{listErr: errors.New("pds down")}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withStandardWriteSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3std", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	if len(pds.deleted) != 0 || len(idx.deleted) != 0 {
		t.Errorf("deletes despite list failure: pds=%v local=%v", pds.deleted, idx.deleted)
	}
}

// The YouTube "exclude Shorts" toggle re-points a subscription to a different feed URL on edit.
func TestSubscriptionsPatch_FeedURLChange_RepointsAndDispatches(t *testing.T) {
	const channel = "https://www.youtube.com/feeds/videos.xml?channel_id=UCabc"
	const playlist = "https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabc"
	idx := newRkeyIndex()
	tags := `["Tech"]`
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   channel,
		Title:     ptrString("Some Channel"),
		IsPrimary: 1,
		Tags:      &tags,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, disp))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"feedUrl":"`+playlist+`"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if pds.puts != 1 {
		t.Fatalf("PDS puts = %d, want 1", pds.puts)
	}
	putSource, ok := pds.lastPut["source"].(map[string]any)
	if !ok || putSource["$type"] != "blue.morgen.feed.subscription#rssFeed" || putSource["feedUrl"] != playlist {
		t.Errorf("put source = %v, want rssFeed variant with feedUrl %s", pds.lastPut["source"], playlist)
	}
	// Tier-2 catalog upsert created the new feed before Tier-1 referenced it.
	if len(idx.upsertedFeeds) != 1 || idx.upsertedFeeds[0] != playlist {
		t.Errorf("UpsertFeed = %v, want [%s]", idx.upsertedFeeds, playlist)
	}
	// fetch_one_feed dispatched exactly once, for the new feed.
	if len(disp.dispatched) != 1 || disp.dispatched[0] != playlist {
		t.Errorf("dispatched = %v, want [%s]", disp.dispatched, playlist)
	}
	// Tier-1 row is re-keyed to the new feed with metadata preserved.
	row, err := idx.GetUserSubscriptionByFeedURL(context.Background(), db.GetUserSubscriptionByFeedURLParams{
		Did:     "did:plc:alice",
		FeedUrl: playlist,
	})
	if err != nil {
		t.Fatalf("GetUserSubscriptionByFeedURL(new): %v", err)
	}
	if row.Rkey != "3la" {
		t.Errorf("rkey = %q, want 3la (same record, re-pointed)", row.Rkey)
	}
	if row.Title == nil || *row.Title != "Some Channel" || row.IsPrimary != 1 {
		t.Errorf("metadata not preserved: title=%v primary=%d", row.Title, row.IsPrimary)
	}
	// Response carries the new feed URL and the dispatched job id.
	var got patchResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.FeedURL != playlist {
		t.Errorf("response feedUrl = %q, want %s", got.FeedURL, playlist)
	}
	if got.JobID == "" {
		t.Errorf("response jobId empty, want a dispatched id")
	}
}

func TestSubscriptionsPatch_FeedURLChange_Invalid_400(t *testing.T) {
	const feed = "https://example.test/feed.xml"
	cases := map[string]string{
		"not a url":       "not a url",
		"javascript":      "javascript:alert(1)",
		"scheme only":     "https://",
		"relative":        "/feeds/videos.xml",
		"unsupported ftp": "ftp://example.test/feed.xml",
	}
	for name, candidate := range cases {
		t.Run(name, func(t *testing.T) {
			idx := newRkeyIndex()
			idx.seed("did:plc:alice", "3la", feed)
			pds := &fakePDS{}
			disp := &fakeDispatcher{}
			mux := http.NewServeMux()
			mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, disp))

			body, _ := json.Marshal(map[string]string{"feedUrl": candidate})
			req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
				strings.NewReader(string(body))), "did:plc:alice", "sid-1")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rr.Code, rr.Body.String())
			}
			// A rejected re-point must not touch the PDS or the catalog.
			if pds.puts != 0 {
				t.Errorf("PDS puts = %d, want 0 on rejected feedUrl", pds.puts)
			}
			if len(idx.upsertedFeeds) != 0 {
				t.Errorf("UpsertFeed called %d times, want 0", len(idx.upsertedFeeds))
			}
			if len(disp.dispatched) != 0 {
				t.Errorf("dispatched %v, want none", disp.dispatched)
			}
		})
	}
}

func TestSubscriptionsPatch_FeedURLUnchanged_NoDispatch(t *testing.T) {
	const feed = "https://example.test/feed.xml"
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", feed)
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, disp))

	// Resubmitting the same feed URL is a no-op: no PDS write, no fetch.
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"feedUrl":"`+feed+`"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if pds.puts != 0 {
		t.Errorf("PDS put on unchanged feedUrl: %d", pds.puts)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatch on unchanged feedUrl: %v", disp.dispatched)
	}
	if len(idx.upsertedFeeds) != 0 {
		t.Errorf("UpsertFeed on unchanged feedUrl: %v", idx.upsertedFeeds)
	}
}

func TestSubscriptionsPatch_FeedURLChange_Conflict_409(t *testing.T) {
	const channel = "https://www.youtube.com/feeds/videos.xml?channel_id=UCabc"
	const playlist = "https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabc"
	idx := newRkeyIndex()
	idx.seedRow(db.UserSubscription{
		Did:       "did:plc:alice",
		Rkey:      "3la",
		AtUri:     "at://did:plc:alice/blue.morgen.feed.subscription/3la",
		FeedUrl:   channel,
		CreatedAt: "2026-05-15T10:00:00Z",
		UpdatedAt: "2026-05-15T10:00:00Z",
	})
	// Alice already subscribes to the target feed under a different rkey.
	idx.fakeIndex.rows["did:plc:alice"] = map[string]db.UserSubscription{
		playlist: {Did: "did:plc:alice", Rkey: "3other", FeedUrl: playlist},
	}
	pds := &fakePDS{}
	disp := &fakeDispatcher{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, disp))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"feedUrl":"`+playlist+`"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rr.Code, rr.Body.String())
	}
	if pds.puts != 0 {
		t.Errorf("PDS put despite conflict: %d", pds.puts)
	}
	if len(disp.dispatched) != 0 {
		t.Errorf("dispatch despite conflict: %v", disp.dispatched)
	}
}

// TestSubscriptionsPatch_InvalidRecord_500_NoWrite proves an over-cap title (>128 graphemes) is rejected before the PDS put, not merely logged.
func TestSubscriptionsPatch_InvalidRecord_500_NoWrite(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:alice", "3la", "https://example.test/feed.xml")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	overlong := strings.Repeat("x", 200) // > maxGraphemes:128
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"`+overlong+`"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_record") {
		t.Errorf("body = %q, want invalid_record code", rr.Body.String())
	}
	if pds.puts != 0 {
		t.Errorf("PDS puts = %d, want 0 (validation must run before the write)", pds.puts)
	}
}

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

func TestSubscriptionsPatch_OtherUserRkey_403(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:bob", "3la", "https://x")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("PATCH /api/subscriptions/{rkey}", SubscriptionsPatchHandler(idx, idx.fakeIndex, pds, &fakeDispatcher{}))

	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/subscriptions/3la",
		strings.NewReader(`{"title":"new"}`)), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
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

	// Resubmit identical values (tags in different order should normalize equal? No —
	// order is significant for tags; we resubmit the SAME order to assert no-op).
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
}

func TestSubscriptionsDelete_OtherUserRkey_403(t *testing.T) {
	idx := newRkeyIndex()
	idx.seed("did:plc:bob", "3la", "https://x")
	pds := &fakePDS{}
	mux := http.NewServeMux()
	mux.Handle("DELETE /api/subscriptions/{rkey}", SubscriptionsDeleteHandler(idx, idx, pds))

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
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

// Re-pointing a subscription to a different feed (the YouTube "exclude Shorts"
// toggle on edit) must rewrite the PDS record, create the Tier-2 catalog row,
// re-key the Tier-1 row, and dispatch a fetch for the new feed — preserving the
// existing title/primary/tags.
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
	if pds.lastPut["feedUrl"] != playlist {
		t.Errorf("put feedUrl = %v, want %s", pds.lastPut["feedUrl"], playlist)
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

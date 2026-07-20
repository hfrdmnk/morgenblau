package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/database/db"
)

// --- Source detail (GET /api/subscriptions/{rkey}) ---

type fakeSourceDetail struct {
	rows    map[string]map[string]db.GetUserSourceWithStatsRow // did → rkey → row
	subRows map[string]map[string]db.UserSubscription          // did → rkey → row
	entries map[string][]db.ListEntriesForSourceRow            // feed_url → rows
	err     error
}

func newFakeSourceDetail() *fakeSourceDetail {
	return &fakeSourceDetail{
		rows:    map[string]map[string]db.GetUserSourceWithStatsRow{},
		subRows: map[string]map[string]db.UserSubscription{},
		entries: map[string][]db.ListEntriesForSourceRow{},
	}
}

func (f *fakeSourceDetail) GetUserSourceWithStats(_ context.Context, arg db.GetUserSourceWithStatsParams) (db.GetUserSourceWithStatsRow, error) {
	if f.err != nil {
		return db.GetUserSourceWithStatsRow{}, f.err
	}
	if rows, ok := f.rows[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			return row, nil
		}
	}
	return db.GetUserSourceWithStatsRow{}, sql.ErrNoRows
}

func (f *fakeSourceDetail) GetUserSubscription(_ context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error) {
	if rows, ok := f.subRows[arg.Did]; ok {
		if row, ok := rows[arg.Rkey]; ok {
			return row, nil
		}
	}
	return db.UserSubscription{}, sql.ErrNoRows
}

func (f *fakeSourceDetail) ListEntriesForSource(_ context.Context, arg db.ListEntriesForSourceParams) ([]db.ListEntriesForSourceRow, error) {
	rows := f.entries[arg.FeedUrl]
	if int64(len(rows)) > arg.Limit {
		rows = rows[:arg.Limit]
	}
	return rows, nil
}

func TestSubscriptionGet_HappyPath(t *testing.T) {
	fake := newFakeSourceDetail()
	title := "Example Feed"
	site := "https://example.test"
	icon := "https://example.test/favicon.ico"
	tagsJSON := `["Tech","Design"]`
	fake.rows["did:plc:alice"] = map[string]db.GetUserSourceWithStatsRow{
		"3la": {
			Did:              "did:plc:alice",
			Rkey:             "3la",
			AtUri:            "at://did:plc:alice/blue.morgen.feed.subscription/3la",
			FeedUrl:          "https://example.test/feed.xml",
			Title:            &title,
			SiteUrl:          &site,
			IconUrl:          &icon,
			IsPrimary:        1,
			Tags:             &tagsJSON,
			LastPublishedAt:  "2026-05-20T10:00:00Z",
			FirstPublishedAt: "2024-01-01T00:00:00Z",
			Count7d:          5,
			Count28d:         15,
			Count56d:         30,
			Count84d:         45,
			TotalEntries:     200,
			SavedByYou:       7,
		},
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}", SubscriptionGetHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got SubscriptionDetailWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Rkey != "3la" {
		t.Errorf("rkey = %q", got.Rkey)
	}
	if got.FeedURL != "https://example.test/feed.xml" {
		t.Errorf("feedUrl = %q", got.FeedURL)
	}
	if got.Title != title {
		t.Errorf("title = %q", got.Title)
	}
	if got.FaviconURL != icon {
		t.Errorf("favicon = %q", got.FaviconURL)
	}
	if got.LastPublishedAt != "2026-05-20T10:00:00Z" {
		t.Errorf("lastPublishedAt = %q", got.LastPublishedAt)
	}
	if got.Frequency == "" {
		t.Errorf("frequency unset")
	}
	if got.TotalEntries != 200 {
		t.Errorf("totalEntries = %d, want 200", got.TotalEntries)
	}
	if got.SavedByYou != 7 {
		t.Errorf("savedByYou = %d, want 7", got.SavedByYou)
	}
	// primary/tags feed the edit-dialog prefill on the detail page.
	if !got.Primary {
		t.Errorf("primary = false, want true")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "Tech" || got.Tags[1] != "Design" {
		t.Errorf("tags = %v, want [Tech Design]", got.Tags)
	}
}

func TestSubscriptionGet_NotFound_404(t *testing.T) {
	fake := newFakeSourceDetail()
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}", SubscriptionGetHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/missing", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestSubscriptionGet_ForeignRkey_404(t *testing.T) {
	fake := newFakeSourceDetail()
	bobTitle := "Bob's Feed"
	fake.rows["did:plc:bob"] = map[string]db.GetUserSourceWithStatsRow{
		"3la": {Did: "did:plc:bob", Rkey: "3la", FeedUrl: "https://bob.test/feed", Title: &bobTitle},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}", SubscriptionGetHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/3la", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// --- Source entries (GET /api/subscriptions/{rkey}/entries) ---

func TestSubscriptionEntries_HappyPath(t *testing.T) {
	fake := newFakeSourceDetail()
	feed := "https://example.test/feed.xml"
	fake.subRows["did:plc:alice"] = map[string]db.UserSubscription{
		"3la": {Did: "did:plc:alice", Rkey: "3la", FeedUrl: feed},
	}
	subTitle := "Example"
	site := "https://example.test"
	icon := "https://example.test/favicon.ico"
	t1 := "Newer post"
	t2 := "Older post"
	fake.entries[feed] = []db.ListEntriesForSourceRow{
		{
			ID: 2, FeedUrl: feed, EntrySlug: "newer", Url: "https://example.test/2",
			Title: &t1, ContentType: "text", PublishedAt: "2026-05-20T10:00:00Z",
			FeedTitle: &subTitle, FeedSiteUrl: &site, FeedIconUrl: &icon,
		},
		{
			ID: 1, FeedUrl: feed, EntrySlug: "older", Url: "https://example.test/1",
			Title: &t2, ContentType: "text", PublishedAt: "2026-05-15T10:00:00Z",
			FeedTitle: &subTitle, FeedSiteUrl: &site, FeedIconUrl: &icon,
		},
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}/entries", SubscriptionEntriesHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/3la/entries", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []EntryWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2", len(got))
	}
	if got[0].EntrySlug != "newer" {
		t.Errorf("first entry = %q, want newer", got[0].EntrySlug)
	}
	if got[0].Source.FaviconURL == nil || *got[0].Source.FaviconURL != icon {
		t.Errorf("favicon = %v", got[0].Source.FaviconURL)
	}
}

func TestSubscriptionEntries_NotSubscribed_404(t *testing.T) {
	fake := newFakeSourceDetail()
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}/entries", SubscriptionEntriesHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/3la/entries", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestSubscriptionEntries_ForeignRkey_404(t *testing.T) {
	fake := newFakeSourceDetail()
	fake.subRows["did:plc:bob"] = map[string]db.UserSubscription{
		"3la": {Did: "did:plc:bob", Rkey: "3la", FeedUrl: "https://bob.test/feed"},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}/entries", SubscriptionEntriesHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/3la/entries", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestSubscriptionEntries_LimitHonored(t *testing.T) {
	fake := newFakeSourceDetail()
	feed := "https://example.test/feed.xml"
	fake.subRows["did:plc:alice"] = map[string]db.UserSubscription{
		"3la": {Did: "did:plc:alice", Rkey: "3la", FeedUrl: feed},
	}
	// Stub 250 entries; the handler should ask for at most 200.
	rows := make([]db.ListEntriesForSourceRow, 250)
	for i := range rows {
		title := "x"
		rows[i] = db.ListEntriesForSourceRow{
			ID: int64(i), FeedUrl: feed, EntrySlug: "s", Url: "u",
			Title: &title, ContentType: "text", PublishedAt: "2026-05-20T10:00:00Z",
		}
	}
	fake.entries[feed] = rows

	mux := http.NewServeMux()
	mux.Handle("GET /api/subscriptions/{rkey}/entries", SubscriptionEntriesHandler(fake))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/subscriptions/3la/entries", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var got []EntryWire
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got) != 200 {
		t.Errorf("entries = %d, want 200", len(got))
	}
}

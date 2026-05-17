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

type fakeEntryReader struct {
	entry        db.FeedEntry
	getErr       error
	subOK        bool
	updatedID    int64
	updatedBody  *string
	subscription db.UserSubscription
	feed         db.Feed
	feedErr      error
}

func (f *fakeEntryReader) GetFeedEntryBySlug(_ context.Context, _ string) (db.FeedEntry, error) {
	if f.getErr != nil {
		return db.FeedEntry{}, f.getErr
	}
	return f.entry, nil
}

func (f *fakeEntryReader) GetUserSubscriptionByFeedURL(_ context.Context, _ db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error) {
	if !f.subOK {
		return db.UserSubscription{}, sql.ErrNoRows
	}
	return f.subscription, nil
}

func (f *fakeEntryReader) GetFeed(_ context.Context, _ string) (db.Feed, error) {
	if f.feedErr != nil {
		return db.Feed{}, f.feedErr
	}
	return f.feed, nil
}

func (f *fakeEntryReader) UpdateFeedEntryExtractedBody(_ context.Context, arg db.UpdateFeedEntryExtractedBodyParams) error {
	f.updatedID = arg.ID
	f.updatedBody = arg.ExtractedBody
	return nil
}

func entryFixture() db.FeedEntry {
	t := "Hello"
	body := "<p>Hello world</p>"
	return db.FeedEntry{
		ID:          42,
		FeedUrl:     "https://example.test/feed.xml",
		Url:         "https://example.test/post",
		Title:       &t,
		ContentHtml: &body,
		ContentType: "blogpost",
		PublishedAt: "2026-05-15T10:00:00Z",
	}
}

func subscriptionFixture(canonical, custom *string) db.UserSubscription {
	return db.UserSubscription{
		Did:         "did:plc:alice",
		FeedUrl:     "https://example.test/feed.xml",
		Title:       canonical,
		CustomTitle: custom,
	}
}

func feedFixture() db.Feed {
	site := "https://example.test"
	icon := "https://example.test/favicon.ico"
	return db.Feed{
		FeedUrl: "https://example.test/feed.xml",
		SiteUrl: &site,
		IconUrl: &icon,
	}
}

func strPtr(s string) *string { return &s }

func TestEntry_HappyPath(t *testing.T) {
	r := &fakeEntryReader{
		entry:        entryFixture(),
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Cool Supply"), nil),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(r))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/entries/abc1234567", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got EntryWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != 42 {
		t.Errorf("ID = %d", got.ID)
	}
	if got.Source.Title == nil || *got.Source.Title != "Cool Supply" {
		t.Errorf("Source.Title = %v, want Cool Supply", got.Source.Title)
	}
	if got.Source.FaviconURL == nil || *got.Source.FaviconURL != "https://example.test/favicon.ico" {
		t.Errorf("Source.FaviconURL = %v, want favicon URL", got.Source.FaviconURL)
	}
	if got.Source.SiteURL == nil || *got.Source.SiteURL != "https://example.test" {
		t.Errorf("Source.SiteURL = %v, want site URL", got.Source.SiteURL)
	}
}

func TestEntry_SourceTitle_CustomOverridesCanonical(t *testing.T) {
	r := &fakeEntryReader{
		entry:        entryFixture(),
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Cool Supply"), strPtr("My Cool Supply")),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(r))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/entries/abc1234567", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var got EntryWire
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Source.Title == nil || *got.Source.Title != "My Cool Supply" {
		t.Errorf("Source.Title = %v, want custom title", got.Source.Title)
	}
}

func TestEntry_SourceTitle_EmptyCustomFallsBackToCanonical(t *testing.T) {
	r := &fakeEntryReader{
		entry:        entryFixture(),
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Cool Supply"), strPtr("")),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(r))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/entries/abc1234567", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var got EntryWire
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Source.Title == nil || *got.Source.Title != "Cool Supply" {
		t.Errorf("Source.Title = %v, want canonical title when custom is empty", got.Source.Title)
	}
}

func TestEntry_NotSubscribed_403(t *testing.T) {
	r := &fakeEntryReader{entry: entryFixture(), subOK: false}
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(r))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/entries/abc1234567", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestEntry_NotFound_404(t *testing.T) {
	r := &fakeEntryReader{getErr: sql.ErrNoRows}
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(r))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/entries/abc1234567", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestEntryExtract_CachedReturn(t *testing.T) {
	cached := "<p>cached</p>"
	entry := entryFixture()
	entry.ExtractedBody = &cached
	r := &fakeEntryReader{
		entry:        entry,
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Cool Supply"), nil),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/extract", EntryExtractHandler(r, r))

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/extract", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if r.updatedID != 0 {
		t.Errorf("Update should not be called when cache hits, got %d", r.updatedID)
	}
	var got EntryWire
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Body == nil || *got.Body != cached {
		t.Errorf("body = %v, want cached", got.Body)
	}
}

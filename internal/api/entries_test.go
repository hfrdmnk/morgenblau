package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
)

type fakeEntryReader struct {
	entry        db.FeedEntry
	getErr       error
	subOK        bool
	subErr       error
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
	if f.subErr != nil {
		return db.UserSubscription{}, f.subErr
	}
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

func subscriptionFixture(title *string) db.UserSubscription {
	return db.UserSubscription{
		Did:     "did:plc:alice",
		FeedUrl: "https://example.test/feed.xml",
		Title:   title,
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
		subscription: subscriptionFixture(strPtr("Example Source")),
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
	if got.Source.Title == nil || *got.Source.Title != "Example Source" {
		t.Errorf("Source.Title = %v, want Example Source", got.Source.Title)
	}
	if got.Source.FaviconURL == nil || *got.Source.FaviconURL != "https://example.test/favicon.ico" {
		t.Errorf("Source.FaviconURL = %v, want favicon URL", got.Source.FaviconURL)
	}
	if got.Source.SiteURL == nil || *got.Source.SiteURL != "https://example.test" {
		t.Errorf("Source.SiteURL = %v, want site URL", got.Source.SiteURL)
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
		subscription: subscriptionFixture(strPtr("Example Source")),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/extract", EntryExtractHandler(r, r, http.DefaultClient))

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

func TestEntryExtract_FreshExtraction_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<head><title>Readable Example</title></head>
<body>
<article>
<h1>Readable Example</h1>
<p>This is the readable article body.</p>
<script>alert("x")</script>
</article>
</body>
</html>`))
	}))
	defer upstream.Close()

	entry := entryFixture()
	entry.Url = upstream.URL + "/post"
	r := &fakeEntryReader{
		entry:        entry,
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Example Source")),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/extract", EntryExtractHandler(r, r, upstream.Client()))

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/extract", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if r.updatedID != entry.ID {
		t.Errorf("updated ID = %d, want %d", r.updatedID, entry.ID)
	}
	if r.updatedBody == nil {
		t.Fatal("UpdateFeedEntryExtractedBody body was nil")
	}
	if strings.Contains(*r.updatedBody, "<script") {
		t.Fatalf("extracted body was not sanitized: %s", *r.updatedBody)
	}
	if !strings.Contains(*r.updatedBody, "readable article body") {
		t.Fatalf("extracted body = %q, want article text", *r.updatedBody)
	}
	var got EntryWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Body == nil || *got.Body != *r.updatedBody {
		t.Errorf("response body = %v, want persisted extraction", got.Body)
	}
}

func TestEntryExtract_UpstreamNon2xx_502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	entry := entryFixture()
	entry.Url = upstream.URL
	r := &fakeEntryReader{
		entry:        entry,
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Example Source")),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/extract", EntryExtractHandler(r, r, upstream.Client()))

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/extract", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
	if r.updatedBody != nil {
		t.Errorf("unexpected persisted body: %q", *r.updatedBody)
	}
}

func TestEntryExtract_NetworkFailure_502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := upstream.URL
	client := upstream.Client()
	upstream.Close()

	entry := entryFixture()
	entry.Url = url
	r := &fakeEntryReader{
		entry:        entry,
		subOK:        true,
		subscription: subscriptionFixture(strPtr("Example Source")),
		feed:         feedFixture(),
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/extract", EntryExtractHandler(r, r, client))

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/extract", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestEntryExtract_DBOutageOnAuth_500(t *testing.T) {
	r := &fakeEntryReader{
		entry:  entryFixture(),
		subErr: errors.New("database unavailable"),
	}
	mux := http.NewServeMux()
	mux.Handle("POST /api/entries/{slug}/extract", EntryExtractHandler(r, r, http.DefaultClient))

	req := withSession(httptest.NewRequest(http.MethodPost, "/api/entries/abc1234567/extract", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

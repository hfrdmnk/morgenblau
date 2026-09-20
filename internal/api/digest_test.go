package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
	"morgenblau/internal/newsletter"
)

type fakeDigestReader struct {
	gotParams db.ListDigestForUserParams
	rows      []db.ListDigestForUserRow

	gotAllDid string
	allRows   []db.ListAllEntriesForUserRow
}

func (f *fakeDigestReader) ListDigestForUser(_ context.Context, arg db.ListDigestForUserParams) ([]db.ListDigestForUserRow, error) {
	f.gotParams = arg
	return f.rows, nil
}

func (f *fakeDigestReader) ListAllEntriesForUser(_ context.Context, did string) ([]db.ListAllEntriesForUserRow, error) {
	f.gotAllDid = did
	return f.allRows, nil
}

type stubJobsProbe struct{ active *jobs.Job }

func (s *stubJobsProbe) ActiveForUser(_ syntax.DID) *jobs.Job { return s.active }

func TestDigest_NoDate_ReturnsAllEntries(t *testing.T) {
	title := "Hello"
	reader := &fakeDigestReader{
		allRows: []db.ListAllEntriesForUserRow{
			{
				ID:          42,
				FeedUrl:     "https://example.test/feed.xml",
				Url:         "https://example.test/post",
				Title:       &title,
				ContentType: "blogpost",
				PublishedAt: time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	h := DigestHandler(reader, &stubJobsProbe{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got DigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if reader.gotAllDid != "did:plc:alice" {
		t.Errorf("ListAllEntriesForUser called with did = %q", reader.gotAllDid)
	}
	if len(got.Entries) != 1 || got.Entries[0].ID != float64(42) {
		t.Errorf("entries = %+v", got.Entries)
	}
}

func TestDigest_DateFilter(t *testing.T) {
	reader := &fakeDigestReader{}
	h := DigestHandler(reader, &stubJobsProbe{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-05-10", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !startsWith(reader.gotParams.PublishedAt, "2026-05-10") {
		t.Errorf("low bound = %q", reader.gotParams.PublishedAt)
	}
	if !startsWith(reader.gotParams.PublishedAt_2, "2026-05-11") {
		t.Errorf("high bound = %q", reader.gotParams.PublishedAt_2)
	}
}

func TestDigest_DateFilterUsesLocalDayBounds(t *testing.T) {
	reader := &fakeDigestReader{}
	h := DigestHandler(reader, &stubJobsProbe{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-07-10&timezone=Europe%2FZurich", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if reader.gotParams.PublishedAt != "2026-07-09T22:00:00Z" {
		t.Errorf("low bound = %q", reader.gotParams.PublishedAt)
	}
	if reader.gotParams.PublishedAt_2 != "2026-07-10T22:00:00Z" {
		t.Errorf("high bound = %q", reader.gotParams.PublishedAt_2)
	}
}

func TestDigest_DateFilterUsesDSTDayLength(t *testing.T) {
	tests := []struct {
		name     string
		date     string
		wantLow  string
		wantHigh string
	}{
		{name: "spring forward is 23 hours", date: "2026-03-29", wantLow: "2026-03-28T23:00:00Z", wantHigh: "2026-03-29T22:00:00Z"},
		{name: "fall back is 25 hours", date: "2026-10-25", wantLow: "2026-10-24T22:00:00Z", wantHigh: "2026-10-25T23:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeDigestReader{}
			h := DigestHandler(reader, &stubJobsProbe{})
			req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date="+tt.date+"&timezone=Europe%2FZurich", nil), "did:plc:alice", "sid-1")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
			if reader.gotParams.PublishedAt != tt.wantLow || reader.gotParams.PublishedAt_2 != tt.wantHigh {
				t.Errorf("bounds = [%q, %q), want [%q, %q)", reader.gotParams.PublishedAt, reader.gotParams.PublishedAt_2, tt.wantLow, tt.wantHigh)
			}
		})
	}
}

func TestDigest_InvalidTimezone_400(t *testing.T) {
	h := DigestHandler(&fakeDigestReader{}, &stubJobsProbe{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-05-10&timezone=Mars%2FOlympus", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDigest_AggregatesPrivateNewsletterMessagesWithinLocalDay(t *testing.T) {
	title := "Feed entry"
	reader := &fakeDigestReader{rows: []db.ListDigestForUserRow{{
		ID: 42, EntrySlug: "feed-1", FeedUrl: "https://example.test/feed.xml",
		Url: "https://example.test/post", Title: &title, ContentType: "blogpost",
		PublishedAt: "2026-07-10T12:00:00Z",
	}}}
	message := newsletterMessageFixture()
	message.ReceivedAt = time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	newsletters := &fakeNewsletterService{messages: []newsletter.Message{message}}
	h := DigestHandler(reader, &stubJobsProbe{}, newsletters)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-07-10&timezone=Europe%2FZurich", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("Cache-Control = %q", rr.Header().Get("Cache-Control"))
	}
	if newsletters.digestStart.Format(time.RFC3339) != "2026-07-09T22:00:00Z" || newsletters.digestEnd.Format(time.RFC3339) != "2026-07-10T22:00:00Z" {
		t.Errorf("newsletter bounds = [%s, %s)", newsletters.digestStart, newsletters.digestEnd)
	}
	var got DigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].ContentType != "blogpost" || got.Entries[1].ContentType != "newsletter" {
		t.Errorf("entries = %+v", got.Entries)
	}
}

func TestDigest_NoDateIncludesUnboundedNewsletterMessages(t *testing.T) {
	newsletters := &fakeNewsletterService{messages: []newsletter.Message{newsletterMessageFixture()}}
	h := DigestHandler(&fakeDigestReader{}, &stubJobsProbe{}, newsletters)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !newsletters.digestStart.IsZero() || !newsletters.digestEnd.IsZero() {
		t.Errorf("unbounded newsletter bounds = [%s, %s)", newsletters.digestStart, newsletters.digestEnd)
	}
	var got DigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].ContentType != "newsletter" {
		t.Errorf("entries = %+v", got.Entries)
	}
}

func TestDigest_InvalidDate_400(t *testing.T) {
	h := DigestHandler(&fakeDigestReader{}, &stubJobsProbe{})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=not-a-date", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestDigest_HasActiveJob_FlagSetWhenJobInFlight(t *testing.T) {
	reader := &fakeDigestReader{}
	probe := &stubJobsProbe{active: &jobs.Job{ID: "x", Status: jobs.StatusRunning}}
	h := DigestHandler(reader, probe)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var got DigestResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if !got.HasActiveJob {
		t.Error("HasActiveJob = false, want true")
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

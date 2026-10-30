package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	afterRead func()
}

func (f *fakeDigestReader) ListDigestForUser(_ context.Context, arg db.ListDigestForUserParams) ([]db.ListDigestForUserRow, error) {
	f.gotParams = arg
	rows := f.rows
	if f.afterRead != nil {
		f.afterRead()
	}
	return rows, nil
}

type stubJobsProbe struct{ active *jobs.Job }

func (s *stubJobsProbe) ActiveForUser(_ syntax.DID) *jobs.Job { return s.active }

func TestDigest_NoDate_ReturnsCurrentDayEntries(t *testing.T) {
	title := "Hello"
	reader := &fakeDigestReader{
		rows: []db.ListDigestForUserRow{
			{
				ID:               42,
				FeedUrl:          "https://feed.example.com/feed.xml",
				Url:              "https://feed.example.com/post",
				Title:            &title,
				ContentType:      "blogpost",
				PublishedAt:      time.Now().UTC().Format(time.RFC3339),
				SubscriptionRkey: "3kexample2a",
			},
		},
	}
	h := DigestHandler(reader, &stubJobsProbe{})
	before := time.Now()
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?timezone=Europe%2FZurich", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	after := time.Now()

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got DigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if reader.gotParams.Did != "did:plc:alice" {
		t.Errorf("ListDigestForUser called with did = %q", reader.gotParams.Did)
	}
	_, beforeStart, beforeEnd, _ := digestDayBounds("", "Europe/Zurich", before)
	_, afterStart, afterEnd, _ := digestDayBounds("", "Europe/Zurich", after)
	gotStart, gotEnd := reader.gotParams.PublishedAt, reader.gotParams.PublishedAt_2
	if (gotStart != beforeStart.Format(time.RFC3339) || gotEnd != beforeEnd.Format(time.RFC3339)) &&
		(gotStart != afterStart.Format(time.RFC3339) || gotEnd != afterEnd.Format(time.RFC3339)) {
		t.Errorf("digest bounds = [%q, %q), want current local day", gotStart, gotEnd)
	}
	if len(got.Entries) != 1 || got.Entries[0].ID != float64(42) {
		t.Fatalf("entries = %+v", got.Entries)
	}
	if got.Entries[0].Source.Rkey != "3kexample2a" {
		t.Errorf("source rkey = %q, want the subscription rkey so the digest can link to the source page", got.Entries[0].Source.Rkey)
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

func TestDigest_MarksEntriesFromPrimarySources(t *testing.T) {
	reader := &fakeDigestReader{rows: []db.ListDigestForUserRow{
		{ID: 1, EntrySlug: "primary-feed", FeedUrl: "https://primary.example.com/feed.xml", ContentType: "blogpost", PublishedAt: "2026-07-10T12:00:00Z", IsPrimary: 1},
		{ID: 2, EntrySlug: "other-feed", FeedUrl: "https://other.example.com/feed.xml", ContentType: "blogpost", PublishedAt: "2026-07-10T11:00:00Z"},
	}}
	primaryMessage := newsletterMessageFixture()
	primaryMessage.EntrySlug = "primary-newsletter"
	primaryMessage.SourcePrimary = true
	otherMessage := newsletterMessageFixture()
	otherMessage.ID = "message-other"
	otherMessage.EntrySlug = "other-newsletter"
	h := DigestHandler(reader, &stubJobsProbe{}, &fakeNewsletterService{messages: []newsletter.Message{primaryMessage, otherMessage}})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-07-10", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Entries []struct {
			EntrySlug string `json:"entrySlug"`
			Source    struct {
				Primary *bool `json:"primary"`
			} `json:"source"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"primary-feed": true, "other-feed": false, "primary-newsletter": true, "other-newsletter": false}
	if len(got.Entries) != len(want) {
		t.Fatalf("entries = %+v", got.Entries)
	}
	// The reader steps through this order, so it must match the digest page: primary first, newest first within each group.
	var order []string
	for _, entry := range got.Entries {
		order = append(order, entry.EntrySlug)
	}
	if want := []string{"primary-feed", "primary-newsletter", "other-feed", "other-newsletter"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
	for _, entry := range got.Entries {
		primary := entry.Source.Primary != nil && *entry.Source.Primary
		if primary != want[entry.EntrySlug] {
			t.Errorf("%s: source.primary = %v, want %v", entry.EntrySlug, primary, want[entry.EntrySlug])
		}
	}
}

func TestDigest_PointsUsableImagesAtTheImageProxy(t *testing.T) {
	meta := func(s string) *string { return &s }
	reader := &fakeDigestReader{rows: []db.ListDigestForUserRow{
		{ID: 1, EntrySlug: "relative", Url: "https://publication.example.com/posts/a", Metadata: meta(`{"image":"/img/a.jpg"}`), PublishedAt: "2026-07-10T12:00:00Z"},
		{ID: 2, EntrySlug: "script", Url: "https://publication.example.com/posts/b", Metadata: meta(`{"image":"javascript:alert(1)"}`), PublishedAt: "2026-07-10T11:00:00Z"},
		{ID: 3, EntrySlug: "none", Url: "https://publication.example.com/posts/c", PublishedAt: "2026-07-10T10:00:00Z"},
	}}
	rr := httptest.NewRecorder()
	DigestHandler(reader, &stubJobsProbe{}).ServeHTTP(rr, withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-07-10", nil), "did:plc:alice", "sid-1"))
	var got struct {
		Entries []struct {
			EntrySlug string  `json:"entrySlug"`
			Image     *string `json:"image"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"relative": "/api/entries/relative/image", "script": "", "none": ""}
	for _, entry := range got.Entries {
		image := ""
		if entry.Image != nil {
			image = *entry.Image
		}
		if image != want[entry.EntrySlug] {
			t.Errorf("%s: image = %q, want %q", entry.EntrySlug, image, want[entry.EntrySlug])
		}
	}
}

func TestDigest_NoDateBoundsNewsletterMessagesToCurrentDay(t *testing.T) {
	newsletters := &fakeNewsletterService{messages: []newsletter.Message{newsletterMessageFixture()}}
	reader := &fakeDigestReader{}
	h := DigestHandler(reader, &stubJobsProbe{}, newsletters)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?timezone=Europe%2FZurich", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if newsletters.digestStart.IsZero() || !newsletters.digestStart.Before(newsletters.digestEnd) {
		t.Errorf("newsletter bounds = [%s, %s), want current local day", newsletters.digestStart, newsletters.digestEnd)
	}
	if newsletters.digestStart.Format(time.RFC3339) != reader.gotParams.PublishedAt || newsletters.digestEnd.Format(time.RFC3339) != reader.gotParams.PublishedAt_2 {
		t.Errorf("newsletter bounds = [%s, %s), feed bounds = [%s, %s)", newsletters.digestStart, newsletters.digestEnd, reader.gotParams.PublishedAt, reader.gotParams.PublishedAt_2)
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

func TestDigest_JobCompletingAfterEntrySnapshotRequiresAnotherRead(t *testing.T) {
	did := syntax.DID("did:plc:example")
	tracker := jobs.New()
	job := tracker.Create(jobs.KindFetchOneFeed, did, jobs.TriggerAddFeed)
	title := "Example imported video"
	reader := &fakeDigestReader{}
	reader.afterRead = func() {
		reader.rows = []db.ListDigestForUserRow{{
			ID: 42, EntrySlug: "example-video", FeedUrl: "https://feed.example.com/videos.xml",
			Url: "https://video.example.com/watch", Title: &title, ContentType: "video",
			PublishedAt: "2026-10-08T09:00:00Z",
		}}
		tracker.SetDone(job.ID)
		reader.afterRead = nil
	}
	h := DigestHandler(reader, tracker)
	read := func() DigestResponse {
		t.Helper()
		req := withSession(httptest.NewRequest(http.MethodGet, "/api/digest?date=2026-10-08", nil), did.String(), "sid-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
		}
		var got DigestResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := read()
	if len(first.Entries) != 0 || !first.HasActiveJob {
		t.Fatalf("stale snapshot must request another read: %+v", first)
	}
	next := read()
	if next.HasActiveJob || len(next.Entries) != 1 || next.Entries[0].Title == nil || *next.Entries[0].Title != title {
		t.Fatalf("idle response must include the committed video: %+v", next)
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

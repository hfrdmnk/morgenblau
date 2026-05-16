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
	if reader.gotAllDid != "did:plc:alice" {
		t.Errorf("ListAllEntriesForUser called with did = %q", reader.gotAllDid)
	}
	var got DigestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].ID != 42 {
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

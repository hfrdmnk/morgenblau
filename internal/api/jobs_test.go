package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/jobs"
)

type fakeJobSource struct {
	byID   map[string]*jobs.Job
	active *jobs.Job
}

func (f *fakeJobSource) Get(id string, did syntax.DID) (*jobs.Job, error) {
	j, ok := f.byID[id]
	if !ok {
		return nil, jobs.ErrNotFound
	}
	if j.UserDID != did.String() {
		return nil, jobs.ErrForbidden
	}
	return j, nil
}

func (f *fakeJobSource) ActiveForUser(_ syntax.DID) *jobs.Job {
	return f.active
}

func TestJobsGet_HappyPath(t *testing.T) {
	src := &fakeJobSource{byID: map[string]*jobs.Job{
		"abc": {ID: "abc", UserDID: "did:plc:alice", Status: jobs.StatusRunning, StartedAt: time.Now()},
	}}
	mux := http.NewServeMux()
	mux.Handle("GET /api/jobs/{id}", JobsGetHandler(src))

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/jobs/abc", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got jobs.Job
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != jobs.StatusRunning {
		t.Errorf("status = %q", got.Status)
	}
}

func TestJobsGet_404(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /api/jobs/{id}", JobsGetHandler(&fakeJobSource{byID: map[string]*jobs.Job{}}))
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/jobs/missing", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestJobsGet_404AcrossUsers(t *testing.T) {
	src := &fakeJobSource{byID: map[string]*jobs.Job{
		"abc": {ID: "abc", UserDID: "did:plc:bob"},
	}}
	mux := http.NewServeMux()
	mux.Handle("GET /api/jobs/{id}", JobsGetHandler(src))
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/jobs/abc", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (collapsed with unknown-job)", rr.Code)
	}
}

func TestJobsActive_NoneReturnsNull(t *testing.T) {
	src := &fakeJobSource{}
	h := JobsActiveHandler(src)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/jobs/active", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if strings.TrimSpace(rr.Body.String()) != "null" {
		t.Errorf("body = %q, want null", rr.Body.String())
	}
}

func TestJobsActive_ReturnsJob(t *testing.T) {
	src := &fakeJobSource{active: &jobs.Job{ID: "xyz", Status: jobs.StatusRunning, UserDID: "did:plc:alice"}}
	h := JobsActiveHandler(src)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/jobs/active", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var got jobs.Job
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "xyz" {
		t.Errorf("id = %q", got.ID)
	}
}

type fakeStarter struct {
	gotDID syntax.DID
	gotSID string
	id     string
	err    error
}

func (s *fakeStarter) StartManualRefresh(_ context.Context, did syntax.DID, sessionID string) (string, error) {
	s.gotDID = did
	s.gotSID = sessionID
	if s.err != nil {
		return "", s.err
	}
	return s.id, nil
}

func TestDigestRefresh_ReturnsJobID(t *testing.T) {
	starter := &fakeStarter{id: "01HKABC"}
	h := DigestRefreshHandler(starter)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/digest/refresh", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["jobId"] != "01HKABC" {
		t.Errorf("jobId = %q", got["jobId"])
	}
}

func TestDigestRefresh_StarterError_500(t *testing.T) {
	starter := &fakeStarter{err: errors.New("boom")}
	h := DigestRefreshHandler(starter)
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/digest/refresh", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

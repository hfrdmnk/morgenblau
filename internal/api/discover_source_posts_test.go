package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/discoverposts"
)

type fakeDiscoverSourcePostsReader struct {
	posts []discoverposts.Post
	err   error
	calls int
}

func (f *fakeDiscoverSourcePostsReader) FetchPosts(_ context.Context, key string) ([]discoverposts.Post, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.posts, nil
}

func TestDiscoverSourcePosts_HappyPath(t *testing.T) {
	reader := &fakeDiscoverSourcePostsReader{posts: []discoverposts.Post{
		{Title: "Post 1", PublishedAt: "2026-07-01T00:00:00Z", URL: "https://blog.example/1", Key: "key-1"},
		{Title: "Post 2", Key: "key-2"},
	}}
	h := DiscoverSourcePostsHandler(reader)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources/posts?key=https%3A%2F%2Fblog.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var got []DiscoverPostWire
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v, want 2 posts", got)
	}
	if got[0].Title != "Post 1" || got[0].PublishedAt != "2026-07-01T00:00:00Z" || got[0].URL != "https://blog.example/1" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].Key != "key-1" {
		t.Errorf("got[0] = %+v, want key-1", got[0])
	}
	if got[1].Title != "Post 2" || got[1].PublishedAt != "" || got[1].URL != "" {
		t.Errorf("got[1] = %+v, want omitted publishedAt/url", got[1])
	}
	if got[1].Key != "key-2" {
		t.Errorf("got[1] = %+v, want key-2", got[1])
	}
}

func TestDiscoverSourcePosts_MissingKey_400(t *testing.T) {
	reader := &fakeDiscoverSourcePostsReader{}
	h := DiscoverSourcePostsHandler(reader)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources/posts", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if reader.calls != 0 {
		t.Errorf("fetch called with a missing key: %d calls", reader.calls)
	}
}

func TestDiscoverSourcePosts_FetchFailure_200EmptyArray(t *testing.T) {
	reader := &fakeDiscoverSourcePostsReader{err: errors.New("upstream unreachable")}
	h := DiscoverSourcePostsHandler(reader)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/discover/sources/posts?key=https%3A%2F%2Fblog.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a preview must never fail a card)", rr.Code)
	}
	if got := rr.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want an empty array, never null", got)
	}
}

func TestDiscoverSourcePosts_NoSession_Rejected(t *testing.T) {
	reader := &fakeDiscoverSourcePostsReader{}
	h := DiscoverSourcePostsHandler(reader)

	req := httptest.NewRequest(http.MethodGet, "/api/discover/sources/posts?key=https%3A%2F%2Fblog.example%2Ffeed.xml", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Errorf("status = %d, want a non-200 without a session", rr.Code)
	}
	if reader.calls != 0 {
		t.Errorf("fetch called without a session: %d calls", reader.calls)
	}
}

package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeFaviconReader stubs GetFeedIconURL with a per-feed-URL lookup table.
type fakeFaviconReader struct {
	icons map[string]*string
	err   error
}

func (f *fakeFaviconReader) GetFeedIconURL(_ context.Context, feedURL string) (*string, error) {
	if f.err != nil {
		return nil, f.err
	}
	v, ok := f.icons[feedURL]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return v, nil
}

func TestFaviconProxy_MissingFeedParam_400(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{}, http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestFaviconProxy_UnknownFeed_404(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_FeedWithEmptyIcon_404(t *testing.T) {
	empty := ""
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &empty,
	}}, http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_FeedWithNullIcon_404(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": nil,
	}}, http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_HappyPath_StreamsImage(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/favicon.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
	if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=86400") {
		t.Errorf("Cache-Control = %q, want max-age=86400", got)
	}
	if got := rr.Body.Bytes(); string(got) != string(body) {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestFaviconProxy_NonImageContentType_502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html/>"))
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/not-an-image"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestFaviconProxy_UpstreamError_502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/oops"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestFaviconProxy_BodyCapEnforced(t *testing.T) {
	// Upstream returns 1 MiB. Handler caps at 256 KiB.
	big := make([]byte, 1<<20)
	for i := range big {
		big[i] = 0x41
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/huge.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Body.Len(); got != 256*1024 {
		t.Errorf("body length = %d, want %d", got, 256*1024)
	}
}

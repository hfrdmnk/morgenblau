package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"morgenblau/internal/safehttp"
)

type fakeFaviconReader struct {
	icons map[string]*string
	err   error
}

func (f *fakeFaviconReader) GetFeedIconURL(_ context.Context, feedURL string) (*string, error) {
	if f.err != nil {
		return nil, f.err
	}
	icon, ok := f.icons[feedURL]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return icon, nil
}

func faviconRequest() *http.Request {
	return withSession(
		httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil),
		"did:plc:alice",
		"sid-1",
	)
}

func TestFaviconProxy_RejectsMissingMiddlewareSession(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{}, http.DefaultClient)
	req := httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
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
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, faviconRequest())

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_EmptyIcon_404(t *testing.T) {
	empty := ""
	tests := []struct {
		name string
		icon *string
	}{
		{name: "empty", icon: &empty},
		{name: "null", icon: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
				"https://example.test/feed.xml": tt.icon,
			}}, http.DefaultClient)
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, faviconRequest())

			if rr.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rr.Code)
			}
		})
	}
}

func TestFaviconProxy_LookupError_500(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{err: errors.New("db unavailable")}, http.DefaultClient)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, faviconRequest())

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestFaviconProxy_StreamsImage(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	var gotUA string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/favicon.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, faviconRequest())

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
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}
}

func TestFaviconProxy_RejectsBadUpstreamResponse(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
	}{
		{name: "non-image", status: http.StatusOK, contentType: "text/html"},
		{name: "error status", status: http.StatusInternalServerError, contentType: "image/png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("body"))
			}))
			defer upstream.Close()

			iconURL := upstream.URL + "/favicon"
			h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
				"https://example.test/feed.xml": &iconURL,
			}}, upstream.Client())
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, faviconRequest())

			if rr.Code != http.StatusBadGateway {
				t.Errorf("status = %d, want 502", rr.Code)
			}
		})
	}
}

func TestFaviconProxy_RejectsOversizeContentLength(t *testing.T) {
	big := make([]byte, faviconProxyMaxBytes+1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(big)))
		_, _ = w.Write(big)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/huge.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, faviconRequest())

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestFaviconProxy_RejectsOversizeUnknownLength(t *testing.T) {
	big := make([]byte, faviconProxyMaxBytes+1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/huge.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, faviconRequest())

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestFaviconProxy_UnderCap_StreamsIntact(t *testing.T) {
	body := make([]byte, faviconProxyMaxBytes-1)
	for i := range body {
		body[i] = 0x41
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/favicon.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, upstream.Client())
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, faviconRequest())

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Body.Bytes(); string(got) != string(body) {
		t.Errorf("body length = %d, want %d", len(got), len(body))
	}
}

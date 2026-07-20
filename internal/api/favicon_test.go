package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"morgenblau/internal/database/db"
	"morgenblau/internal/safehttp"
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

// fakeDiscoverCandidateFaviconReader stubs the discover-candidate fallback lookup keyed by source key.
type fakeDiscoverCandidateFaviconReader struct {
	icons map[string]*string
	err   error
}

func (f *fakeDiscoverCandidateFaviconReader) GetDiscoverSourceFaviconURL(_ context.Context, sourceKey string) (*string, error) {
	if f.err != nil {
		return nil, f.err
	}
	v, ok := f.icons[sourceKey]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return v, nil
}

func noDiscoverCandidateFavicons() *fakeDiscoverCandidateFaviconReader {
	return &fakeDiscoverCandidateFaviconReader{icons: map[string]*string{}}
}

// fakePublicationResolutionFaviconReader stubs the discover-resolutions fallback lookup keyed by canonical_key.
type fakePublicationResolutionFaviconReader struct {
	icons map[string]*string
	err   error
}

func (f *fakePublicationResolutionFaviconReader) GetDiscoverPublicationResolutionByCanonicalKey(_ context.Context, canonicalKey *string) (db.DiscoverPublicationResolution, error) {
	if f.err != nil {
		return db.DiscoverPublicationResolution{}, f.err
	}
	v, ok := f.icons[*canonicalKey]
	if !ok {
		return db.DiscoverPublicationResolution{}, sql.ErrNoRows
	}
	return db.DiscoverPublicationResolution{IconUrl: v}, nil
}

func noPublicationResolutionFavicons() *fakePublicationResolutionFaviconReader {
	return &fakePublicationResolutionFaviconReader{icons: map[string]*string{}}
}

// fakeOnDemandFaviconResolver stubs the last-resort resolver keyed by the resolutionKey (feedkey.Normalize'd feed URL).
type fakeOnDemandFaviconResolver struct {
	icons map[string]string
	err   error
	calls int
}

func (f *fakeOnDemandFaviconResolver) Resolve(_ context.Context, candidateKey string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.icons[candidateKey], nil
}

func noOnDemandFavicon() *fakeOnDemandFaviconResolver {
	return &fakeOnDemandFaviconResolver{icons: map[string]string{}}
}

func TestFaviconProxy_MissingFeedParam_400(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestFaviconProxy_UnknownFeed_404(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), http.DefaultClient)
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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), http.DefaultClient)
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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_HappyPath_StreamsImage(t *testing.T) {
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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
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
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestFaviconProxy_OversizeByContentLength_502(t *testing.T) {
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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rr.Code)
	}
}

func TestFaviconProxy_OversizeUnknownLength_502(t *testing.T) {
	// No explicit Content-Length: a write this large forces net/http to switch to chunked transfer, so ContentLength is -1 on the client.
	big := make([]byte, faviconProxyMaxBytes+1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/huge.png"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{
		"https://example.test/feed.xml": &iconURL,
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

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
	}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Body.Bytes(); string(got) != string(body) {
		t.Errorf("body length = %d, want %d (byte-for-byte)", len(got), len(body))
	}
}

func TestFaviconProxy_UnsubscribedCandidate_FallsBackToDiscoverCache(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/favicon.ico"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, &fakeDiscoverCandidateFaviconReader{icons: map[string]*string{
		"https://candidate.example/feed.xml": &iconURL,
	}}, noPublicationResolutionFavicons(), noOnDemandFavicon(), upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the discover-candidate cache to serve a real favicon", rr.Code, rr.Body.String())
	}
	if got := rr.Body.Bytes(); string(got) != string(body) {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestFaviconProxy_NeitherCatalogNorCandidateHasIcon_404(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(), noPublicationResolutionFavicons(), noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Fnowhere.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_CandidateWithNullFaviconURL_404(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, &fakeDiscoverCandidateFaviconReader{icons: map[string]*string{
		"https://candidate.example/feed.xml": nil, // favicon discovery failed on the last fetch
	}}, noPublicationResolutionFavicons(), noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_ResolutionsCache_FallsBackWhenFeedAndCandidateMiss(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	// canonical_key is stored as the publication's at:// URI verbatim; the proxy must not feedkey.Normalize it away.
	pubURI := "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/blue.example.publication/3kexample1234"
	iconURL := upstream.URL + "/favicon.ico"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(),
		&fakePublicationResolutionFaviconReader{icons: map[string]*string{pubURI: &iconURL}}, noOnDemandFavicon(), upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed="+url.QueryEscape(pubURI), nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the resolutions cache to serve a real favicon", rr.Code, rr.Body.String())
	}
	if got := rr.Body.Bytes(); string(got) != string(body) {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestFaviconProxy_CandidateHit_WinsOverResolutions(t *testing.T) {
	candidateBody := []byte("candidate-icon")
	candidateUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(candidateBody)
	}))
	defer candidateUpstream.Close()
	resolutionsUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("resolutions-icon"))
	}))
	defer resolutionsUpstream.Close()

	candidateIconURL := candidateUpstream.URL + "/favicon.ico"
	resolutionsIconURL := resolutionsUpstream.URL + "/favicon.ico"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}},
		&fakeDiscoverCandidateFaviconReader{icons: map[string]*string{"https://candidate.example/feed.xml": &candidateIconURL}},
		&fakePublicationResolutionFaviconReader{icons: map[string]*string{"https://candidate.example/feed.xml": &resolutionsIconURL}},
		noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != string(candidateBody) {
		t.Errorf("body = %q, want the candidate cache's icon %q (it must win over resolutions)", got, candidateBody)
	}
}

func TestFaviconProxy_FeedHit_WinsOverCandidateAndResolutions(t *testing.T) {
	feedBody := []byte("feed-icon")
	feedUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(feedBody)
	}))
	defer feedUpstream.Close()

	feedIconURL := feedUpstream.URL + "/favicon.ico"
	otherIconURL := "https://unused.example/favicon.ico"
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{"https://example.test/feed.xml": &feedIconURL}},
		&fakeDiscoverCandidateFaviconReader{icons: map[string]*string{"https://example.test/feed.xml": &otherIconURL}},
		&fakePublicationResolutionFaviconReader{icons: map[string]*string{"https://example.test/feed.xml": &otherIconURL}},
		noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fexample.test%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != string(feedBody) {
		t.Errorf("body = %q, want the feed catalog's icon %q (it must win over both fallbacks)", got, feedBody)
	}
}

func TestFaviconProxy_ResolutionsRowWithNullIconURL_404(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(),
		&fakePublicationResolutionFaviconReader{icons: map[string]*string{"https://candidate.example/feed.xml": nil}},
		noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestFaviconProxy_ResolutionsLookupError_500(t *testing.T) {
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(),
		&fakePublicationResolutionFaviconReader{err: errors.New("db unavailable")}, noOnDemandFavicon(), http.DefaultClient)
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestFaviconProxy_AllCachesMiss_FallsBackToOnDemandResolver(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/favicon.ico"
	onDemand := &fakeOnDemandFaviconResolver{icons: map[string]string{"https://candidate.example/feed.xml": iconURL}}
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(),
		noPublicationResolutionFavicons(), onDemand, upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the on-demand resolver to serve a real favicon", rr.Code, rr.Body.String())
	}
	if got := rr.Body.Bytes(); string(got) != string(body) {
		t.Errorf("body = %q, want %q", got, body)
	}
	if onDemand.calls != 1 {
		t.Errorf("onDemand.calls = %d, want 1 (called only once all three caches miss)", onDemand.calls)
	}
}

func TestFaviconProxy_ResolutionsHit_SkipsOnDemandResolver(t *testing.T) {
	body := []byte("\x89PNG\r\n\x1a\nfake-png-bytes")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	iconURL := upstream.URL + "/favicon.ico"
	onDemand := noOnDemandFavicon()
	h := FaviconProxyHandler(&fakeFaviconReader{icons: map[string]*string{}}, noDiscoverCandidateFavicons(),
		&fakePublicationResolutionFaviconReader{icons: map[string]*string{"https://candidate.example/feed.xml": &iconURL}},
		onDemand, upstream.Client())
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/favicon?feed=https%3A%2F%2Fcandidate.example%2Ffeed.xml", nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if onDemand.calls != 0 {
		t.Errorf("onDemand.calls = %d, want 0 (a resolutions-cache hit must short-circuit before on-demand discovery)", onDemand.calls)
	}
}

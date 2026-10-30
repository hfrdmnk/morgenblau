package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
)

func bodyImageUpstream(t *testing.T) (*httptest.Server, *atomic.Int64, *atomic.Value) {
	t.Helper()
	var hits atomic.Int64
	var lastPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastPath.Store(r.URL.RequestURI())
		switch r.URL.Path {
		case "/img/chart.png", "/img/extracted.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png bytes"))
		case "/img/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<p>not an image</p>"))
		case "/img/huge.png":
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Length", strconv.Itoa(entryBodyImageMaxBytes+1))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &lastPath
}

func bodyImageRequest(src string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/entries/abc123/images?src="+url.QueryEscape(src), nil)
	req.SetPathValue("slug", "abc123")
	return withSession(req, "did:plc:alice", "sid-1")
}

func bodyImageReader(postURL string) *fakeEntryReader {
	entry := entryFixture()
	entry.Url = postURL
	content := `<p>Intro</p><img src="../img/chart.png?a=1&amp;b=2" alt="Chart"><img src="/img/page"><img src="/img/huge.png">`
	extracted := `<img src="/img/extracted.png">`
	entry.ContentHtml = &content
	entry.ExtractedBody = &extracted
	return &fakeEntryReader{entry: entry, subOK: true}
}

func TestEntryBodyImage_StreamsAnImageTheStoredArticleShows(t *testing.T) {
	upstream, _, lastPath := bodyImageUpstream(t)
	for _, src := range []string{"../img/chart.png?a=1&b=2", "/img/extracted.png"} {
		t.Run(src, func(t *testing.T) {
			rr := httptest.NewRecorder()
			EntryBodyImageHandler(bodyImageReader(upstream.URL+"/posts/a"), upstream.Client()).ServeHTTP(rr, bodyImageRequest(src))
			if rr.Code != http.StatusOK || rr.Body.String() != "png bytes" {
				t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
			}
			if src == "../img/chart.png?a=1&b=2" && lastPath.Load() != "/img/chart.png?a=1&b=2" {
				t.Errorf("fetched %v, want the src resolved against the post URL", lastPath.Load())
			}
			for header, want := range map[string]string{
				"Content-Type":           "image/png",
				"X-Content-Type-Options": "nosniff",
				"Cache-Control":          "private, max-age=86400",
			} {
				if got := rr.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
			// An SVG opened on its own would otherwise run script on the app's origin.
			if rr.Header().Get("Content-Security-Policy") == "" {
				t.Error("missing Content-Security-Policy")
			}
		})
	}
}

func TestEntryBodyImage_RefusesWhatItShouldNotServe(t *testing.T) {
	upstream, hits, _ := bodyImageUpstream(t)
	notFollowed := bodyImageReader(upstream.URL + "/posts/a")
	notFollowed.subOK = false
	cases := []struct {
		name    string
		reader  *fakeEntryReader
		src     string
		status  int
		fetches bool
	}{
		{"feed not followed", notFollowed, "../img/chart.png?a=1&b=2", http.StatusNotFound, false},
		{"src not in the article", bodyImageReader(upstream.URL + "/posts/a"), upstream.URL + "/img/chart.png?a=1&b=2", http.StatusNotFound, false},
		{"missing src", bodyImageReader(upstream.URL + "/posts/a"), "", http.StatusNotFound, false},
		{"not an image", bodyImageReader(upstream.URL + "/posts/a"), "/img/page", http.StatusBadGateway, true},
		{"over the size cap", bodyImageReader(upstream.URL + "/posts/a"), "/img/huge.png", http.StatusBadGateway, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := hits.Load()
			rr := httptest.NewRecorder()
			EntryBodyImageHandler(tc.reader, upstream.Client()).ServeHTTP(rr, bodyImageRequest(tc.src))
			if rr.Code != tc.status {
				t.Errorf("status = %d, want %d", rr.Code, tc.status)
			}
			if fetched := hits.Load() > before; fetched != tc.fetches {
				t.Errorf("fetched upstream = %v, want %v", fetched, tc.fetches)
			}
		})
	}
}

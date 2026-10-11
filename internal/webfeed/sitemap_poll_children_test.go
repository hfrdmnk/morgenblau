package webfeed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

// etagSite serves fixed bodies with strong ETags and answers a matching If-None-Match with 304, as most CDNs do.
type etagSite struct {
	srv   *httptest.Server
	mu    sync.Mutex
	pages map[string]string
}

func newETagSite(t *testing.T) *etagSite {
	e := &etagSite{pages: map[string]string{}}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/news" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		e.mu.Lock()
		body, ok := e.pages[r.URL.Path]
		e.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256([]byte(body))
		tag := `"` + hex.EncodeToString(sum[:8]) + `"`
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
		} else {
			w.Header().Set("Content-Type", "application/xml")
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *etagSite) set(path, body string) {
	e.mu.Lock()
	e.pages[path] = body
	e.mu.Unlock()
}

func (e *etagSite) url(path string) string { return e.srv.URL + path }

func TestSitemapPollSeesPostsInALaterChildSitemap(t *testing.T) {
	ctx := context.Background()
	e := newETagSite(t)
	e.set("/robots.txt", "User-agent: *\nAllow: /\nSitemap: "+e.url("/sitemap-index.xml")+"\n")
	e.set("/sitemap-index.xml", `<sitemapindex><sitemap><loc>`+e.url("/sitemap-posts-2023.xml")+`</loc></sitemap><sitemap><loc>`+e.url("/sitemap-posts-2024.xml")+`</loc></sitemap></sitemapindex>`)
	older := []string{e.url("/news/first-launch-of-the-reader"), e.url("/news/a-second-announcement-today"), e.url("/news/third-time-is-the-charm")}
	newer := []string{e.url("/news/a-fourth-story-for-readers"), e.url("/news/the-fifth-story-of-the-year")}
	e.set("/sitemap-posts-2023.xml", sitemapXML(older...))
	e.set("/sitemap-posts-2024.xml", sitemapXML(newer...))
	opts := Options{Client: e.srv.Client()}
	res, err := Setup(ctx, e.url("/news"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.Mode != ModeSitemap || len(res.Items) != 5 {
		t.Fatalf("setup mode %s items %v", res.Recipe.Mode, itemURLs(res.Items))
	}

	first, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res), Known: res.Items}, opts)
	if err != nil || first.Verdict != PollOK || len(first.Items) != 5 {
		t.Fatalf("first poll = verdict %q items %v, %v", first.Verdict, itemURLs(first.Items), err)
	}

	fresh := e.url("/news/a-story-published-after-setup")
	e.set("/sitemap-posts-2024.xml", sitemapXML(append(slices.Clone(newer), fresh)...))
	got, err := Poll(ctx, PollInput{
		Recipe: res.Recipe, History: historyFrom(res), Known: res.Items,
		ETag: first.ETag, LastModified: first.LastModified, LinkSetHash: first.LinkSetHash,
	}, opts)
	if err != nil || got.Verdict != PollOK || got.NotModified {
		t.Fatalf("poll after a new post = verdict %q not modified %v, %v", got.Verdict, got.NotModified, err)
	}
	if !slices.Contains(itemURLs(got.Items), fresh) {
		t.Fatalf("poll after a new post = %v, want %s from the newest child sitemap", itemURLs(got.Items), fresh)
	}
}

func TestSitemapPollOfASingleFileStaysConditional(t *testing.T) {
	ctx := context.Background()
	e := newETagSite(t)
	e.set("/robots.txt", "User-agent: *\nAllow: /\nSitemap: "+e.url("/sitemap-news.xml")+"\n")
	e.set("/sitemap-news.xml", sitemapXML(e.url("/news/first-launch-of-the-reader"), e.url("/news/a-second-announcement-today"), e.url("/news/third-time-is-the-charm")))
	opts := Options{Client: e.srv.Client()}
	res, err := Setup(ctx, e.url("/news"), opts)
	if err != nil || res.Recipe.FeedURL != e.url("/sitemap-news.xml") {
		t.Fatalf("setup feed %q, %v", res.Recipe.FeedURL, err)
	}
	first, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res)}, opts)
	if err != nil || first.Verdict != PollOK || first.ETag == "" {
		t.Fatalf("first poll = verdict %q etag %q, %v", first.Verdict, first.ETag, err)
	}
	again, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res), ETag: first.ETag, LinkSetHash: first.LinkSetHash}, opts)
	if err != nil || !again.NotModified || again.ETag != first.ETag {
		t.Fatalf("unchanged poll = not modified %v etag %q, %v", again.NotModified, again.ETag, err)
	}
}

package webfeed

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testSite is an invented publication served over httptest, recording how it was fetched.
type testSite struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	pages    map[string]string
	status   map[string]int
	hits     map[string]int
	agents   map[string]bool
	langs    map[string]string
	inflight atomic.Int32
	peak     atomic.Int32
	delay    time.Duration
}

func newTestSite(t *testing.T) *testSite {
	s := &testSite{t: t, pages: map[string]string{}, status: map[string]int{}, hits: map[string]int{}, agents: map[string]bool{}, langs: map[string]string{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *testSite) serve(w http.ResponseWriter, r *http.Request) {
	cur := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		p := s.peak.Load()
		if cur <= p || s.peak.CompareAndSwap(p, cur) {
			break
		}
	}
	s.mu.Lock()
	s.hits[r.URL.Path]++
	s.agents[r.Header.Get("User-Agent")] = true
	s.langs[r.URL.Path] = r.Header.Get("Accept-Language")
	body, ok := s.pages[r.URL.RequestURI()]
	code := s.status[r.URL.Path]
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if code != 0 {
		w.WriteHeader(code)
		fmt.Fprint(w, body)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, ".xml"):
		w.Header().Set("Content-Type", "application/xml")
	case strings.HasSuffix(r.URL.Path, ".txt"):
		w.Header().Set("Content-Type", "text/plain")
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	fmt.Fprint(w, body)
}

func (s *testSite) set(path, body string) {
	s.mu.Lock()
	s.pages[path] = body
	s.mu.Unlock()
}

func (s *testSite) url(path string) string { return s.srv.URL + path }

func (s *testSite) host() string { return strings.TrimPrefix(s.srv.URL, "http://") }

func (s *testSite) opts() Options { return Options{Client: s.srv.Client()} }

// article is a post page with og:title and a publish date.
func article(title, ogTitle, published string) string {
	meta := ""
	if ogTitle != "" {
		meta += `<meta property="og:title" content="` + ogTitle + `"><meta property="og:type" content="article">`
	}
	if published != "" {
		meta += `<meta property="article:published_time" content="` + published + `">`
	}
	return `<!doctype html><html lang="en"><head><title>` + title + ` | Example Publication</title>` + meta + `</head><body>` + siteNav +
		`<main><article><h1>` + title + `</h1><p>Body text of the post.</p></article></main>` + siteFooter + `</body></html>`
}

// addBlog serves /blog with n posts and their article pages.
func (s *testSite) addBlog(n int) {
	s.set("/blog", page("Blog | Example Publication", siteNav+blogList(n)+siteFooter))
	for i := range n {
		t := postTitles[i%len(postTitles)]
		s.set("/blog/"+slugify(t), article(t, t, fmt.Sprintf("2024-03-%02dT09:00:00Z", 20-i)))
	}
}

func (s *testSite) get(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return []byte(s.pages[path]), nil
}

package webfeed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// historyFrom is the good-poll record a setup leaves behind, the way the server stores it.
func historyFrom(res *Result) []GoodPoll {
	g := GoodPoll{URLTemplate: DominantTemplate(res.Items, streamTemplates(res.Recipe.Streams)), TitleShape: ComputeSignals(res.Items, nil).TitleShape}
	for _, it := range res.Items {
		g.URLs = append(g.URLs, it.URL)
		g.Titles = append(g.Titles, it.Title)
	}
	return []GoodPoll{g}
}

func movedList(n int, prefix string) string {
	return strings.ReplaceAll(blogList(n), `href="/blog/`, `href="/`+prefix+`/`)
}

func TestPollDetectorVerdicts(t *testing.T) {
	listing := siteNav + `<div class="layout">` + blogList(8) + `</div>` + siteFooter
	newPost := `<ul class="posts"><li class="post"><h2><a href="/blog/a-brand-new-post-today">A brand new post today</a></h2><time datetime="2024-03-25">March 25, 2024</time></li>`
	cases := []struct {
		name    string
		status  int
		body    string
		verdict PollVerdict
		reason  string
		blocked bool
		items   int
	}{
		{name: "new post", body: page("Blog", strings.Replace(listing, `<ul class="posts">`, newPost, 1)), verdict: PollOK, items: 9},
		{name: "server error", status: 500, body: "oops", verdict: PollBroken, reason: ReasonHTTPStatus},
		{name: "forbidden", status: 403, body: "no", verdict: PollBroken, reason: ReasonHTTPStatus, blocked: true},
		{name: "challenge", body: page("Just a moment...", `<p>Checking your browser before accessing the site.</p>`), verdict: PollBroken, reason: ReasonChallenge, blocked: true},
		{name: "soft 404", body: page("Page not found | Example Publication", siteNav+`<main><h1>Page not found</h1><p>Try the archive.</p></main>`+siteFooter), verdict: PollBroken, reason: ReasonErrorPage},
		{name: "cookie wall over the list", body: page("Blog", `<div class="consent"><h2>Before you continue</h2><p>We and our partners use cookies.</p><button>Accept all</button></div>`+listing), verdict: PollBroken, reason: ReasonConsentWall},
		{name: "spa shell", body: page("Blog", `<div id="root"></div><script src="/assets/app.js"></script>`), verdict: PollBroken, reason: ReasonSPAShell},
		{name: "empty behind a cookie notice", body: page("Blog", `<p>We use cookies to improve your experience. Manage consent.</p>`), verdict: PollBroken, reason: ReasonConsentWall},
		{name: "url scheme changed", body: page("Blog", siteNav+movedList(8, "posts")+siteFooter), verdict: PollSuspect, reason: ReasonEmpty},
		{name: "list cut to three", body: page("Blog", siteNav+blogList(3)+siteFooter), verdict: PollSuspect, reason: ReasonCountRatio, items: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSite(t)
			s.set("/blog", page("Blog", listing))
			res := setupRecipe(t, s, "/blog")
			s.set("/blog", tc.body)
			if tc.status != 0 {
				s.mu.Lock()
				s.status["/blog"] = tc.status
				s.mu.Unlock()
			}
			got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tc.verdict || got.Reason != tc.reason || got.Blocked != tc.blocked {
				t.Fatalf("verdict %q reason %q blocked %v, want %q %q %v", got.Verdict, got.Reason, got.Blocked, tc.verdict, tc.reason, tc.blocked)
			}
			if len(got.Items) != tc.items {
				t.Fatalf("items = %v, want %d", itemURLs(got.Items), tc.items)
			}
		})
	}
}

// conditionalSite serves only /blog and answers If-None-Match with 304 until told to ignore validators.
type conditionalSite struct {
	mu         sync.Mutex
	body, etag string
	ignore     bool
	ctype      string
	seen       []http.Header
}

func (c *conditionalSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/blog" {
		http.NotFound(w, r)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, r.Header.Clone())
	if !c.ignore && r.Header.Get("If-None-Match") == c.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ctype := c.ctype
	if ctype == "" {
		ctype = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("ETag", c.etag)
	w.Header().Set("Last-Modified", "Fri, 09 Oct 2026 08:00:00 GMT")
	_, _ = w.Write([]byte(c.body))
}

func (c *conditionalSite) last() http.Header {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seen[len(c.seen)-1]
}

func TestPollUsesValidatorsAndSkipsAnUnchangedLinkSet(t *testing.T) {
	listing := page("Blog", siteNav+blogList(8)+siteFooter)
	site := &conditionalSite{body: listing, etag: `"v1"`}
	srv := httptest.NewServer(site)
	t.Cleanup(srv.Close)
	opts := Options{Client: srv.Client()}
	res, err := Setup(context.Background(), srv.URL+"/blog", opts)
	if err != nil {
		t.Fatal(err)
	}
	in := PollInput{Recipe: res.Recipe, History: historyFrom(res)}
	ctx := context.Background()

	first, err := Poll(ctx, in, opts)
	if err != nil || first.Verdict != PollOK || first.NotModified || len(first.Items) != 8 {
		t.Fatalf("first poll = %+v, %v", first, err)
	}
	if first.ETag != `"v1"` || first.LastModified == "" || first.LinkSetHash == "" {
		t.Fatalf("validators = %q %q %q", first.ETag, first.LastModified, first.LinkSetHash)
	}
	if got := site.last().Get("Accept-Language"); got != "en" {
		t.Errorf("Accept-Language = %q, want the page's own lang", got)
	}
	if ex := first.Excerpts[first.Items[0].URL]; ex != "A short excerpt that explains the post in one or two sentences for readers." {
		t.Errorf("excerpt = %q", ex)
	}

	in.ETag, in.LastModified, in.LinkSetHash = first.ETag, first.LastModified, first.LinkSetHash
	notModified, err := Poll(ctx, in, opts)
	if err != nil || notModified.Verdict != PollOK || !notModified.NotModified || len(notModified.Items) != 0 || notModified.ETag != `"v1"` {
		t.Fatalf("304 poll = %+v, %v", notModified, err)
	}
	if got := site.last().Get("If-None-Match"); got != `"v1"` {
		t.Errorf("If-None-Match = %q", got)
	}

	site.mu.Lock()
	site.ignore, site.etag, site.body = true, `"v2"`, rehash(listing, "next-build")
	site.mu.Unlock()
	sameLinks, err := Poll(ctx, in, opts)
	if err != nil || !sameLinks.NotModified || sameLinks.ETag != `"v2"` || sameLinks.LinkSetHash != first.LinkSetHash {
		t.Fatalf("restyled page with the same links = %+v, %v", sameLinks, err)
	}

	in.Unconditional = true
	forced, err := Poll(ctx, in, opts)
	if err != nil || forced.NotModified || len(forced.Items) != 8 {
		t.Fatalf("unconditional poll = %+v, %v", forced, err)
	}
	if got := site.last().Get("If-None-Match"); got != "" {
		t.Errorf("unconditional poll sent If-None-Match %q", got)
	}
}

func TestPollMarksNonHTMLBroken(t *testing.T) {
	listing := page("Blog", siteNav+blogList(8)+siteFooter)
	site := &conditionalSite{body: listing, etag: `"v1"`, ignore: true}
	srv := httptest.NewServer(site)
	t.Cleanup(srv.Close)
	opts := Options{Client: srv.Client()}
	res, err := Setup(context.Background(), srv.URL+"/blog", opts)
	if err != nil {
		t.Fatal(err)
	}
	site.mu.Lock()
	site.ctype, site.body = "application/json", `{"posts":[]}`
	site.mu.Unlock()
	got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, opts)
	if err != nil || got.Verdict != PollBroken || got.Reason != ReasonNotHTML {
		t.Fatalf("poll = %+v, %v", got, err)
	}
}

func TestPollReportsAnUnreachableSiteAsBroken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	opts := Options{Client: srv.Client()}
	recipe := NewRecipe(srv.URL+"/blog", ModeStreams)
	srv.Close()
	got, err := Poll(context.Background(), PollInput{Recipe: recipe}, opts)
	if err != nil || got.Verdict != PollBroken || got.Reason != ReasonUnreachable {
		t.Fatalf("poll = %+v, %v", got, err)
	}
}

func TestPollHonorsRobotsWithoutFetchingTheListing(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	s.set("/robots.txt", "User-agent: *\nDisallow: /blog\n")
	before := s.hits["/blog"]
	got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != PollBroken || got.Reason != ReasonRobots || !got.Blocked {
		t.Fatalf("verdict %q reason %q blocked %v, want broken by robots.txt", got.Verdict, got.Reason, got.Blocked)
	}
	if s.hits["/blog"] != before {
		t.Error("the poll fetched a listing robots.txt disallows")
	}
}

func TestPollReadsRobotsFromADailyCache(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	now := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	cache := NewRobotsCache()
	cache.now = func() time.Time { return now }
	opts := s.opts()
	opts.Robots = cache
	poll := func() *PollResult {
		t.Helper()
		got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, opts)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	robotsBefore := s.hits["/robots.txt"]
	poll()
	poll()
	if n := s.hits["/robots.txt"] - robotsBefore; n != 1 {
		t.Fatalf("robots.txt fetched %d times for two polls, want 1", n)
	}
	s.set("/robots.txt", "User-agent: *\nDisallow: /blog\n")
	now = now.Add(23 * time.Hour)
	if got := poll(); got.Verdict == PollBroken {
		t.Fatalf("a cached robots.txt was refetched within the day: %+v", got)
	}
	now = now.Add(2 * time.Hour)
	if got := poll(); got.Verdict != PollBroken || got.Reason != ReasonRobots {
		t.Fatalf("verdict %q %q after a day, want the new robots.txt honored", got.Verdict, got.Reason)
	}
}

package webfeed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/safehttp"
)

// lazyRequest is the body Livewire's script posts to load lazy components.
type lazyRequest struct {
	Token      string `json:"_token"`
	Components []struct {
		Snapshot string          `json:"snapshot"`
		Updates  json.RawMessage `json:"updates"`
		Calls    []struct {
			Path   string   `json:"path"`
			Method string   `json:"method"`
			Params []string `json:"params"`
		} `json:"calls"`
	} `json:"components"`
}

type lazyPost struct {
	path   string
	header http.Header
	body   lazyRequest
}

// lazySite is an invented Laravel site whose pages come from testSite and whose Livewire endpoint answers each
// __lazyLoad call with the list registered for its parameter.
type lazySite struct {
	*testSite
	lmu    sync.Mutex
	update string
	lists  map[string]string
	// redirect, postStatus and reply, when set, answer every POST instead of the registered lists.
	redirect   string
	postStatus int
	reply      string
	etag       string
	posts      []lazyPost
	gets       []http.Header
}

func newLazySite(t *testing.T) *lazySite {
	l := &lazySite{testSite: unservedTestSite(t), update: "/livewire/update", lists: map[string]string{}, etag: `"shell-1"`}
	l.srv = httptest.NewServer(l)
	t.Cleanup(l.srv.Close)
	return l
}

func (l *lazySite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		l.lmu.Lock()
		if r.URL.Path == "/blog" {
			l.gets = append(l.gets, r.Header.Clone())
		}
		etag := l.etag
		l.lmu.Unlock()
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", "Fri, 09 Oct 2026 08:00:00 GMT")
		w.Header().Add("Set-Cookie", "XSRF-TOKEN=xsrf-value; expires=Fri, 09 Oct 2026 10:00:00 GMT; Max-Age=7200; path=/; samesite=lax")
		w.Header().Add("Set-Cookie", "example_session=session-value; path=/; httponly; samesite=lax")
		l.serve(w, r)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var req lazyRequest
	_ = json.Unmarshal(raw, &req)
	l.lmu.Lock()
	defer l.lmu.Unlock()
	l.posts = append(l.posts, lazyPost{path: r.URL.Path, header: r.Header.Clone(), body: req})
	switch {
	case r.URL.Path != l.update:
		http.NotFound(w, r)
		return
	case l.redirect != "":
		http.Redirect(w, r, l.redirect, http.StatusTemporaryRedirect)
		return
	case l.postStatus != 0:
		w.WriteHeader(l.postStatus)
		fmt.Fprint(w, l.reply)
		return
	case l.reply != "":
		fmt.Fprint(w, l.reply)
		return
	}
	type component struct {
		Snapshot string            `json:"snapshot"`
		Effects  map[string]string `json:"effects"`
	}
	var out struct {
		Components []component `json:"components"`
	}
	for _, c := range req.Components {
		out.Components = append(out.Components, component{Snapshot: c.Snapshot, Effects: map[string]string{"html": l.lists[c.Calls[0].Params[0]]}})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (l *lazySite) postCount() int {
	l.lmu.Lock()
	defer l.lmu.Unlock()
	return len(l.posts)
}

func snapshotJSON(param string) string {
	return `{"data":{"list":"` + param + `"},"memo":{"id":"c-` + param + `","name":"posts-list","path":"/"},"checksum":"0f0f"}`
}

// placeholder is the element Livewire renders for a lazy component; attr is x-init or x-intersect.
func placeholder(attr, param string) string {
	return `<section wire:snapshot="` + html.EscapeString(snapshotJSON(param)) + `" wire:effects="[]" wire:id="c-` + param + `" ` +
		attr + `="$wire.__lazyLoad('` + param + `')"><div class="spinner"><span>Loading</span></div></section>`
}

// lazyShell is a listing whose posts all sit behind lazy placeholders, beside a few off-site links.
func lazyShell(updateURI string, placeholders ...string) string {
	tweets := `<aside><a href="https://x.example/example/status/1">A post elsewhere</a><a href="https://x.example/example/status/2">Another post elsewhere</a></aside>`
	script := `<script src="/livewire/livewire.min.js?id=1a2b" data-csrf="token-123" data-update-uri="` + updateURI + `" data-navigate-once="true"></script>`
	return page("Blog | Example Publication", siteNav+`<main>`+strings.Join(placeholders, "")+`</main>`+tweets+siteFooter+script)
}

func lazyComponent(param, inner string) string {
	return `<section wire:snapshot="` + html.EscapeString(snapshotJSON(param)) + `" wire:id="c-` + param + `"><div class="layout">` + inner + `</div></section>`
}

const lazyParam = "eyJsaXN0IjoicG9zdHMifQ"

// lazyBlog serves /blog as a lazy shell whose one component lists n posts.
func lazyBlog(t *testing.T, n int) *lazySite {
	l := newLazySite(t)
	l.addBlog(n)
	l.update = "/app/livewire/update"
	l.set("/blog", lazyShell("/app/livewire/update", placeholder("x-init", lazyParam)))
	l.lists[lazyParam] = lazyComponent(lazyParam, blogList(n))
	return l
}

func TestSetupReadsALivewireLazyList(t *testing.T) {
	l := lazyBlog(t, 8)
	res := setupRecipe(t, l.testSite, "/blog")
	if !res.Healthy() || len(res.Items) != 8 || res.Items[0].Title != postTitles[0] {
		t.Fatalf("reasons %v, items %v", res.Reasons, itemURLs(res.Items))
	}
	if !res.Recipe.Lazy {
		t.Error("recipe does not record that its listing needs the lazy fetch")
	}
	if len(l.posts) != 1 {
		t.Fatalf("%d POSTs, want 1", len(l.posts))
	}
	p := l.posts[0]
	if p.path != "/app/livewire/update" {
		t.Errorf("POST went to %s, want the page's data-update-uri", p.path)
	}
	if p.header.Get("Content-Type") != "application/json" || p.header.Get("X-Livewire") != "1" || p.header.Get("User-Agent") != safehttp.UserAgent {
		t.Errorf("headers = %v", p.header)
	}
	if got := p.header.Get("Cookie"); got != "XSRF-TOKEN=xsrf-value; example_session=session-value" {
		t.Errorf("Cookie = %q, want the listing's cookies as name=value only", got)
	}
	b := p.body
	if b.Token != "token-123" || len(b.Components) != 1 {
		t.Fatalf("body = %+v", b)
	}
	c := b.Components[0]
	if c.Snapshot != snapshotJSON(lazyParam) || string(c.Updates) != "{}" {
		t.Errorf("component = %+v", c)
	}
	if len(c.Calls) != 1 || c.Calls[0].Path != "" || c.Calls[0].Method != "__lazyLoad" || !slices.Equal(c.Calls[0].Params, []string{lazyParam}) {
		t.Errorf("calls = %+v", c.Calls)
	}
}

func TestSetupOfAPageWithoutLazyListsPostsNothing(t *testing.T) {
	l := newLazySite(t)
	l.addBlog(8)
	res := setupRecipe(t, l.testSite, "/blog")
	if len(res.Items) != 8 || res.Recipe.Lazy || len(l.posts) != 0 {
		t.Fatalf("items %d, lazy %v, %d POSTs", len(res.Items), res.Recipe.Lazy, len(l.posts))
	}
}

func TestLazyListIgnoresAnUpdateURIOnAnotherHost(t *testing.T) {
	var otherHits int
	var mu sync.Mutex
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		otherHits++
		mu.Unlock()
	}))
	t.Cleanup(other.Close)
	l := lazyBlog(t, 8)
	l.set("/blog", lazyShell(other.URL+"/app/livewire/update", placeholder("x-init", lazyParam)))
	res := setupRecipe(t, l.testSite, "/blog")
	mu.Lock()
	defer mu.Unlock()
	if otherHits != 0 || len(l.posts) != 0 || res.Recipe.Lazy {
		t.Fatalf("other host hit %d times, %d POSTs, lazy %v", otherHits, len(l.posts), res.Recipe.Lazy)
	}
}

func TestLazyListHonorsRobots(t *testing.T) {
	l := lazyBlog(t, 8)
	l.set("/robots.txt", "User-agent: *\nDisallow: /app/livewire/\n")
	res := setupRecipe(t, l.testSite, "/blog")
	if len(l.posts) != 0 || res.Recipe.Lazy {
		t.Fatalf("%d POSTs to a disallowed path, lazy %v", len(l.posts), res.Recipe.Lazy)
	}
}

func TestLazyListFailureLeavesThePageAsFetched(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reply  string
	}{
		{name: "server error", status: http.StatusInternalServerError, reply: `{"components":[]}`},
		{name: "page expired", status: 419, reply: "Page Expired"},
		{name: "not json", reply: "<html>oops</html>"},
		{name: "no html", reply: `{"components":[{"snapshot":"{}","effects":{"dirty":[]}}]}`},
		{name: "fewer components", reply: `{"components":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := lazyBlog(t, 8)
			l.postStatus, l.reply = tc.status, tc.reply
			s := newSession(l.opts())
			resp := s.get(context.Background(), l.url("/blog"))
			got, ok := s.expandLazy(context.Background(), resp, "", nil)
			if ok || !bytes.Equal(got.body, resp.body) || l.postCount() != 1 {
				t.Fatalf("expanded %v, body changed %v, %d POSTs", ok, !bytes.Equal(got.body, resp.body), l.postCount())
			}
		})
	}
	t.Run("budget spent", func(t *testing.T) {
		l := lazyBlog(t, 8)
		s := newSession(l.opts())
		resp := s.get(context.Background(), l.url("/blog"))
		s.robotsFor(context.Background(), l.srv.URL)
		s.budget = 0
		if got, ok := s.expandLazy(context.Background(), resp, "", nil); ok || !bytes.Equal(got.body, resp.body) || l.postCount() != 0 {
			t.Fatalf("expanded %v, %d POSTs with no budget left", ok, l.postCount())
		}
	})
}

func TestLazyListsGoInOneRequest(t *testing.T) {
	l := newLazySite(t)
	params := []string{"eyJhIjoxfQ", "eyJiIjoyfQ", "eyJjIjozfQ", "eyJkIjo0fQ", "eyJlIjo1fQ"}
	var holders []string
	for i, p := range params {
		attr := "x-init"
		if i%2 == 1 {
			attr = "x-intersect"
		}
		holders = append(holders, placeholder(attr, p))
		l.lists[p] = lazyComponent(p, `<p>component `+p+`</p>`)
	}
	l.set("/blog", lazyShell("/livewire/update", holders...))
	s := newSession(l.opts())
	resp := s.get(context.Background(), l.url("/blog"))
	got, ok := s.expandLazy(context.Background(), resp, "", nil)
	if !ok || l.postCount() != 1 {
		t.Fatalf("expanded %v, %d POSTs, want one", ok, l.postCount())
	}
	if n := len(l.posts[0].body.Components); n != maxLazyComponents {
		t.Fatalf("%d components in the request, want %d", n, maxLazyComponents)
	}
	body := string(got.body)
	for i, p := range params[:maxLazyComponents] {
		if l.posts[0].body.Components[i].Calls[0].Params[0] != p || !strings.Contains(body, "component "+p) {
			t.Errorf("component %d (%s) missing from the request or the page", i, p)
		}
	}
	if strings.Count(body, "__lazyLoad") != 1 || strings.Count(body, `class="spinner"`) != 1 {
		t.Errorf("the four sent placeholders were not all replaced:\n%s", body)
	}
}

func TestPollReadsALivewireLazyListUnconditionally(t *testing.T) {
	l := lazyBlog(t, 8)
	res := setupRecipe(t, l.testSite, "/blog")
	in := PollInput{Recipe: res.Recipe, History: historyFrom(res), ETag: l.etag, LastModified: "Fri, 09 Oct 2026 08:00:00 GMT"}
	ctx := context.Background()

	first, err := Poll(ctx, in, l.opts())
	if err != nil || first.Verdict != PollOK || first.NotModified || len(first.Items) != 8 {
		t.Fatalf("first poll = %+v, %v", first, err)
	}
	last := l.gets[len(l.gets)-1]
	if last.Get("If-None-Match") != "" || last.Get("If-Modified-Since") != "" {
		t.Errorf("a lazy listing's poll sent validators: %v", last)
	}
	if !strings.Contains(string(first.Page()), "/blog/"+slugify(postTitles[0])) {
		t.Error("the poll's page for repair and the weekly check is the shell, not the expanded list")
	}
	if first.Excerpts[first.Items[0].URL] == "" {
		t.Error("no excerpt from the expanded list")
	}

	fresh := `<ul class="posts"><li class="post"><h2><a href="/blog/a-brand-new-post-today">A brand new post today</a></h2><time datetime="2024-03-25">March 25, 2024</time></li>`
	l.lmu.Lock()
	l.lists[lazyParam] = lazyComponent(lazyParam, strings.Replace(blogList(8), `<ul class="posts">`, fresh, 1))
	l.lmu.Unlock()
	in.LinkSetHash = first.LinkSetHash
	second, err := Poll(ctx, in, l.opts())
	if err != nil || second.NotModified || len(second.Items) != 9 || !slices.Contains(itemURLs(second.Items), l.url("/blog/a-brand-new-post-today")) {
		t.Fatalf("poll after the list changed behind an identical shell = %+v, %v", second, err)
	}
}

func TestLazyListPostFollowsNoRedirect(t *testing.T) {
	var otherHits int
	var mu sync.Mutex
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		otherHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"components":[{"effects":{"html":"<p>relayed</p>"}}]}`)
	}))
	t.Cleanup(other.Close)
	l := lazyBlog(t, 8)
	l.redirect = other.URL + "/livewire/update"

	s := newSession(l.opts())
	resp := s.get(context.Background(), l.url("/blog"))
	got, ok := s.expandLazy(context.Background(), resp, "", nil)
	mu.Lock()
	defer mu.Unlock()
	if ok || otherHits != 0 || !bytes.Equal(got.body, resp.body) {
		t.Fatalf("expanded %v, other host received %d POSTs; a redirect must not carry the POST anywhere", ok, otherHits)
	}
}

func TestLazyListNeedsAPostClient(t *testing.T) {
	l := lazyBlog(t, 8)
	opts := l.opts()
	opts.PostClient = nil
	s := newSession(opts)
	resp := s.get(context.Background(), l.url("/blog"))
	if got, ok := s.expandLazy(context.Background(), resp, "", nil); ok || !bytes.Equal(got.body, resp.body) || l.postCount() != 0 {
		t.Fatalf("expanded %v, %d POSTs without a client that refuses redirects", ok, l.postCount())
	}
}

func TestPollOfALazyListWhosePostFailsIsUnreachable(t *testing.T) {
	l := lazyBlog(t, 8)
	res := setupRecipe(t, l.testSite, "/blog")
	in := PollInput{Recipe: res.Recipe, History: historyFrom(res), ETag: l.etag}
	ctx := context.Background()
	first, err := Poll(ctx, in, l.opts())
	if err != nil || first.Verdict != PollOK {
		t.Fatalf("first poll = %+v, %v", first, err)
	}

	l.lmu.Lock()
	l.postStatus, l.reply = http.StatusInternalServerError, "oops"
	l.lmu.Unlock()
	in.LinkSetHash = first.LinkSetHash
	got, err := Poll(ctx, in, l.opts())
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != PollBroken || got.Reason != ReasonUnreachable || got.Blocked || got.NotModified {
		t.Fatalf("verdict %s reason %s blocked %v not modified %v; a failed lazy load is the site being unreachable, not a changed page",
			got.Verdict, got.Reason, got.Blocked, got.NotModified)
	}
}

func TestPollOfALazyRecipeReadsAListTheSiteNowServes(t *testing.T) {
	l := lazyBlog(t, 8)
	res := setupRecipe(t, l.testSite, "/blog")
	l.set("/blog", page("Blog | Example Publication", siteNav+blogList(8)+siteFooter))
	got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, l.opts())
	if err != nil || got.Verdict != PollOK || len(got.Items) != 8 {
		t.Fatalf("poll = %+v, %v; want the served list read without a lazy load", got, err)
	}
}

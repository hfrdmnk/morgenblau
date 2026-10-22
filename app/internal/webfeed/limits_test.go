package webfeed

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// countingDoer records how many body bytes the pipeline read from each path.
type countingDoer struct {
	next  Doer
	mu    sync.Mutex
	read  map[string]int64
	total int64
}

func newCountingDoer(next Doer) *countingDoer {
	return &countingDoer{next: next, read: map[string]int64{}}
}

func (c *countingDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.next.Do(req)
	if err != nil {
		return resp, err
	}
	resp.Body = &countingBody{ReadCloser: resp.Body, add: func(n int) {
		c.mu.Lock()
		c.read[req.URL.Path] += int64(n)
		c.total += int64(n)
		c.mu.Unlock()
	}}
	return resp, nil
}

type countingBody struct {
	io.ReadCloser
	add func(int)
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.add(n)
	return n, err
}

// paddedArticle is a post page followed by size bytes of markup nobody needs.
func paddedArticle(title string, size int) string {
	return strings.Replace(article(title, title, "2024-03-20T09:00:00Z"), "</main>", "</main><!--"+strings.Repeat("x", size)+"-->", 1)
}

func TestSetupCapsEachArticleBody(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	for i := range 5 {
		s.set("/blog/"+slugify(postTitles[i]), paddedArticle(postTitles[i], 2<<20))
	}
	doer := newCountingDoer(s.srv.Client())
	opts := s.opts()
	opts.Client = doer
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Healthy() || len(res.Items) != 5 {
		t.Fatalf("reasons %v, items %d", res.Reasons, len(res.Items))
	}
	for i := range 5 {
		if n := doer.read["/blog/"+slugify(postTitles[i])]; n > maxArticleBody {
			t.Errorf("article %d: read %d bytes, want at most %d", i, n, maxArticleBody)
		}
	}
}

func TestSetupStaysWithinItsByteBudget(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(10)
	for i := range 10 {
		s.set("/blog/"+slugify(postTitles[i]), paddedArticle(postTitles[i], 100<<10))
	}
	doer := newCountingDoer(s.srv.Client())
	opts := s.opts()
	opts.Client = doer
	opts.MaxSetupBytes = 300 << 10
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 10 {
		t.Fatalf("items = %d, want the listing's posts even when the budget stops article fetches", len(res.Items))
	}
	if doer.total > opts.MaxSetupBytes {
		t.Errorf("read %d bytes, want at most the %d byte budget", doer.total, opts.MaxSetupBytes)
	}
}

func TestFetchArticlesKeepsNeitherBodiesNorParsedPages(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(3)
	sess := newSession(s.opts())
	var urls []string
	for i := range 3 {
		urls = append(urls, s.url("/blog/"+slugify(postTitles[i])))
	}
	infos := sess.fetchArticles(context.Background(), urls)
	for _, u := range urls {
		if info := infos[u]; info == nil || !info.ok || info.page != nil {
			t.Errorf("%s: info = %+v, want an extracted article without its parsed page", u, info)
		}
		for key, r := range sess.cache {
			if strings.HasSuffix(key, u) && len(r.body) > 0 {
				t.Errorf("%s: the session still holds the article body", u)
			}
		}
	}
	again := sess.fetchArticles(context.Background(), urls[:1])
	if s.hits["/blog/"+slugify(postTitles[0])] != 1 || again[urls[0]] == nil || !again[urls[0]].ok {
		t.Errorf("a second ask fetched the article again or lost it: hits %d", s.hits["/blog/"+slugify(postTitles[0])])
	}
}

// longList is n distinct dated posts, newest first.
func longList(n int) string {
	var b strings.Builder
	b.WriteString(`<main><h1>Blog</h1><ul class="posts">`)
	for i := range n {
		fmt.Fprintf(&b, `<li class="post"><h2><a href="/blog/post-number-%d">Post number %d about calm reading</a></h2><time datetime="2024-%02d-%02d">date</time></li>`,
			i, i, 12-i/28%12, 28-i%28)
	}
	b.WriteString(`</ul></main>`)
	return b.String()
}

func TestItemsAreCappedPerSource(t *testing.T) {
	s := newTestSite(t)
	s.set("/blog", page("Blog", siteNav+longList(250)+siteFooter))
	res := setupRecipe(t, s, "/blog")
	if len(res.Items) != MaxItems {
		t.Errorf("setup items = %d, want %d", len(res.Items), MaxItems)
	}
	body, _ := s.get(t, "/blog")
	if items, err := apply(res.Recipe, body, s.url("/blog")); err != nil || len(items) != MaxItems {
		t.Errorf("streams apply = %d items, %v; want %d", len(items), err, MaxItems)
	}
	css := Recipe{Mode: ModeCSS, CSS: &CSSRecipe{Item: "li.post", Link: "a"}}
	if items, err := apply(css, body, s.url("/blog")); err != nil || len(items) != MaxItems {
		t.Errorf("css apply = %d items, %v; want %d", len(items), err, MaxItems)
	}
	var locs []string
	for i := range 250 {
		locs = append(locs, fmt.Sprintf("https://news.example.com/news/post-number-%d", i))
	}
	sm := Recipe{Mode: ModeSitemap, ListingURL: "https://news.example.com/news", Streams: []Stream{{URLTemplate: "news.example.com/news/*"}}}
	if items, err := apply(sm, []byte(sitemapXML(locs...)), "https://news.example.com/sitemap.xml"); err != nil || len(items) != MaxItems {
		t.Errorf("sitemap apply = %d items, %v; want %d", len(items), err, MaxItems)
	}

	blocked := newTestSite(t)
	blocked.status["/news"] = 403
	var many []string
	for i := range 250 {
		many = append(many, blocked.url(fmt.Sprintf("/news/post-number-%d", i)))
	}
	blocked.set("/sitemap.xml", sitemapXML(many...))
	if res := setupRecipe(t, blocked, "/news"); len(res.Items) != MaxItems {
		t.Errorf("sitemap setup items = %d, want %d", len(res.Items), MaxItems)
	}
}

func TestSitemapsDropLocationsThatAreNotWebPages(t *testing.T) {
	body := sitemapXML(
		"https://news.example.com/news/first-post",
		"javascript://news.example.com/news/%0aalert(1)",
		"data:text/html,hello",
		"ftp://news.example.com/news/file",
		"https://news.example.com/news/second-post",
	)
	sm, err := parseSitemap([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range sm.urls {
		got = append(got, u.loc)
	}
	if want := "https://news.example.com/news/first-post https://news.example.com/news/second-post"; strings.Join(got, " ") != want {
		t.Errorf("sitemap urls = %v", got)
	}

	s := newTestSite(t)
	s.status["/news"] = 403
	var evil []string
	for i := range 5 {
		evil = append(evil, fmt.Sprintf("javascript://%s/news/%%0aalert(%d)", s.host(), i))
	}
	s.set("/sitemap.xml", sitemapXML(append(evil, s.url("/news/one-real-post"))...))
	res := setupRecipe(t, s, "/news")
	for _, it := range res.Items {
		if !strings.HasPrefix(it.URL, "http") {
			t.Errorf("setup kept %q", it.URL)
		}
	}
}

func TestSessionKeysItsCacheOnTheLanguage(t *testing.T) {
	s := newTestSite(t)
	s.set("/page", page("Page", "<p>hello</p>"))
	sess := newSession(s.opts())
	ctx := context.Background()
	sess.get(ctx, s.url("/page"))
	sess.setLang("de")
	sess.get(ctx, s.url("/page"))
	if s.hits["/page"] != 2 || s.langs["/page"] != "de" {
		t.Errorf("hits %d, last Accept-Language %q; want a German fetch apart from the first", s.hits["/page"], s.langs["/page"])
	}
	sess.get(ctx, s.url("/page"))
	if s.hits["/page"] != 2 {
		t.Errorf("hits %d, want the German response reused", s.hits["/page"])
	}
}

func TestSetupSendsOnlyAWellFormedPageLanguage(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	body, _ := s.get(t, "/blog")
	s.set("/blog", strings.Replace(string(body), `lang="en"`, `lang="de, en;q=0.5"`, 1))
	res := setupRecipe(t, s, "/blog")
	if res.Recipe.Lang != "" {
		t.Errorf("recipe lang = %q, want a malformed lang dropped", res.Recipe.Lang)
	}
	if got := s.langs["/blog/"+slugify(postTitles[0])]; got != "" {
		t.Errorf("article Accept-Language = %q, want none", got)
	}
	sess := newSession(s.opts())
	sess.setLang("en\tx")
	sess.get(context.Background(), s.url("/robots.txt"))
	if got := s.langs["/robots.txt"]; got != "" {
		t.Errorf("Accept-Language = %q, want a malformed session language left out", got)
	}
}

func TestSetupReturnsTeasersFromTheListingItAlreadyParsed(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	res := setupRecipe(t, s, "/blog")
	if got := res.Excerpts[res.Items[0].URL]; !strings.Contains(got, "A short excerpt") {
		t.Errorf("excerpt = %q, want the listing's teaser", got)
	}
	if s.hits["/blog"] != 1 {
		t.Errorf("listing fetched %d times", s.hits["/blog"])
	}
}

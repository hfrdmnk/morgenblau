package feedfinder

import (
	"context"
	"testing"

	"morgenblau/internal/safehttp"
)

const advertisedPostsFeed = `<link rel="alternate" type="application/rss+xml" title="Example Publication" href="/blog/index.xml">`

func resolveNone(t *testing.T, rawURL string) {
	t.Helper()
	cands, err := discoverFinder().Resolve(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
}

func TestResolve_HomepageWithoutAFeedFindsTheFeedItsBlogAdvertises(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/":               htmlPage(`<html><body><nav><a href="/about">About</a><a href="/blog">Blog</a></nav><p>Hello.</p></body></html>`),
		"/blog":           htmlPage(`<html><head>` + advertisedPostsFeed + `</head><body>` + listingPosts + `</body></html>`),
		"/blog/index.xml": rssPage("Example Publication", "https://blog.example.com/posts/first"),
	})

	got := resolveOne(t, srv.URL+"/")
	if got.FeedURL != srv.URL+"/blog/index.xml" {
		t.Errorf("FeedURL = %q, want the blog's advertised feed", got.FeedURL)
	}
	if got.Title != "Example Publication" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.SiteURL != srv.URL+"/" {
		t.Errorf("SiteURL = %q, want the pasted site, not the section", got.SiteURL)
	}
	if srv.requested("/about") {
		t.Error("/about is no section and must not be fetched")
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, r := range srv.requests {
		if r.URL.Path == "/blog" && r.Header.Get("User-Agent") != safehttp.UserAgent {
			t.Errorf("section User-Agent = %q", r.Header.Get("User-Agent"))
		}
	}
}

func TestResolve_SectionWithOnlyAHiddenFeedHint(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/":                    htmlPage(`<html><body><a href="/posts/">Posts</a></body></html>`),
		"/posts/":              htmlPage(`<html><body><a href="/posts/subscribe.xml">RSS</a>` + listingPosts + `</body></html>`),
		"/posts/subscribe.xml": rssPage("Example Publication", "https://blog.example.com/posts/first", "https://blog.example.com/posts/second"),
	})

	got := resolveOne(t, srv.URL+"/")
	if got.FeedURL != srv.URL+"/posts/subscribe.xml" {
		t.Errorf("FeedURL = %q, want the section's hidden feed", got.FeedURL)
	}
	if got.SiteURL != srv.URL+"/" {
		t.Errorf("SiteURL = %q, want the pasted site", got.SiteURL)
	}
}

func TestResolve_SectionLinkShapes(t *testing.T) {
	cases := []struct {
		name, pasted, href, sectionPath string
	}{
		{"dot-relative", "/", "./writing", "/writing"},
		{"bare relative", "/", "writing", "/writing"},
		{"root without a slash", "", "writing", "/writing"},
		{"trailing slash", "/", "/writing/", "/writing/"},
		{"query and fragment", "/", "/writing?ref=nav#top", "/writing"},
		{"capitalised", "/", "/Writing", "/Writing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newSiteServer(t, map[string]page{
				"/":            htmlPage(`<html><body><a href="` + tc.href + `">Writing</a></body></html>`),
				tc.sectionPath: htmlPage(`<html><head>` + advertisedPostsFeed + `</head><body></body></html>`),
			})

			got := resolveOne(t, srv.URL+tc.pasted)
			if got.FeedURL != srv.URL+"/blog/index.xml" {
				t.Errorf("FeedURL = %q", got.FeedURL)
			}
		})
	}
}

func TestResolve_NoSectionLinkStaysUnresolvable(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/": htmlPage(`<html><body>
<a href="/about">About</a>
<a href="/blog/2024/first-post">A post</a>
<a href="/projects">Projects</a>
<a href="mailto:blog@example.com">Mail</a>
</body></html>`),
	})

	resolveNone(t, srv.URL+"/")
	for _, p := range []string{"/about", "/blog/2024/first-post", "/projects"} {
		if srv.requested(p) {
			t.Errorf("%s was fetched as a section", p)
		}
	}
}

func TestResolve_SectionLinkToAnotherHostIsIgnored(t *testing.T) {
	other := newSiteServer(t, map[string]page{
		"/blog": htmlPage(`<html><head>` + advertisedPostsFeed + `</head></html>`),
	})
	srv := newSiteServer(t, map[string]page{
		"/": htmlPage(`<html><body><a href="` + other.URL + `/blog">Blog</a></body></html>`),
	})

	resolveNone(t, srv.URL+"/")
	if len(other.paths()) != 0 {
		t.Errorf("other host got %v, want no requests", other.paths())
	}
}

func TestResolve_TriesOnlyTheFirstTwoSections(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/":       htmlPage(`<html><body><a href="/notes">Notes</a><a href="/notes/">Notes again</a><a href="/essays">Essays</a><a href="/blog">Blog</a></body></html>`),
		"/notes":  htmlPage(`<html><body></body></html>`),
		"/essays": htmlPage(`<html><body></body></html>`),
		"/blog":   htmlPage(`<html><head>` + advertisedPostsFeed + `</head></html>`),
		"/notes/": htmlPage(`<html><head>` + advertisedPostsFeed + `</head></html>`),
	})

	resolveNone(t, srv.URL+"/")
	if !srv.requested("/notes") || !srv.requested("/essays") {
		t.Errorf("requests = %v, want /notes and /essays tried", srv.paths())
	}
	for _, p := range []string{"/blog", "/notes/"} {
		if srv.requested(p) {
			t.Errorf("%s was fetched past the two-section limit", p)
		}
	}
}

func TestResolve_ASubpageNeverHopsToASiblingSection(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":      htmlPage(`<html><body><nav><a href="/blog">Blog</a><a href="/changelog">Changelog</a></nav>` + listingPosts + `</body></html>`),
		"/changelog": htmlPage(`<html><head><link rel="alternate" type="application/rss+xml" href="/changelog/rss.xml"></head></html>`),
	})

	resolveNone(t, srv.URL+"/blog")
	if srv.requested("/changelog") {
		t.Error("/changelog is another stream than the pasted listing and must not be fetched")
	}
}

func TestResolve_HomepageWithAFeedNeverFetchesTheSection(t *testing.T) {
	cases := map[string]map[string]page{
		"advertised": {
			"/": htmlPage(`<html><head><link rel="alternate" type="application/rss+xml" href="/rss.xml"></head><body><a href="/blog">Blog</a></body></html>`),
		},
		"hidden": {
			"/":           htmlPage(`<html><body><a href="/blog">Blog</a><a href="/s/feed.xml">RSS</a></body></html>`),
			"/s/feed.xml": rssPage("Example Publication"),
		},
	}
	for name, pages := range cases {
		t.Run(name, func(t *testing.T) {
			pages["/blog"] = htmlPage(`<html><head>` + advertisedPostsFeed + `</head></html>`)
			srv := newSiteServer(t, pages)

			resolveOne(t, srv.URL+"/")
			if srv.requested("/blog") {
				t.Error("/blog was fetched although the homepage has a feed")
			}
		})
	}
}

func TestResolve_RootFeedRejectedOnTheHomepageIsReweighedOnTheSectionWithoutRefetching(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/": htmlPage(`<html><body><a href="/blog">Blog</a><ul>
<li><a href="https://blog.example.com/projects/a">Project A</a></li>
<li><a href="https://blog.example.com/projects/b">Project B</a></li>
</ul></body></html>`),
		"/blog":    htmlPage(`<html><body>` + listingPosts + `</body></html>`),
		"/rss.xml": rssPage("Example Publication", "https://blog.example.com/posts/first", "https://blog.example.com/posts/second"),
	})

	got := resolveOne(t, srv.URL+"/")
	if got.FeedURL != srv.URL+"/rss.xml" {
		t.Errorf("FeedURL = %q, want the root feed the blog listing shares posts with", got.FeedURL)
	}
	if got.SiteURL != srv.URL+"/" {
		t.Errorf("SiteURL = %q, want the pasted site", got.SiteURL)
	}
	counts := map[string]int{}
	for _, p := range srv.paths() {
		counts[p]++
	}
	for p, n := range counts {
		if n > 1 {
			t.Errorf("%s fetched %d times, want at most once", p, n)
		}
	}
	if !srv.requested("/blog/rss.xml") {
		t.Error("the section's own guessed paths were never probed")
	}
}

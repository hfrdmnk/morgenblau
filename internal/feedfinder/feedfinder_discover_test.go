package feedfinder

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"morgenblau/internal/safehttp"
)

type page struct {
	status      int
	contentType string
	body        string
	redirectTo  string
}

// siteServer serves fixed pages by path, 404s everything else, and records every request.
type siteServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newSiteServer(t *testing.T, pages map[string]page) *siteServer {
	t.Helper()
	s := &siteServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Clone(context.Background()))
		s.mu.Unlock()
		p, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if p.redirectTo != "" {
			http.Redirect(w, r, p.redirectTo, http.StatusMovedPermanently)
			return
		}
		if p.contentType != "" {
			w.Header().Set("Content-Type", p.contentType)
		}
		if p.status != 0 {
			w.WriteHeader(p.status)
		}
		_, _ = fmt.Fprint(w, p.body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *siteServer) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.requests))
	for _, r := range s.requests {
		out = append(out, r.URL.Path)
	}
	return out
}

func (s *siteServer) requested(path string) bool {
	for _, p := range s.paths() {
		if p == path {
			return true
		}
	}
	return false
}

func discoverFinder() *Finder {
	return New(safehttp.NewClient(5*time.Second, 5, safehttp.WithAllowLoopback()))
}

func htmlPage(body string) page {
	return page{contentType: "text/html; charset=utf-8", body: body}
}

func rssPage(title string, itemLinks ...string) page {
	var items strings.Builder
	for _, l := range itemLinks {
		fmt.Fprintf(&items, "<item><title>Post</title><link>%s</link></item>", l)
	}
	return page{
		contentType: "application/rss+xml",
		body: fmt.Sprintf(`<?xml version="1.0"?><rss version="2.0"><channel><title>%s</title><link>https://blog.example.com/</link>%s</channel></rss>`,
			title, items.String()),
	}
}

func atomPage(title string, itemLinks ...string) page {
	var entries strings.Builder
	for _, l := range itemLinks {
		fmt.Fprintf(&entries, `<entry><title>Post</title><link href="%s"/><id>%s</id></entry>`, l, l)
	}
	return page{
		contentType: "application/atom+xml",
		body:        fmt.Sprintf(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>%s</title>%s</feed>`, title, entries.String()),
	}
}

func resolveOne(t *testing.T, rawURL string) Candidate {
	t.Helper()
	cands, err := discoverFinder().Resolve(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(cands), cands)
	}
	return cands[0]
}

const listingPosts = `<ul>
<li><a href="https://blog.example.com/posts/first">First</a></li>
<li><a href="https://blog.example.com/posts/second">Second</a></li>
</ul>`

func TestResolve_DiscoversFeedsHiddenInPageMarkup(t *testing.T) {
	cases := []struct {
		name   string
		markup string
	}{
		{"anchor text", `<a href="/s/updates">RSS</a>`},
		{"anchor title", `<a href="/s/updates" title="Subscribe via Atom">Subscribe</a>`},
		{"anchor aria-label", `<a href="/s/updates" aria-label="Atom feed"><svg></svg></a>`},
		{"icon-only anchor", `<a href="/s/updates"><img src="/img/rss.svg" alt=""></a>`},
		{"anchor class", `<a href="/s/updates" class="rss-link">Subscribe</a>`},
		{"icon class anchor", `<a href="/s/updates"><i class="fa fa-rss"></i></a>`},
		{"anchor href", `<a href="/s/feed.xml">Subscribe</a>`},
		{"untyped alternate link", `<link rel="alternate" href="/s/updates" title="RSS">`},
		{"icon link", `<link rel="icon" href="/s/rss">`},
		{"meta rss-feed", `<meta name="rss-feed" content="/s/updates">`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			feedPath := "/s/updates"
			if strings.Contains(tc.markup, `"/s/feed.xml"`) {
				feedPath = "/s/feed.xml"
			} else if strings.Contains(tc.markup, `"/s/rss"`) {
				feedPath = "/s/rss"
			}
			srv := newSiteServer(t, map[string]page{
				"/blog":  htmlPage(`<html><head>` + tc.markup + `</head><body>` + tc.markup + listingPosts + `</body></html>`),
				feedPath: rssPage("Example Publication", "https://blog.example.com/posts/first"),
			})

			got := resolveOne(t, srv.URL+"/blog")
			if got.FeedURL != srv.URL+feedPath {
				t.Errorf("FeedURL = %q, want %q", got.FeedURL, srv.URL+feedPath)
			}
			if got.Title != "Example Publication" {
				t.Errorf("Title = %q, want the feed's own title", got.Title)
			}
			if got.SiteURL != srv.URL+"/blog" {
				t.Errorf("SiteURL = %q, want the listing page", got.SiteURL)
			}
			if got.ContentType != "application/rss+xml" {
				t.Errorf("ContentType = %q, want the probe response's", got.ContentType)
			}
		})
	}
}

func TestResolve_ProbesCommonPathsUnderTheListingPath(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/news/":         htmlPage(`<html><body>` + listingPosts + `</body></html>`),
		"/news/atom.xml": atomPage("Example News", "https://blog.example.com/posts/first"),
	})

	got := resolveOne(t, srv.URL+"/news/")
	if got.FeedURL != srv.URL+"/news/atom.xml" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
	if got.Title != "Example News" || got.ContentType != "application/atom+xml" {
		t.Errorf("candidate = %+v", got)
	}
}

func TestResolve_ProbesCommonPathsAtTheSiteRoot(t *testing.T) {
	for _, feedPath := range []string{"/feed/", "/index.xml", "/news/rss.xml"} {
		t.Run(feedPath, func(t *testing.T) {
			srv := newSiteServer(t, map[string]page{
				"/journal/posts": htmlPage(`<html><body>` + listingPosts + `</body></html>`),
				feedPath:         rssPage("Example Publication"),
			})

			got := resolveOne(t, srv.URL+"/journal/posts")
			if got.FeedURL != srv.URL+feedPath {
				t.Errorf("FeedURL = %q, want %q", got.FeedURL, srv.URL+feedPath)
			}
		})
	}
}

func TestResolve_RefusedListingStillFindsTheRootFeed(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":    {status: http.StatusForbidden, contentType: "text/html", body: `<html><body>Access denied</body></html>`},
		"/rss.xml": rssPage("Example Publication"),
	})

	got := resolveOne(t, srv.URL+"/blog")
	if got.FeedURL != srv.URL+"/rss.xml" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
}

func TestResolve_ProbeMustParseAsAFeed(t *testing.T) {
	// The content type claims RSS, but the body is the site's HTML shell.
	srv := newSiteServer(t, map[string]page{
		"/blog": htmlPage(`<html><body><a href="/feed">RSS</a>` + listingPosts + `</body></html>`),
		"/feed": {contentType: "application/rss+xml", body: `<html><body>Our newest posts</body></html>`},
	})

	cands, err := discoverFinder().Resolve(context.Background(), srv.URL+"/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
	if !srv.requested("/feed") {
		t.Error("/feed was never probed")
	}
}

func TestResolve_NonSuccessProbeIsAMiss(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":    htmlPage(`<html><body>` + listingPosts + `</body></html>`),
		"/rss.xml": {status: http.StatusInternalServerError, contentType: "application/rss+xml", body: rssPage("Example Publication").body},
	})

	cands, err := discoverFinder().Resolve(context.Background(), srv.URL+"/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
}

func TestResolve_WordsThatOnlyContainFeedAreNotProbed(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog": htmlPage(`<html><body>
<a href="/feedback">Send feedback</a>
<a href="/s/contact" title="Feedback">Contact</a>
<a href="/s/anatomy">Anatomy</a>
` + listingPosts + `</body></html>`),
	})

	cands, err := discoverFinder().Resolve(context.Background(), srv.URL+"/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
	for _, p := range []string{"/feedback", "/s/contact", "/s/anatomy"} {
		if srv.requested(p) {
			t.Errorf("%s was probed as a feed", p)
		}
	}
}

func TestResolve_PicksTheFeedThatLinksTheListingsPosts(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog": htmlPage(`<html><body>
<a href="/s/comments.xml">Comments RSS</a>
<a href="/s/all-the-posts.xml">Posts RSS</a>
<ul>
<li><a href="https://www.blog.example.com/posts/first/">First</a></li>
<li><a href="https://blog.example.com/posts/second#top">Second</a></li>
<li><a href="https://BLOG.example.com/posts/third">Third</a></li>
</ul></body></html>`),
		"/s/comments.xml": rssPage("Example Comments",
			"https://blog.example.com/comments/1", "https://blog.example.com/comments/2",
			"https://blog.example.com/comments/3", "https://blog.example.com/posts/first#comment-1"),
		"/s/all-the-posts.xml": rssPage("Example Publication",
			"https://blog.example.com/posts/first", "https://blog.example.com/posts/second/", "https://blog.example.com/posts/third"),
		"/rss.xml": rssPage("Example Everything", "https://blog.example.com/posts/first"),
	})

	got := resolveOne(t, srv.URL+"/blog")
	if got.FeedURL != srv.URL+"/s/all-the-posts.xml" {
		t.Errorf("FeedURL = %q, want the posts feed", got.FeedURL)
	}
}

func TestResolve_OverlapTiePrefersAFeedThePageLinks(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":             htmlPage(`<html><body><a href="/s/everything.xml">RSS</a>` + listingPosts + `</body></html>`),
		"/s/everything.xml": rssPage("Example Publication"),
		"/rss.xml":          rssPage("Example Root"),
	})

	got := resolveOne(t, srv.URL+"/blog")
	if got.FeedURL != srv.URL+"/s/everything.xml" {
		t.Errorf("FeedURL = %q, want the page-linked feed", got.FeedURL)
	}
}

func TestResolve_OverlapTiePrefersTheShorterURL(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":     htmlPage(`<html><body>` + listingPosts + `</body></html>`),
		"/feed.xml": rssPage("Example Long"),
		"/rss":      rssPage("Example Short"),
	})

	got := resolveOne(t, srv.URL+"/blog")
	if got.FeedURL != srv.URL+"/rss" {
		t.Errorf("FeedURL = %q, want the shorter URL", got.FeedURL)
	}
}

func TestResolve_ProbesThatRedirectToTheSameFeedCollapse(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":     htmlPage(`<html><body><a href="/feed.xml">RSS</a>` + listingPosts + `</body></html>`),
		"/feed.xml": rssPage("Example Publication"),
		"/feed":     {redirectTo: "/feed.xml"},
		"/rss":      {redirectTo: "/feed.xml"},
	})

	got := resolveOne(t, srv.URL+"/blog")
	if got.FeedURL != srv.URL+"/feed.xml" {
		t.Errorf("FeedURL = %q, want the final URL", got.FeedURL)
	}
}

func TestResolve_NoFeedAnywhereIsUnresolvable(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog": htmlPage(`<html><body>` + listingPosts + `</body></html>`),
	})

	cands, err := discoverFinder().Resolve(context.Background(), srv.URL+"/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
}

func TestResolve_AdvertisedFeedsSkipDiscovery(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog": htmlPage(`<html><head>
<link rel="alternate" type="application/rss+xml" title="Posts" href="/s/posts.xml">
</head><body><a href="/s/other.xml">RSS</a><meta name="rss-feed" content="/s/meta.xml">` + listingPosts + `</body></html>`),
		"/rss.xml": rssPage("Example Root"),
	})

	cands, err := discoverFinder().Resolve(context.Background(), srv.URL+"/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 || cands[0].FeedURL != srv.URL+"/s/posts.xml" || cands[0].Title != "Posts" {
		t.Errorf("cands = %+v, want only the advertised feed", cands)
	}
	if got := srv.paths(); len(got) != 1 {
		t.Errorf("requests = %v, want only the listing page", got)
	}
}

func TestResolve_ProbesSendThePageLanguage(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":    htmlPage(`<html lang="de-AT"><body>` + listingPosts + `</body></html>`),
		"/rss.xml": rssPage("Example Publication"),
	})

	resolveOne(t, srv.URL+"/blog")

	srv.mu.Lock()
	defer srv.mu.Unlock()
	probes := 0
	for _, r := range srv.requests {
		if r.URL.Path == "/blog" {
			continue
		}
		probes++
		if got := r.Header.Get("Accept-Language"); got != "de-AT" {
			t.Errorf("%s Accept-Language = %q, want de-AT", r.URL.Path, got)
		}
		if got := r.Header.Get("User-Agent"); got != safehttp.UserAgent {
			t.Errorf("%s User-Agent = %q", r.URL.Path, got)
		}
	}
	if probes == 0 {
		t.Fatal("no probes were sent")
	}
}

func TestResolve_ProbesOmitAMalformedPageLanguage(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":    htmlPage(`<html lang="de, en;q=0.5"><body>` + listingPosts + `</body></html>`),
		"/rss.xml": rssPage("Example Publication"),
	})

	resolveOne(t, srv.URL+"/blog")

	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, r := range srv.requests {
		if got := r.Header.Get("Accept-Language"); got != "" {
			t.Errorf("%s Accept-Language = %q, want none", r.URL.Path, got)
		}
	}
}

func TestResolve_GuessedFeedThatSharesNoPostsWithTheListingIsAnotherStream(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/journal": htmlPage(`<html><body>` + listingPosts + `</body></html>`),
		"/feed":    rssPage("Example Publication Press", "https://blog.example.com/press/unrelated"),
	})

	cands, err := discoverFinder().Resolve(context.Background(), srv.URL+"/journal")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
}

func TestResolve_RefusedListingAcceptsAGuessedFeedWithoutOverlap(t *testing.T) {
	srv := newSiteServer(t, map[string]page{
		"/blog":    {status: http.StatusForbidden, contentType: "text/html", body: `<html><body>Access denied</body></html>`},
		"/rss.xml": rssPage("Example Publication", "https://blog.example.com/posts/first"),
	})

	got := resolveOne(t, srv.URL+"/blog")
	if got.FeedURL != srv.URL+"/rss.xml" {
		t.Errorf("FeedURL = %q", got.FeedURL)
	}
}

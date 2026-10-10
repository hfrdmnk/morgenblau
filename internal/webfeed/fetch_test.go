package webfeed

import (
	"strings"
	"testing"
)

func TestRobotsRules(t *testing.T) {
	txt := `# example
User-agent: *
Disallow: /private/
Allow: /private/blog
Disallow: /*.pdf$

User-agent: SomeOtherBot
Disallow: /

Sitemap: https://news.example.com/sitemap.xml
sitemap: https://news.example.com/posts-sitemap.xml
`
	r := parseRobots(txt)
	for path, want := range map[string]bool{
		"/blog":               true,
		"/private/x":          false,
		"/private/blog/post":  true,
		"/files/report.pdf":   false,
		"/files/report.pdf?x": true,
	} {
		if got := r.allowed(path); got != want {
			t.Errorf("allowed(%q) = %v, want %v", path, got, want)
		}
	}
	if len(r.sitemaps) != 2 || r.sitemaps[1] != "https://news.example.com/posts-sitemap.xml" {
		t.Errorf("sitemaps = %v", r.sitemaps)
	}
}

func TestRobotsPrefersOurOwnGroup(t *testing.T) {
	r := parseRobots("User-agent: *\nDisallow:\n\nUser-agent: Morgenblau\nUser-agent: OtherBot\nDisallow: /news\n")
	if r.allowed("/news") {
		t.Fatal("a group naming Morgenblau must win over *")
	}
	if !parseRobots("User-agent: *\nDisallow:\n").allowed("/news") {
		t.Fatal("empty Disallow allows everything")
	}
}

func TestChallengePages(t *testing.T) {
	cases := map[string]bool{
		`<html><head><title>Just a moment...</title></head><body>Checking your browser</body></html>`:                                                                                            true,
		`<html><head><title>Attention Required! | Cloudflare</title></head></html>`:                                                                                                              true,
		`<html><head><title>News</title><script src="/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page"></script></head>`:                                                                     true,
		`<html><head><title>Access Denied</title></head><body>You don't have permission</body></html>`:                                                                                           true,
		`<html><head><title>Example Publication news</title><script src="https://www.google.com/recaptcha/api.js"></script></head><body>` + strings.Repeat("<p>post</p>", 10) + `</body></html>`: false,
	}
	for body, want := range cases {
		if got := isChallenge([]byte(body)); got != want {
			t.Errorf("isChallenge(%.60q) = %v, want %v", body, got, want)
		}
	}
}

func TestParseSitemap(t *testing.T) {
	urlset := `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://news.example.com/news/first-post</loc><lastmod>2024-03-01</lastmod></url>
<url><loc> https://news.example.com/news/second-post </loc></url></urlset>`
	sm, err := parseSitemap([]byte(urlset))
	if err != nil || len(sm.urls) != 2 || sm.urls[1].loc != "https://news.example.com/news/second-post" || sm.urls[0].lastmod.IsZero() {
		t.Fatalf("urlset = %+v, %v", sm, err)
	}
	index := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://news.example.com/sitemap-posts.xml</loc></sitemap></sitemapindex>`
	sm, err = parseSitemap([]byte(index))
	if err != nil || len(sm.children) != 1 {
		t.Fatalf("index = %+v, %v", sm, err)
	}
}

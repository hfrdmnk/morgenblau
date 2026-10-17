package webfeed

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// teaserHome is a homepage that shows the newest n posts of /blog and links the full list in its nav.
func teaserHome(n int) string {
	return page("Example Publication", siteNav+`<section class="intro"><p>Calm software for careful readers.</p></section>`+blogList(n)+siteFooter)
}

// homeOnly is the homepage's own setup, before any section check, for fixture preconditions.
func homeOnly(t *testing.T, s *testSite, path string) *Result {
	t.Helper()
	res, err := newSession(s.opts()).setup(context.Background(), s.url(path), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSetupMovesATeaserHomepageToItsSection(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	s.set("/", teaserHome(3))
	if home := homeOnly(t, s, "/"); !home.Healthy() || len(home.Items) != 3 {
		t.Fatalf("fixture: homepage healthy %v reasons %v items %d", home.Healthy(), home.Reasons, len(home.Items))
	}
	s.hits = map[string]int{}

	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Recipe
	if rec.ListingURL != s.url("/blog") || len(res.Items) != 5 || !res.Healthy() {
		t.Fatalf("listing %s items %v reasons %v", rec.ListingURL, itemURLs(res.Items), res.Reasons)
	}
	if res.Title != "Example Publication" {
		t.Fatalf("title %q, want the homepage's", res.Title)
	}
	if rec.Setup.Section == nil || rec.Setup.Section.From != s.url("/") || !slices.Contains(rec.Setup.Stages, "section") || !rec.Setup.SectionChecked {
		t.Fatalf("setup record %+v section %+v", rec.Setup, rec.Setup.Section)
	}
	if len(res.Excerpts) != 5 {
		t.Fatalf("excerpts %v, want the section page's five", res.Excerpts)
	}
	if s.hits["/blog"] != 1 || s.hits["/robots.txt"] != 1 {
		t.Fatalf("section fetched %d times, robots.txt %d times", s.hits["/blog"], s.hits["/robots.txt"])
	}
}

func TestSetupKeepsAHealthyHomepageWhoseSectionMissesOneOfItsPosts(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(6)
	s.set("/blog", page("Blog", siteNav+strings.Replace(blogList(6), "/blog/"+slugify(postTitles[0]), "/blog/an-older-post-instead", 1)+siteFooter))
	s.set("/", teaserHome(3))
	if home := homeOnly(t, s, "/"); !home.Healthy() {
		t.Fatalf("fixture: homepage reasons %v", home.Reasons)
	}

	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.ListingURL != s.url("/") || len(res.Items) != 3 || res.Recipe.Setup.Section != nil || slices.Contains(res.Recipe.Setup.Stages, "section") {
		t.Fatalf("listing %s items %v setup %+v", res.Recipe.ListingURL, itemURLs(res.Items), res.Recipe.Setup)
	}
	if !res.Recipe.Setup.SectionChecked {
		t.Fatal("section check not recorded")
	}
}

// weakHome links /writing and shows two root-level posts, too few for a healthy feed.
func weakHome() string {
	return page("Example Publication", `<header><a href="/">Example Publication</a><nav><a href="/about">About</a><a href="/writing">Writing</a></nav></header>`+
		`<main><ul class="latest"><li><h2><a href="/`+slugify(postTitles[0])+`">`+postTitles[0]+`</a></h2></li>`+
		`<li><h2><a href="/`+slugify(postTitles[1])+`">`+postTitles[1]+`</a></h2></li></ul></main>`)
}

func TestSetupMovesAWeakHomepageToANamedSection(t *testing.T) {
	s := newTestSite(t)
	s.set("/", weakHome())
	s.set("/writing", page("Writing", movedList(6, "writing")))
	if home := homeOnly(t, s, "/"); home.Healthy() && len(home.Items) >= 5 {
		t.Fatalf("fixture: homepage is not weak: items %d", len(home.Items))
	}

	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.ListingURL != s.url("/writing") || len(res.Items) != 6 || !res.Healthy() || res.Recipe.Setup.Section == nil {
		t.Fatalf("listing %s items %v reasons %v", res.Recipe.ListingURL, itemURLs(res.Items), res.Reasons)
	}
}

func TestSetupKeepsAWeakHomepageWhenItsSectionIsNoBetter(t *testing.T) {
	s := newTestSite(t)
	s.set("/", weakHome())
	s.set("/writing", page("Writing", movedList(2, "writing")))

	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.ListingURL != s.url("/") || res.Recipe.Setup.Section != nil || !res.Recipe.Setup.SectionChecked {
		t.Fatalf("listing %s setup %+v", res.Recipe.ListingURL, res.Recipe.Setup)
	}
	if s.hits["/writing"] != 1 {
		t.Fatalf("section fetched %d times", s.hits["/writing"])
	}
}

func TestSetupFollowsARelativeSectionLink(t *testing.T) {
	s := newTestSite(t)
	nav := `<header><a href="/">Example Publication</a><nav><a href="./about">About</a><a href="./updates">Updates</a></nav></header>`
	s.set("/", page("Example Publication", nav+movedList(3, "updates")))
	s.set("/updates", page("Updates", nav+movedList(5, "updates")))

	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.ListingURL != s.url("/updates") || len(res.Items) != 5 {
		t.Fatalf("listing %s items %v", res.Recipe.ListingURL, itemURLs(res.Items))
	}
}

func TestSetupKeepsTheHomepageWhenTheSectionCannotBeRead(t *testing.T) {
	cases := map[string]func(*testSite){
		"disallowed by robots.txt": func(s *testSite) { s.set("/robots.txt", "User-agent: *\nDisallow: /blog\n") },
		"missing":                  func(s *testSite) { s.set("/blog", ""); s.status["/blog"] = 404 },
		// A refused section falls back to the sitemap inside setup; a move needs the section page itself.
		"refused with a sitemap": func(s *testSite) {
			s.status["/blog"] = 403
			s.set("/sitemap.xml", sitemapXML(s.url("/blog/one-more-post-here"), s.url("/blog/and-another-post-here"), s.url("/blog/a-third-post-from-the-map"), s.url("/blog/a-fourth-post-from-the-map")))
		},
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			s := newTestSite(t)
			s.addBlog(5)
			s.set("/", teaserHome(3))
			block(s)
			res, err := Setup(context.Background(), s.url("/"), s.opts())
			if err != nil {
				t.Fatal(err)
			}
			if res.Recipe.ListingURL != s.url("/") || len(res.Items) != 3 || res.Recipe.Setup.Section != nil || !res.Recipe.Setup.SectionChecked {
				t.Fatalf("listing %s items %v setup %+v", res.Recipe.ListingURL, itemURLs(res.Items), res.Recipe.Setup)
			}
		})
	}
}

func TestSetupDoesNotFetchADisallowedSection(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	s.set("/", teaserHome(3))
	s.set("/robots.txt", "User-agent: *\nDisallow: /blog\n")
	if _, err := Setup(context.Background(), s.url("/"), s.opts()); err != nil {
		t.Fatal(err)
	}
	if s.hits["/blog"] != 0 {
		t.Fatalf("disallowed section fetched %d times", s.hits["/blog"])
	}
}

func TestSectionCandidate(t *testing.T) {
	nav := `<nav><a href="/about">About</a><a href="/blog/">Blog</a><a href="/news?ref=nav#top">News</a><a href="https://other.example.com/notes">Notes</a></nav>`
	items := func(paths ...string) []Item {
		var out []Item
		for _, p := range paths {
			out = append(out, Item{URL: "https://www.example.com" + p})
		}
		return out
	}
	cases := []struct {
		name    string
		listing string
		body    string
		items   []Item
		weak    bool
		want    string
	}{
		{"teaser", "https://www.example.com/", nav, items("/blog/a-year-in", "/blog/second-post"), false, "https://www.example.com/blog/"},
		{"teaser ignores query and fragment", "https://www.example.com/", nav, items("/news/2024/first", "/news/second"), false, "https://www.example.com/news"},
		{"teaser ignores other hosts' items", "https://www.example.com/", nav, append(items("/blog/a-year-in"), Item{URL: "https://other.example.com/press/x"}), false, "https://www.example.com/blog/"},
		{"posts span two sections", "https://www.example.com/", nav, items("/blog/a-year-in", "/news/second"), false, ""},
		{"posts span two sections when weak", "https://www.example.com/", `<nav><a href="/projects">Projects</a></nav>`, items("/blog/a-year-in", "/news/second"), true, ""},
		{"one-segment posts are no teaser", "https://www.example.com/", nav, items("/blog", "/a-year-in"), false, ""},
		{"no link to the section", "https://www.example.com/", `<nav><a href="/about">About</a></nav>`, items("/blog/a-year-in"), false, ""},
		{"already the section", "https://www.example.com/blog/", nav, items("/blog/a-year-in"), false, ""},
		{"a deeper listing was chosen on purpose", "https://www.example.com/blog/tag/design", nav, items("/blog/a-year-in"), true, ""},
		{"weak follows a blog-like word", "https://www.example.com/", `<nav><a href="/projects">Projects</a><a href="/Writing">Writing</a></nav>`, nil, true, "https://www.example.com/Writing"},
		{"weak only leaves the front page", "https://www.example.com/updates", `<nav><a href="/blog">Blog</a></nav>`, items("/updates/one"), true, ""},
		{"weak leaves an index document", "https://www.example.com/index.html", `<nav><a href="/blog">Blog</a></nav>`, nil, true, "https://www.example.com/blog"},
		{"teaser leaves a one-segment page", "https://www.example.com/home", `<nav><a href="/blog">Blog</a></nav>`, items("/blog/one", "/blog/two"), false, "https://www.example.com/blog"},
		{"healthy ignores blog-like words", "https://www.example.com/", nav, items("/a-year-in"), false, ""},
		{"weak skips other hosts", "https://www.example.com/", `<nav><a href="https://other.example.com/blog">Blog</a></nav>`, nil, true, ""},
		{"teaser wins over weak", "https://www.example.com/", `<nav><a href="/blog">Blog</a><a href="/updates">Updates</a></nav>`, items("/updates/one", "/updates/two"), true, "https://www.example.com/updates"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := mustPage(t, page("Home", tc.body), tc.listing)
			if got := sectionCandidate(p, tc.items, tc.weak); got != tc.want {
				t.Fatalf("sectionCandidate = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetupRecordsTheSectionCheckOnlyOutsideAnchoredSetup(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(6)
	plain := setupRecipe(t, s, "/blog")
	if !plain.Recipe.Setup.SectionChecked || plain.Recipe.Setup.Section != nil {
		t.Fatalf("plain listing setup %+v", plain.Recipe.Setup)
	}
	s.set("/robots.txt", "User-agent: *\nDisallow: /blog\n")
	if blocked := setupRecipe(t, s, "/blog"); !blocked.Blocked || !blocked.Recipe.Setup.SectionChecked {
		t.Fatalf("blocked setup %v record %+v", blocked.Blocked, blocked.Recipe.Setup)
	}
	s.set("/robots.txt", "")
	anchored, err := SetupAnchored(context.Background(), s.url("/blog"), s.url("/blog/"+slugify(postTitles[0])), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if anchored.Recipe.Setup.SectionChecked {
		t.Fatal("anchored setup recorded a section check")
	}
}

// judgedHome is a homepage whose heuristic is unsure between a guides list and three newest posts, with a nav link to /blog.
func judgedHome() string {
	var b strings.Builder
	b.WriteString(`<header><nav><a href="/about">About</a><a href="/blog">Blog</a></nav></header><main><div class="col-a">`)
	for i := 6; i < 11; i++ {
		fmt.Fprintf(&b, `<div class="item"><a href="/guides/%s">%s</a></div>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</div><div class="col-b">`)
	for i := range 3 {
		fmt.Fprintf(&b, `<div class="item"><a href="/blog/%s">%s</a></div>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</div></main>`)
	return page("Example Publication", b.String())
}

func TestSetupCountsTheJudgeOfAHomepageItMovedAwayFrom(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	s.set("/", judgedHome())
	home := homeOnly(t, s, "/")
	if !home.Recipe.Setup.JudgeWanted {
		t.Fatalf("fixture: homepage is not unsure: %+v", home.Recipe.Setup)
	}
	blog := slices.IndexFunc(home.Candidates, func(c Candidate) bool { return strings.Contains(c.Template, "/blog/") })
	if blog < 0 {
		t.Fatalf("fixture: no blog candidate among %+v", home.Candidates)
	}
	m := &fakeModel{replies: map[Effort][]string{EffortHigh: {judgeReply(OrderPage, blog)}}}
	opts := s.opts()
	opts.Model = m

	res, err := Setup(context.Background(), s.url("/"), opts)
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Recipe.Setup
	if res.Recipe.ListingURL != s.url("/blog") || rec.Section == nil || rec.Section.Judge == nil || rec.Section.Judge.Calls != 3 {
		t.Fatalf("listing %s setup %+v section %+v", res.Recipe.ListingURL, rec, rec.Section)
	}
	if n := len(m.calls(EffortHigh)); n != 3 {
		t.Fatalf("judge calls = %d, want the homepage's three and none for the section", n)
	}
	calls, usd := rec.ModelCost()
	if want := 3 * stepUsage[EffortHigh].USD; calls != 3 || math.Abs(usd-want) > 1e-9 {
		t.Fatalf("model cost = %d calls, $%v; want 3 calls, $%v", calls, usd, want)
	}
	if u := rec.ModelUsage(); u.JudgeCalls != 3 || u.InputTokens != 3*stepUsage[EffortHigh].InputTokens || u.OutputTokens != 3*stepUsage[EffortHigh].OutputTokens {
		t.Fatalf("model usage = %+v", u)
	}
}

func TestPollReadsTheSectionAMovedRecipePointsAt(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	s.set("/", teaserHome(3))
	res := setupRecipe(t, s, "/")
	if res.Recipe.ListingURL != s.url("/blog") {
		t.Fatalf("fixture: recipe stayed on %s", res.Recipe.ListingURL)
	}
	s.hits = map[string]int{}
	polled, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if s.hits["/blog"] != 1 || s.hits["/"] != 0 || polled.Verdict != PollOK || len(polled.Items) != 5 {
		t.Fatalf("hits %v verdict %s items %d", s.hits, polled.Verdict, len(polled.Items))
	}
}

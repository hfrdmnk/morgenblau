package webfeed

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var reClassAttr = regexp.MustCompile(`class="([^"]*)"`)

// rehash renames every class the way a new CSS-module or Tailwind build would.
func rehash(html, salt string) string {
	return reClassAttr.ReplaceAllStringFunc(html, func(m string) string {
		var out []string
		for _, c := range strings.Fields(reClassAttr.FindStringSubmatch(m)[1]) {
			h := fnv.New32a()
			h.Write([]byte(salt + c))
			out = append(out, "_"+strconv.FormatUint(uint64(h.Sum32()), 36))
		}
		return `class="` + strings.Join(out, " ") + `"`
	})
}

func setupRecipe(t *testing.T, s *testSite, path string) *Result {
	t.Helper()
	res, err := Setup(context.Background(), s.url(path), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func itemURLs(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.URL
	}
	return out
}

func popularAside() string {
	var b strings.Builder
	b.WriteString(`<aside class="sidebar"><h3>Popular</h3><ol class="popular">`)
	for i := 7; i < 12; i++ {
		fmt.Fprintf(&b, `<li class="pop"><a class="pop-link" href="/blog/%s">%s</a></li>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</ol></aside>`)
	return b.String()
}

func TestApplyMatchesSetupOnTheSamePage(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(6)
	res := setupRecipe(t, s, "/blog")
	body, _ := s.get(t, "/blog")
	items, err := apply(res.Recipe, body, s.url("/blog"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(items, res.Items) {
		t.Fatalf("Apply = %+v\nSetup = %+v", items, res.Items)
	}
}

func TestApplySurvivesDrift(t *testing.T) {
	s := newTestSite(t)
	listing := siteNav + `<div class="layout">` + blogList(8) + popularAside() + `</div>` + siteFooter
	s.set("/blog", page("Blog", listing))
	res := setupRecipe(t, s, "/blog")
	want := itemURLs(res.Items)
	if len(want) != 8 {
		t.Fatalf("setup items = %v", want)
	}
	cases := map[string]string{
		"unchanged":        page("Blog", listing),
		"classes rehashed": rehash(page("Blog", listing), "2026"),
		"wrapper added":    page("Blog", strings.Replace(listing, `<main>`, `<main><div class="container"><div class="inner">`, 1)),
		"new post on top":  page("Blog", strings.Replace(listing, `<ul class="posts">`, `<ul class="posts"><li class="post"><h2><a href="/blog/a-brand-new-post-today">A brand new post today</a></h2><time datetime="2024-03-25">March 25, 2024</time></li>`, 1)),
	}
	for name, html := range cases {
		t.Run(name, func(t *testing.T) {
			items, err := apply(res.Recipe, []byte(html), s.url("/blog"))
			if err != nil {
				t.Fatal(err)
			}
			got := itemURLs(items)
			if name == "new post on top" {
				if len(got) != 9 || !strings.HasSuffix(got[0], "/blog/a-brand-new-post-today") {
					t.Fatalf("got %v", got)
				}
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v\nwant %v", got, want)
			}
		})
	}
}

func TestApplyKeepsDecoyOutWhileTheHintMatches(t *testing.T) {
	s := newTestSite(t)
	listing := siteNav + `<div class="layout">` + blogList(5) + popularAside() + `</div>`
	s.set("/blog", page("Blog", listing))
	res := setupRecipe(t, s, "/blog")
	items, err := apply(res.Recipe, []byte(page("Blog", listing)), s.url("/blog"))
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if strings.Contains(it.URL, slugify(postTitles[11])) {
			t.Fatalf("decoy popular post %s leaked into %v", it.URL, itemURLs(items))
		}
	}
}

func TestApplyDedupesExcludesAndOrders(t *testing.T) {
	html := page("Blog", `<main><ul class="posts">
<li><h2><a href="/blog/older-post-from-january">Older post from January</a></h2><time datetime="2024-01-01"></time></li>
<li><h2><a href="/blog/newest-post-from-march">Newest post from March</a></h2><time datetime="2024-03-01"></time></li>
<li><h2><a href="/blog/the-archive-hub-page">The archive hub page</a></h2></li>
<li><h2><a href="/blog/older-post-from-january/">Older post from January</a></h2><time datetime="2024-01-01"></time></li>
</ul></main>`)
	r := newRecipe("https://www.example.com/blog", ModeStreams)
	r.Streams = []Stream{{URLTemplate: "example.com/blog/*", ContainerHints: []string{"ul.posts<main<body"}}}
	r.ExcludeURLs = []string{"https://www.example.com/blog/the-archive-hub-page"}
	r.Order = OrderDate
	items, err := apply(r, []byte(html), "https://www.example.com/blog")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://www.example.com/blog/newest-post-from-march", "https://www.example.com/blog/older-post-from-january"}
	if !reflect.DeepEqual(itemURLs(items), want) {
		t.Fatalf("got %v", itemURLs(items))
	}
}

func TestApplySitemapMode(t *testing.T) {
	r := newRecipe("https://news.example.com/news", ModeSitemap)
	r.Streams = []Stream{{URLTemplate: "news.example.com/news/*"}}
	items, err := apply(r, []byte(sitemapXML("https://news.example.com/news", "https://news.example.com/news/hello-there-world", "https://news.example.com/about/team")), "https://news.example.com/sitemap.xml")
	if err != nil || len(items) != 1 || items[0].Title != "Hello there world" {
		t.Fatalf("items %+v err %v", items, err)
	}
}

func TestApplyRejectsModesItCannotRun(t *testing.T) {
	if _, err := apply(newRecipe("https://news.example.com/", Mode("feed")), nil, "https://news.example.com/"); !errors.Is(err, errUnsupportedMode) {
		t.Fatalf("err = %v", err)
	}
}

func TestCategoryInPathRecipeFollowsNewCategories(t *testing.T) {
	s := newTestSite(t)
	s.set("/fr/blog", page("Blog", categoryPathList([]string{"money/tax", "start/ideas", "money/cash"}, 6)))
	res := setupRecipe(t, s, "/fr/blog")
	if len(res.Items) != 6 || len(res.Recipe.Streams) != 1 || res.Recipe.Streams[0].URLTemplate != s.host()+"/fr/blog/*/*/*" {
		t.Fatalf("items %d streams %+v", len(res.Items), res.Recipe.Streams)
	}
	later := page("Blog", categoryPathList([]string{"brand/new", "money/tax"}, 4))
	items, err := apply(res.Recipe, []byte(later), s.url("/fr/blog"))
	if err != nil || len(items) != 4 {
		t.Fatalf("Apply on a new category = %v, %v", itemURLs(items), err)
	}
}

func TestBlocksCoveredByTheTopPatternJoinOneStream(t *testing.T) {
	s := newTestSite(t)
	mixed := categoryPathList([]string{"money/tax", "start/ideas", "money/cash", "team/hiring"}, 8)
	var b strings.Builder
	b.WriteString(`<section class="spotlight"><h2>Tax corner</h2><div class="row">`)
	for i := 8; i < 11; i++ {
		fmt.Fprintf(&b, `<div class="tile"><h4><a href="/fr/blog/money/tax/%s">%s</a></h4><time datetime="2026-08-%02d"></time></div>`, slugify(postTitles[i]), postTitles[i], i)
	}
	b.WriteString(`</div></section>`)
	s.set("/fr/blog", page("Blog", strings.Replace(mixed, "</main>", b.String()+"</main>", 1)))
	res := setupRecipe(t, s, "/fr/blog")
	if len(res.Items) != 11 || len(res.Recipe.Streams) != 1 || res.Recipe.Streams[0].URLTemplate != s.host()+"/fr/blog/*/*/*" || len(res.Recipe.Streams[0].ContainerHints) != 2 {
		t.Fatalf("items %d streams %+v", len(res.Items), res.Recipe.Streams)
	}
}

package webfeed

import (
	"fmt"
	"strings"
	"testing"
)

func TestBetterTitle(t *testing.T) {
	cases := []struct {
		name    string
		e       entry
		article string
		want    bool
	}{
		{"slug loses", entry{Title: "My first post", TitleSource: "slug", text: ""}, "My First Post, Revisited", true},
		{"label loses", entry{Title: "Product", TitleSource: "anchor", text: "Product A new reader"}, "A new reader for everyone", true},
		{"subtitle loses to the headline shown in the card", entry{Title: "Ein Untertitel, der vor der Überschrift steht", TitleSource: "anchor", text: "Ein Untertitel, der vor der Überschrift steht Die eigentliche Überschrift"}, "Die eigentliche Überschrift", true},
		{"a good title never loses to a bare number", entry{Title: "Field note #1", TitleSource: "attr", text: ""}, "1", false},
		{"listing title with a date and label prefix loses", entry{Title: "Sep 10, 2026 | Benefits Open enrollment, without the fire drill", TitleSource: "class"}, "Open enrollment, without the fire drill", true},
		{"same title stays", entry{Title: "A quieter way to follow the news", TitleSource: "heading"}, "A quieter way to follow the news", false},
		{"rewritten headline not in the card stays", entry{Title: "A quieter way to follow the news", TitleSource: "heading", text: "A quieter way to follow the news"}, "Why we built a calm reader", false},
	}
	for _, c := range cases {
		if got := betterTitle(&c.e, c.article); got != c.want {
			t.Errorf("%s: betterTitle = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHubPagesAreListsNotArticlesWithRelatedPosts(t *testing.T) {
	related := `<aside><h3>More stories</h3><ul class="related">`
	for i := range 6 {
		related += `<li><a href="/blog/` + slugify(postTitles[i]) + `">` + postTitles[i] + `</a></li>`
	}
	related += `</ul></aside><nav class="cats"><a href="/blog/inside">Inside</a><a href="/blog/business">Business</a><a href="/blog/drivers">Drivers</a><a href="/blog/fleets">Fleets</a></nav>`
	prose := strings.Repeat("<p>This paragraph is part of a long article body that a reader came here to read, sentence after sentence.</p>", 25)
	articleHTML := `<html><head><meta property="og:type" content="website"></head><body><main><h1>A post without article markup</h1>` + prose + related + `</main></body></html>`
	hubHTML := page("Archive", blogList(12))

	for name, c := range map[string]struct {
		html string
		want bool
	}{"article with related posts": {articleHTML, false}, "archive page": {hubHTML, true}} {
		p := mustPage(t, c.html, "https://example.com/blog/some-page")
		info := &articleInfo{url: "https://example.com/blog/some-page", ok: true, page: p}
		info.summarizeLists()
		info.page = nil
		if got := info.isHub("example.com/blog/*"); got != c.want {
			t.Errorf("%s: isHub = %v, want %v", name, got, c.want)
		}
	}
}

func TestTitleShapeOfLongJapaneseHeadlines(t *testing.T) {
	jp := "次世代ハイブリッドシステムの名称を「例示ハイブリッド」に決定 ～搭載モデルを来年からグローバルで順次展開～"
	if !goodTitleShape(jp) {
		t.Fatalf("a normal Japanese headline (%d words by our count) fails title_shape", wordCount(jp))
	}
	if goodTitleShape("お知らせ") {
		t.Fatal("a bare Japanese label passes title_shape")
	}
}

func TestArticleTitleSharedByManyPagesIsTheSiteName(t *testing.T) {
	infos := map[string]*articleInfo{
		"a": {ogTitle: "Example Lab", ok: true},
		"b": {ogTitle: "Example Lab", ok: true},
		"c": {ogTitle: "A real headline | Example Lab", ok: true},
	}
	cleanArticleTitles(infos)
	if infos["a"].clean != "" || infos["b"].clean != "" || infos["c"].clean != "A real headline" {
		t.Fatalf("clean = %q %q %q", infos["a"].clean, infos["b"].clean, infos["c"].clean)
	}
}

func TestVeryLongJapanesePressTitlesKeepTheirShape(t *testing.T) {
	jp := strings.Repeat("研究所の新しい取り組みについてのお知らせと詳細な説明", 4)
	if !goodTitleShape(jp) {
		t.Fatalf("a %d-character Japanese press title fails title_shape (%d words)", len([]rune(jp)), wordCount(jp))
	}
}

func TestAllCapsRuleIgnoresAcronymsInUncasedScripts(t *testing.T) {
	if !goodTitleShape("TVアニメ10周年記念「ABEMAチャンネル」初開設決定") {
		t.Fatal("Japanese title with Latin acronyms read as all caps")
	}
	if goodTitleShape("BREAKING NEWS FROM THE EXAMPLE LAB") {
		t.Fatal("an all-caps Latin title passed")
	}
}

func TestCalibrateLimitFetchesMoreWhenMostPostsAreUndated(t *testing.T) {
	for _, c := range []struct{ n, dated, want int }{
		{10, 10, 20},
		{10, 5, 20},
		{10, 4, 30},
		{10, 0, 30},
		{0, 0, 20},
	} {
		if got := calibrateLimit(c.n, c.dated); got != c.want {
			t.Errorf("calibrateLimit(%d, %d) = %d, want %d", c.n, c.dated, got, c.want)
		}
	}
}

func TestDateOrderNeedsSeventyPercentDated(t *testing.T) {
	if got := orderFor(0.7); got != OrderDate {
		t.Errorf("orderFor(0.7) = %s, want %s", got, OrderDate)
	}
	if got := orderFor(0.69); got != OrderPage {
		t.Errorf("orderFor(0.69) = %s, want %s", got, OrderPage)
	}
}

// numberedBlog serves /blog with n distinct posts and their pages; dated posts carry a listing date.
func numberedBlog(s *testSite, n int, dated bool) {
	var b strings.Builder
	b.WriteString(`<main><h1>Blog</h1><ul class="posts">`)
	for i := range n {
		title := fmt.Sprintf("Example post number %d about calm reading", i)
		path := "/blog/" + slugify(title)
		date := ""
		if dated {
			date = fmt.Sprintf(`<time datetime="2024-%02d-%02d"></time>`, 12-i/28, 28-i%28)
		}
		fmt.Fprintf(&b, `<li class="post"><h2><a href="%s">%s</a></h2>%s</li>`, path, title, date)
		s.set(path, article(title, title, ""))
	}
	b.WriteString(`</ul></main>`)
	s.set("/blog", page("Blog | Example Publication", b.String()))
}

func (s *testSite) hitsUnder(prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for path, c := range s.hits {
		if strings.HasPrefix(path, prefix) {
			n += c
		}
	}
	return n
}

func TestSetupCalibrationFetchesAtMostItsCap(t *testing.T) {
	for _, c := range []struct {
		name  string
		posts int
		dated bool
		want  int
	}{
		{"dated list", 25, true, 20},
		{"undated list", 35, false, 30},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newTestSite(t)
			numberedBlog(s, c.posts, c.dated)
			res := setupRecipe(t, s, "/blog")
			if len(res.Items) != c.posts {
				t.Fatalf("items = %d, want all %d posts", len(res.Items), c.posts)
			}
			if got := s.hitsUnder("/blog/"); got != c.want {
				t.Fatalf("article pages fetched = %d, want %d", got, c.want)
			}
		})
	}
}

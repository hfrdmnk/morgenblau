package webfeed

import (
	"fmt"
	"strings"
	"testing"
)

func TestCandidatesFindPlainPostList(t *testing.T) {
	p := mustPage(t, page("Blog | Example Publication", siteNav+blogList(10)+siteFooter), "https://www.example.com/blog")
	cands := p.candidates()
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	top := cands[0]
	if top.template != "example.com/blog/*" || len(top.entries) != 10 {
		t.Fatalf("top = %s with %d entries %v", top.template, len(top.entries), entryURLs(top))
	}
	e := top.entries[0]
	if e.URL != "https://www.example.com/blog/how-we-rebuilt-the-reading-queue-from-scratch" || e.Title != postTitles[0] || e.Date.Format("2006-01-02") != "2024-03-20" {
		t.Fatalf("entry 0 = %+v", e)
	}
	if top.dateFrac != 1 || top.titleShape != 1 || top.navShare != 0 {
		t.Fatalf("features = date %v shape %v nav %v", top.dateFrac, top.titleShape, top.navShare)
	}
	conf, margin := confidence(cands)
	if conf < 0.6 || margin < 2 {
		t.Fatalf("confidence %.2f margin %.2f, want a confident pick", conf, margin)
	}
	for _, c := range cands[1:] {
		if c.template == "example.com/blog/*" {
			t.Fatalf("post list split into a second candidate: %v", entryURLs(c))
		}
	}
}

func TestCandidatesSkipSamePageAndFragmentLinks(t *testing.T) {
	body := `<main><ul class="toc"><li><a href="#one">One section</a></li><li><a href="#two">Two section</a></li><li><a href="/blog#three">Three section</a></li><li><a href="/blog">This page</a></li></ul></main>` + blogList(4)
	p := mustPage(t, page("Blog", body), "https://example.com/blog")
	for _, c := range p.candidates() {
		for _, u := range entryURLs(c) {
			if strings.Contains(u, "#") || u == "https://example.com/blog" {
				t.Fatalf("same-page link %s became an entry", u)
			}
		}
	}
}

func TestCardTitleIsTheHeadingNotTheCategoryOrExcerpt(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="grid">`)
	for i := range 6 {
		cat := []string{"Product", "Research", "Company"}[i%3]
		fmt.Fprintf(&b, `<a class="card" href="/news/%s"><span class="eyebrow">%s</span><h3>%s</h3><p>%s</p><span>Read more</span></a>`, slugify(postTitles[i]), cat, postTitles[i], "This excerpt is a long sentence that follows the headline and should never become the title of the entry in the feed.")
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("News", siteNav+b.String()), "https://example.com/news")
	top := p.candidates()[0]
	for i, e := range top.entries {
		if e.Title != postTitles[i] {
			t.Fatalf("entry %d title = %q, want %q", i, e.Title, postTitles[i])
		}
	}
}

func TestCardTitleWithoutHeadingSkipsLabelsDatesAndExcerpt(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="list">`)
	for i := range 6 {
		cat := []string{"Announcements", "Engineering"}[i%2]
		fmt.Fprintf(&b, `<div class="row"><a href="/news/%s"><div>%s</div><div>Mar %d, 2024</div><div>%s</div><div>%s</div></a></div>`, slugify(postTitles[i]), cat, i+1, postTitles[i], "A much longer excerpt sentence that describes what the post is about and keeps going for a while longer.")
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("News", b.String()), "https://example.com/news")
	top := p.candidates()[0]
	for i, e := range top.entries {
		if e.Title != postTitles[i] {
			t.Fatalf("entry %d title = %q, want %q", i, e.Title, postTitles[i])
		}
		if e.Date.IsZero() {
			t.Fatalf("entry %d has no date", i)
		}
	}
}

func TestCardsWithoutTextFallBackToAltThenSlug(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="notes">`)
	for i := range 5 {
		alt := ""
		if i == 0 {
			alt = "Example cover for the first note"
		}
		fmt.Fprintf(&b, `<a class="note" href="/notes/%s"><img src="/n%d.png" alt="%s"></a>`, slugify(postTitles[i]), i, alt)
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("Notes", b.String()), "https://example.com/notes")
	top := p.candidates()[0]
	if len(top.entries) != 5 {
		t.Fatalf("entries = %v", entryURLs(top))
	}
	if top.entries[0].Title != "Example cover for the first note" || top.entries[0].TitleSource != "attr" {
		t.Errorf("entry 0 = %+v", top.entries[0])
	}
	if top.entries[1].Title != "A quieter way to follow the news" || top.entries[1].TitleSource != "slug" {
		t.Errorf("entry 1 = %+v", top.entries[1])
	}
}

func TestFeaturedHeroAndListAreSeparateSameShapeCandidates(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><section class="featured">`)
	for i := range 2 {
		fmt.Fprintf(&b, `<article class="hero"><a href="/blog/%s"><img src="/h.png" alt=""><h2>%s</h2></a></article>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</section><section class="latest"><h2>Latest</h2><ul>`)
	for i := 2; i < 10; i++ {
		fmt.Fprintf(&b, `<li><a href="/blog/%s">%s</a> <span>Mar %d, 2024</span></li>`, slugify(postTitles[i]), postTitles[i], 20-i)
	}
	b.WriteString(`</ul></section></main>`)
	p := mustPage(t, page("Blog", siteNav+b.String()+siteFooter), "https://example.com/blog")
	cands := p.candidates()
	var shapes []*cluster
	for _, c := range cands {
		if c.template == "example.com/blog/*" {
			shapes = append(shapes, c)
		}
	}
	if len(shapes) != 2 || len(shapes[0].entries) != 8 || len(shapes[1].entries) != 2 {
		for _, c := range cands {
			t.Logf("%s %v", c.template, entryURLs(c))
		}
		t.Fatalf("want list (8) then hero (2) as separate candidates, got %d", len(shapes))
	}
	pick := heuristicPick(cands, "https://example.com/blog")
	got := 0
	for _, i := range pick {
		got += len(cands[i].entries)
	}
	if got != 10 {
		t.Fatalf("heuristic pick %v covers %d entries, want hero and list", pick, got)
	}
}

func TestDecoyPopularSidebarRanksBelowMainList(t *testing.T) {
	var b strings.Builder
	b.WriteString(blogList(10))
	b.WriteString(`<aside class="sidebar"><h3>Popular</h3><ol>`)
	for i := 6; i < 11; i++ {
		fmt.Fprintf(&b, `<li><a href="/blog/%s">%s</a></li>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</ol></aside>`)
	p := mustPage(t, page("Blog", siteNav+b.String()+siteFooter), "https://example.com/blog")
	cands := p.candidates()
	if len(cands[0].entries) != 10 {
		t.Fatalf("top = %v", entryURLs(cands[0]))
	}
}

func TestNavMenuTagCloudAndPaginationAreNotPosts(t *testing.T) {
	var b strings.Builder
	b.WriteString(siteNav)
	b.WriteString(`<div class="menu-mega">`)
	for _, s := range []string{"Platform", "Solutions", "Resources", "Partners", "Enterprise", "Developers", "Community", "Events", "Webinars", "Guides", "Support", "Press"} {
		fmt.Fprintf(&b, `<div class="menu-item"><a href="/%s">%s</a></div>`, strings.ToLower(s), s)
	}
	b.WriteString(`</div>`)
	b.WriteString(blogList(6))
	b.WriteString(`<div class="tags">`)
	for _, s := range []string{"design", "engineering", "product", "culture", "research", "news", "ai", "security"} {
		fmt.Fprintf(&b, `<span><a href="/blog/tag/%s">%s</a></span>`, s, s)
	}
	b.WriteString(`</div><div class="pager"><a href="/blog/page/1">1</a><a href="/blog/page/2">2</a><a href="/blog/page/3">3</a><a href="/blog/page/2">Older posts</a></div>`)
	p := mustPage(t, page("Blog", b.String()+siteFooter), "https://example.com/blog")
	cands := p.candidates()
	if cands[0].template != "example.com/blog/*" || len(cands[0].entries) != 6 {
		t.Fatalf("top = %s %v", cands[0].template, entryURLs(cands[0]))
	}
	for _, u := range entryURLs(cands[0]) {
		if strings.Contains(u, "/tag/") || strings.Contains(u, "/page/") {
			t.Fatalf("top list contains %s", u)
		}
	}
}

func TestGridRowsYieldOneEntryPerCard(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="grid">`)
	for r := range 3 {
		b.WriteString(`<div class="row">`)
		for c := range 3 {
			i := r*3 + c
			fmt.Fprintf(&b, `<div class="col"><article><a href="/posts/%s"><img src="/i.png" alt=""></a><h3><a href="/posts/%s">%s</a></h3><a class="cat" href="/topics/%s">Topic %d</a><time datetime="2023-12-%02d"></time></article></div>`, slugify(postTitles[i]), slugify(postTitles[i]), postTitles[i], []string{"design", "infra", "people"}[c], c, i+1)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("Posts", b.String()), "https://example.com/posts")
	cands := p.candidates()
	top := cands[0]
	if top.template != "example.com/posts/*" || len(top.entries) != 9 {
		t.Fatalf("top = %s %v", top.template, entryURLs(top))
	}
	for i, e := range top.entries {
		if e.Title != postTitles[i] || e.Date.IsZero() {
			t.Fatalf("entry %d = %+v", i, e)
		}
	}
	for _, c := range cands[1:] {
		if c.template == "example.com/posts/*" {
			t.Fatalf("image links became a second candidate: %v", entryURLs(c))
		}
	}
}

func TestGermanAndJapaneseListingDates(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="beitraege">`)
	for i := range 4 {
		fmt.Fprintf(&b, `<div class="teaser"><h3><a href="/de/blog/beitrag-nummer-%d">Ein ausführlicher Beitrag über Nummer %d</a></h3><span>%d. März 2024</span></div>`, i, i, 10+i)
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("Blog", b.String()), "https://example.com/de/blog")
	top := p.candidates()[0]
	if top.entries[2].Date.Format("2006-01-02") != "2024-03-12" {
		t.Fatalf("German date = %v", top.entries[2].Date)
	}

	b.Reset()
	b.WriteString(`<main><ul class="news">`)
	for i := range 4 {
		fmt.Fprintf(&b, `<li><a href="/ja/news/%d"><span class="date">2024年5月%d日</span><span class="title">研究所の新しい取り組みについてのお知らせ その%d</span></a></li>`, 100+i, i+1, i)
	}
	b.WriteString(`</ul></main>`)
	p = mustPage(t, page("ニュース", b.String()), "https://example.com/ja/news")
	top = p.candidates()[0]
	if top.entries[1].Date.Format("2006-01-02") != "2024-05-02" || !strings.HasPrefix(top.entries[1].Title, "研究所") {
		t.Fatalf("Japanese entry = %+v", top.entries[1])
	}
}

func TestContainerHintDescribesListContainer(t *testing.T) {
	body := `<main class="content"><section class="feed">` + `<div class="grid cols-3">` +
		`<article><a href="/posts/one-long-post">One long post title here</a></article>` +
		`<article><a href="/posts/two-long-post">Two long post title here</a></article>` +
		`<article><a href="/posts/three-long-post">Three long post title here</a></article></div></section></main>`
	p := mustPage(t, page("Posts", body), "https://example.com/posts")
	if got := p.candidates()[0].hint; got != "div.grid.cols-3<section.feed<main.content" {
		t.Fatalf("hint = %q", got)
	}
}

func TestStableClassTokensDropPerItemAndHashSuffixes(t *testing.T) {
	cases := map[string]string{
		"post-123":             "post",
		"Card_featured__x8Ab2": "Card_featured",
		"mt-4":                 "mt",
		"x3k9a":                "",
		"css-1x2y3z":           "css",
		"card":                 "card",
		"grid-cols-3":          "grid-cols",
	}
	for in, want := range cases {
		if got := stableClass(in); got != want {
			t.Errorf("stableClass(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCSSModuleListsSplitDespiteHashedClasses(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main>`)
	for _, sec := range []struct{ cls, path string }{{"Featured_list__a1B2c", "/news/"}, {"Latest_list__d3E4f", "/news/"}} {
		fmt.Fprintf(&b, `<section class="%s"><ul>`, sec.cls)
		for i := range 3 {
			fmt.Fprintf(&b, `<li><a href="%s%s-%s">%s</a></li>`, sec.path, sec.cls[:4], slugify(postTitles[i]), postTitles[i])
		}
		b.WriteString(`</ul></section>`)
	}
	b.WriteString(`</main>`)
	p := mustPage(t, page("News", b.String()), "https://example.com/news")
	if n := len(p.candidates()); n != 2 {
		t.Fatalf("got %d candidates, want the two sections apart", n)
	}
}

func TestSameShapeCategoryLinksDoNotJoinThePick(t *testing.T) {
	cats := `<main><div class="cats"><a href="/blog/inside">Inside</a><a href="/blog/business">Business</a><a href="/blog/drivers">Drivers</a><a href="/blog/fleets">Fleets</a></div>` + strings.TrimPrefix(blogList(6), "<main>")
	p := mustPage(t, page("Blog", cats), "https://example.com/blog")
	cands := p.candidates()
	for _, i := range heuristicPick(cands, "https://example.com/blog") {
		for _, u := range entryURLs(cands[i]) {
			if strings.HasSuffix(u, "/blog/inside") {
				t.Fatalf("category link joined the pick: %v", entryURLs(cands[i]))
			}
		}
	}
}

func TestResponsiveDuplicateListsCollapseToOneCandidate(t *testing.T) {
	list := strings.TrimSuffix(strings.TrimPrefix(blogList(6), "<main>"), "</main>")
	body := `<main><div class="hidden md:block">` + list + `</div><div class="md:hidden">` + strings.ReplaceAll(list, `class="posts"`, `class="posts-mobile"`) + `</div></main>`
	p := mustPage(t, page("Blog", body), "https://example.com/blog")
	cands := p.candidates()
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want the mobile copy dropped", len(cands))
	}
}

func TestDatedSameShapeListWithShortSlugsJoinsThePick(t *testing.T) {
	var b strings.Builder
	b.WriteString(blogList(6))
	b.WriteString(`<section class="customers"><h2>Customer stories</h2><div class="grid">`)
	for i, name := range []string{"acme", "globex", "initech"} {
		fmt.Fprintf(&b, `<a class="story" href="/blog/%s"><span>How %s ships calmer software with Example</span><span>Sep %d, 2026</span></a>`, name, name, i+1)
	}
	b.WriteString(`</div></section>`)
	p := mustPage(t, page("Blog", b.String()), "https://example.com/blog")
	cands := p.candidates()
	got := 0
	for _, i := range heuristicPick(cands, "https://example.com/blog") {
		got += len(cands[i].entries)
	}
	if got != 9 {
		t.Fatalf("pick covers %d entries, want list and customer stories", got)
	}
}

func TestListsInsideNoscriptAreSeen(t *testing.T) {
	p := mustPage(t, page("Blog", `<div id="app"></div><noscript>`+blogList(5)+`</noscript>`), "https://example.com/blog")
	cands := p.candidates()
	if len(cands) == 0 || len(cands[0].entries) != 5 {
		t.Fatalf("noscript list not found: %d candidates", len(cands))
	}
}

func categoryPathList(cats []string, n int) string {
	var b strings.Builder
	b.WriteString(`<main><div class="cards">`)
	for i := range n {
		fmt.Fprintf(&b, `<article><h3><a href="/fr/blog/%s/%s">%s</a></h3><time datetime="2026-09-%02d"></time></article>`, cats[i%len(cats)], slugify(postTitles[i]), postTitles[i], 20-i)
	}
	b.WriteString(`</div></main>`)
	return b.String()
}

func TestCategoryInPathListKeepsOneGeneralizedShape(t *testing.T) {
	cats := []string{"money/tax", "start/ideas", "money/cash", "team/hiring"}
	p := mustPage(t, page("Blog", categoryPathList(cats, 8)), "https://example.com/fr/blog")
	top := p.candidates()[0]
	if top.template != "example.com/fr/blog/*/*/*" || len(top.entries) != 8 || top.tmplShare != 1 {
		t.Fatalf("top = %s n=%d share=%.2f", top.template, len(top.entries), top.tmplShare)
	}
}

func TestTrackingNoscriptIframeDoesNotSwallowThePage(t *testing.T) {
	gtm := `<noscript><iframe src="https://tags.example.com/ns.html" height="0" width="0" style="display:none" /></noscript>`
	p := mustPage(t, page("Blog", gtm+blogList(5)), "https://example.com/blog")
	cands := p.candidates()
	if len(cands) == 0 || len(cands[0].entries) != 5 {
		t.Fatalf("list after a tracking noscript not found: %d candidates", len(cands))
	}
}

func TestTailwindVariantClassesAreNotTitleMarkers(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><ul class="latest">`)
	for i := range 4 {
		fmt.Fprintf(&b, `<li><a href="/blog/%s"><div class="flex flex-col [&_[data-title-wrapper]]:text-p1"><div class="meta"><span>Sep %d, 2026</span><div>|</div><div>Benefits</div></div><p data-title-wrapper="true"><span data-title="true">%s</span></p></div></a></li>`, slugify(postTitles[i]), i+1, postTitles[i])
	}
	b.WriteString(`</ul></main>`)
	p := mustPage(t, page("Blog", b.String()), "https://example.com/blog")
	for i, e := range p.candidates()[0].entries {
		if e.Title != postTitles[i] {
			t.Fatalf("entry %d title = %q", i, e.Title)
		}
	}
}

func TestSeeAllLinksAreNotEntries(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="sections">`)
	for s, sec := range []string{"engineering", "benchmarks"} {
		for i := range 3 {
			fmt.Fprintf(&b, `<a class="card" href="/journal/%s-%s">%s</a>`, sec, slugify(postTitles[s*3+i]), postTitles[s*3+i])
		}
		fmt.Fprintf(&b, `<a class="card" href="/journal/%s">See all</a>`, sec)
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("Journal", b.String()), "https://example.com/journal")
	for _, u := range entryURLs(p.candidates()[0]) {
		if strings.HasSuffix(u, "/journal/engineering") || strings.HasSuffix(u, "/journal/benchmarks") {
			t.Fatalf("See-all link %s became an entry", u)
		}
	}
}

func TestFeedAndImageLinksAreNotEntries(t *testing.T) {
	body := strings.Replace(blogList(4), `</ul>`, `<li><h2><a href="/blog/feed.xml">Subscribe to every new story by RSS</a></h2></li><li><h2><a href="/blog/cover-image-large.png">A big cover image of the blog</a></h2></li></ul>`, 1)
	p := mustPage(t, page("Blog", body), "https://example.com/blog")
	for _, u := range entryURLs(p.candidates()[0]) {
		if strings.HasSuffix(u, ".xml") || strings.HasSuffix(u, ".png") {
			t.Fatalf("%s became an entry", u)
		}
	}
}

func TestTwoShapeNewsroomGeneralizes(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><div class="items">`)
	for i := range 10 {
		kind := "topics"
		if i%3 == 0 {
			kind = "news"
		}
		fmt.Fprintf(&b, `<div class="item"><a href="/jp/%s/2026/item-%d-%s.html"><span class="title">%s</span></a><span>2026.09.%02d</span></div>`, kind, i, slugify(postTitles[i]), postTitles[i], 20-i)
	}
	b.WriteString(`</div></main>`)
	p := mustPage(t, page("Newsroom", b.String()), "https://example.com/jp/newsroom/")
	if top := p.candidates()[0]; top.template != "example.com/jp/*/#/*" || top.tmplShare != 1 {
		t.Fatalf("top = %s share %.2f", top.template, top.tmplShare)
	}
}

func TestSameSiteUnderCountrySecondLevels(t *testing.T) {
	if !sameSite("blog.example.co.uk", "www.example.co.uk") || sameSite("example.co.uk", "other.co.uk") || !sameSite("news.example.com", "example.com:8443") {
		t.Fatal("sameSite misjudges registrable domains")
	}
}

func TestCategoryLinksInCardsStayOutOfThePostList(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main><ul class="cards">`)
	for i := range 6 {
		cat := []string{"culture", "ai", "maker-stories"}[i%3]
		fmt.Fprintf(&b, `<li><div class="card"><a class="post" href="/blog/%s/"><h3>%s</h3></a><a class="tag" href="/blog/%s/">%s</a></div></li>`, slugify(postTitles[i]), postTitles[i], cat, strings.ToUpper(cat[:1])+cat[1:])
	}
	b.WriteString(`</ul></main>`)
	p := mustPage(t, page("Blog", b.String()), "https://example.com/blog/")
	top := p.candidates()[0]
	if len(top.entries) != 6 {
		t.Fatalf("top has %d entries: %v", len(top.entries), entryURLs(top))
	}
	for _, u := range entryURLs(top) {
		if strings.HasSuffix(u, "/culture/") || strings.HasSuffix(u, "/ai/") {
			t.Fatalf("category link %s in the post list", u)
		}
	}
}

func TestLegalAndUtilityLinksAreNeverAConfidentStream(t *testing.T) {
	body := `<main><div id="app"></div></main><div class="bottom"><div class="eyebrow">` +
		`<a href="/terms-of-use">Terms of Use</a><a href="/privacy-policy">Privacy Policy</a><a href="/code-of-conduct">Code of Conduct</a>` +
		`</div></div>`
	p := mustPage(t, page("News", body), "https://example.com/news")
	cands := p.candidates()
	if len(cands) == 0 {
		return
	}
	if conf, _ := confidence(cands); conf >= judgeBelowConfidence {
		t.Fatalf("legal links scored confidence %.2f: %v", conf, entryURLs(cands[0]))
	}
}

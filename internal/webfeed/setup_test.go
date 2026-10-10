package webfeed

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"morgenblau/internal/safehttp"
)

func TestSetupBuildsAStreamsRecipe(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(10)
	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Health != HealthOK || res.Title != "Blog | Example Publication" {
		t.Fatalf("health = %s %v, title %q", res.Health, res.HealthReasons, res.Title)
	}
	if len(res.Items) != 10 || res.Items[0].Title != postTitles[0] || res.Items[0].Date.Format("2006-01-02") != "2024-03-20" {
		t.Fatalf("items = %+v", res.Items)
	}
	r := res.Recipe
	wantTmpl := s.host() + "/blog/*"
	if r.Version != 1 || r.Mode != ModeStreams || len(r.Streams) != 1 || r.Streams[0].URLTemplate != wantTmpl || len(r.Streams[0].ContainerHints) == 0 {
		t.Fatalf("recipe = %+v", r)
	}
	if r.Order != OrderDate || r.DateSource != DateSourceListing || r.Lang != "en" || r.Input != "raw" {
		t.Fatalf("order %s, date source %s, lang %q", r.Order, r.DateSource, r.Lang)
	}
	if r.Fingerprint.Count != 10 || r.Fingerprint.DateFrac != 1 || r.Fingerprint.TitleShape != 1 || !reflect.DeepEqual(r.Fingerprint.URLTemplates, []string{wantTmpl}) {
		t.Fatalf("fingerprint = %+v", r.Fingerprint)
	}
	if !slices.Contains(r.Setup.Stages, "heuristic") || !slices.Contains(r.Setup.Stages, "verify") || r.Setup.Confidence < 0.6 || r.Setup.JudgeWanted {
		t.Fatalf("setup record = %+v", r.Setup)
	}
	if len(s.agents) != 1 || !s.agents[safehttp.UserAgent] {
		t.Fatalf("user agents = %v", s.agents)
	}
	if got := s.langs["/blog/"+slugify(postTitles[0])]; got != "en" {
		t.Fatalf("article Accept-Language = %q, want the listing's lang", got)
	}
	if s.hits["/blog"] != 1 {
		t.Fatalf("listing fetched %d times", s.hits["/blog"])
	}
}

func TestSetupFetchesArticlesPolitely(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(12)
	s.delay = 15 * 1e6
	if _, err := Setup(context.Background(), s.url("/blog"), s.opts()); err != nil {
		t.Fatal(err)
	}
	if p := s.peak.Load(); p > maxArticleConcurrency {
		t.Fatalf("peak parallel fetches = %d, want ≤ %d", p, maxArticleConcurrency)
	}
	for path, n := range s.hits {
		if n > 1 {
			t.Fatalf("%s fetched %d times", path, n)
		}
	}
}

func TestSetupHonorsRobots(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(5)
	s.set("/robots.txt", "User-agent: *\nDisallow: /blog\n")
	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Health != HealthBlockedBySite || s.hits["/blog"] != 0 {
		t.Fatalf("health %s, listing hits %d", res.Health, s.hits["/blog"])
	}
}

func sitemapXML(locs ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		fmt.Fprintf(&b, "<url><loc>%s</loc></url>", l)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func TestSetupBlockedListingFallsBackToSitemap(t *testing.T) {
	for name, block := range map[string]func(*testSite){
		"403":       func(s *testSite) { s.status["/news"] = 403 },
		"challenge": func(s *testSite) { s.set("/news", `<html><head><title>Just a moment...</title></head></html>`) },
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestSite(t)
			block(s)
			s.set("/robots.txt", "User-agent: *\nAllow: /\nSitemap: "+s.url("/sitemap-index.xml")+"\n")
			s.set("/sitemap-index.xml", `<sitemapindex><sitemap><loc>`+s.url("/sitemap-pages.xml")+`</loc></sitemap><sitemap><loc>`+s.url("/sitemap-news.xml")+`</loc></sitemap></sitemapindex>`)
			s.set("/sitemap-pages.xml", sitemapXML(s.url("/about"), s.url("/pricing")))
			s.set("/sitemap-news.xml", sitemapXML(s.url("/news"), s.url("/news/first-launch-of-the-reader"), s.url("/news/a-second-announcement-today"), s.url("/news/third-time-is-the-charm"), s.url("/careers/engineer")))
			res, err := Setup(context.Background(), s.url("/news"), s.opts())
			if err != nil {
				t.Fatal(err)
			}
			if res.Recipe.Mode != ModeSitemap || res.Health != HealthOK || len(res.Items) != 3 {
				t.Fatalf("mode %s health %s items %+v", res.Recipe.Mode, res.Health, res.Items)
			}
			if res.Items[0].Title != "First launch of the reader" || res.Recipe.Streams[0].URLTemplate != s.host()+"/news/*" {
				t.Fatalf("items %+v streams %+v", res.Items, res.Recipe.Streams)
			}
		})
	}
}

func TestSetupBlockedWithoutSitemapIsBlockedBySite(t *testing.T) {
	s := newTestSite(t)
	s.status["/news"] = 403
	res, err := Setup(context.Background(), s.url("/news"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Health != HealthBlockedBySite || len(res.Items) != 0 {
		t.Fatalf("health %s items %d", res.Health, len(res.Items))
	}
}

// ambiguousPage has two plausible lists with different URL shapes and nothing to tell them apart.
func ambiguousPage() string {
	var b strings.Builder
	b.WriteString(`<main><div class="col-a">`)
	for i := range 5 {
		fmt.Fprintf(&b, `<div class="item"><a href="/guides/%s">%s</a></div>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</div><div class="col-b">`)
	for i := 5; i < 10; i++ {
		fmt.Fprintf(&b, `<div class="item"><a href="/stories/%s">%s</a></div>`, slugify(postTitles[i]), postTitles[i])
	}
	b.WriteString(`</div></main>`)
	return page("Resources", b.String())
}

func TestSetupAsksTheJudgeWhenUnsure(t *testing.T) {
	s := newTestSite(t)
	s.set("/resources", ambiguousPage())
	j := &fakeJudge{replies: []string{"```json\n{\"pick\":[1],\"order\":\"page\",\"none\":false,\"reason\":\"stories\"}\n```"}}
	opts := s.opts()
	opts.Judge = j
	res, err := Setup(context.Background(), s.url("/resources"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if j.calls.Load() != 3 {
		t.Fatalf("judge calls = %d", j.calls.Load())
	}
	rec := res.Recipe.Setup
	if !slices.Contains(rec.Stages, "judge") || rec.Judge == nil || rec.Judge.Calls != 3 || rec.Judge.InputTokens != 7500 || rec.Judge.USD == 0 || !rec.JudgeWanted {
		t.Fatalf("setup record = %+v judge %+v", rec, rec.Judge)
	}
	want := res.Candidates[1].Template
	if res.Recipe.Streams[0].URLTemplate != want || len(res.Items) != 5 || res.Recipe.Order != OrderPage {
		t.Fatalf("streams %+v items %d", res.Recipe.Streams, len(res.Items))
	}
}

func TestSetupWithoutJudgeKeepsHeuristicPickAndRecordsTheWish(t *testing.T) {
	s := newTestSite(t)
	s.set("/resources", ambiguousPage())
	res, err := Setup(context.Background(), s.url("/resources"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Recipe.Setup
	if !rec.JudgeWanted || len(rec.JudgeReasons) == 0 || rec.Judge != nil || slices.Contains(rec.Stages, "judge") {
		t.Fatalf("setup record = %+v", rec)
	}
	if res.Recipe.Streams[0].URLTemplate != res.Candidates[0].Template || len(res.Items) != 5 {
		t.Fatalf("streams %+v items %d", res.Recipe.Streams, len(res.Items))
	}
}

func TestSetupCalibratesSubtitleWithOGTitleAndRecordsTitleHint(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><div class="teasers">`)
	for i := range 5 {
		fmt.Fprintf(&b, `<a class="teaser" href="/blog/post-%d-slug"><p class="intro">Ein Untertitel, der vor der Überschrift steht %d</p><p class="big">%s</p><span>%d. März 2024</span></a>`, i, i, postTitles[i], 10+i)
		s.set(fmt.Sprintf("/blog/post-%d-slug", i), article(postTitles[i], postTitles[i]+" | Example Publication", ""))
	}
	b.WriteString(`</div></main>`)
	s.set("/blog", `<!doctype html><html lang="de"><head><title>Blog</title></head><body>`+b.String()+`</body></html>`)
	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	for i, it := range res.Items {
		if it.Title != postTitles[4-i] {
			t.Fatalf("item %d title %q, want headline %q", i, it.Title, postTitles[4-i])
		}
	}
	if hint := res.Recipe.Streams[0].TitleHints; hint.Source != "calibrated" || hint.Tag != "p.big" {
		t.Fatalf("title hints = %+v", hint)
	}
	page, _ := s.get(t, "/blog")
	items, err := Apply(res.Recipe, page, s.url("/blog"))
	if err != nil || len(items) != 5 || items[0].Title != postTitles[4] {
		t.Fatalf("Apply with title hint = %+v, %v", items, err)
	}
}

func TestSetupFillsMissingDatesFromArticlesButNotFromASharedDate(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><ul class="notes">`)
	for i := range 6 {
		fmt.Fprintf(&b, `<li><a href="/notes/%s">%s</a></li>`, slugify(postTitles[i]), postTitles[i])
		s.set("/notes/"+slugify(postTitles[i]), article(postTitles[i], postTitles[i], fmt.Sprintf("2024-01-%02dT00:00:00Z", 10-i)))
	}
	b.WriteString(`</ul></main>`)
	s.set("/notes", page("Notes", b.String()))
	res, err := Setup(context.Background(), s.url("/notes"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.DateSource != DateSourceArticle || res.Items[0].Date.Format("2006-01-02") != "2024-01-10" || res.Recipe.Order != OrderDate {
		t.Fatalf("date source %s order %s first %+v", res.Recipe.DateSource, res.Recipe.Order, res.Items[0])
	}

	for i := range 6 {
		s.set("/notes/"+slugify(postTitles[i]), article(postTitles[i], postTitles[i], "2024-06-01T00:00:00Z"))
	}
	s2 := newTestSite(t)
	for k, v := range s.pages {
		s2.set(k, v)
	}
	res, err = Setup(context.Background(), s2.url("/notes"), s2.opts())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Items[0].Date.IsZero() || res.Recipe.Order != OrderPage || res.Recipe.DateSource != DateSourceNone {
		t.Fatalf("shared article date was trusted: %+v, order %s", res.Items[0], res.Recipe.Order)
	}
}

func TestSetupExcludesHubPages(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	var b strings.Builder
	b.WriteString(blogList(8))
	listing := page("Blog", siteNav+strings.Replace(b.String(), `</ul>`, `<li class="post"><h2><a href="/blog/archive-of-everything">The archive of everything we wrote</a></h2></li></ul>`, 1)+siteFooter)
	s.set("/blog", listing)
	s.set("/blog/archive-of-everything", page("Archive", blogList(12)))
	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Recipe.ExcludeURLs, []string{s.url("/blog/archive-of-everything")}) || len(res.Items) != 8 {
		t.Fatalf("exclude %v items %d", res.Recipe.ExcludeURLs, len(res.Items))
	}
}

func TestSetupSelfCheckAsksForAPostWhenThinOrOdd(t *testing.T) {
	s := newTestSite(t)
	s.set("/blog", page("Blog", `<main><ul class="posts"><li><h2><a href="/blog/only-one-post-here">Only one post here</a></h2></li><li><h2><a href="/blog/and-a-second-post">And a second post</a></h2></li></ul></main>`))
	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Health != HealthNeedsAttention || !slices.Contains(res.HealthReasons, "count") {
		t.Fatalf("health %s %v", res.Health, res.HealthReasons)
	}
}

func TestSetupAnchoredPicksTheListHoldingThePost(t *testing.T) {
	s := newTestSite(t)
	s.set("/resources", ambiguousPage())
	plain, err := Setup(context.Background(), s.url("/resources"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	other := "/guides/"
	if strings.Contains(plain.Recipe.Streams[0].URLTemplate, "/guides/") {
		other = "/stories/"
	}
	anchor := s.url(other + "anchor-post-not-on-the-page")
	res, err := SetupAnchored(context.Background(), s.url("/resources"), anchor, s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Recipe.Streams[0].URLTemplate, other) || len(res.Items) != 5 || res.Recipe.Setup.Anchor != anchor || !slices.Contains(res.Recipe.Setup.Stages, "anchored") {
		t.Fatalf("anchored recipe = %+v items %d", res.Recipe, len(res.Items))
	}
}

func TestSetupOrdersByPageWhenFewDates(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><ul class="posts">`)
	for i := range 10 {
		date := ""
		if i < 5 {
			date = fmt.Sprintf(`<time datetime="2024-02-%02d"></time>`, i+1)
		}
		fmt.Fprintf(&b, `<li><h2><a href="/blog/%s">%s</a></h2>%s</li>`, slugify(postTitles[i]), postTitles[i], date)
	}
	b.WriteString(`</ul></main>`)
	s.set("/blog", page("Blog", b.String()))
	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.Order != OrderPage || res.Items[0].Title != postTitles[0] {
		t.Fatalf("order %s first %q", res.Recipe.Order, res.Items[0].Title)
	}
}

func TestSetupKeepsPostLikeMinorityShapesOfThePickedList(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><section class="journal">`)
	paths := []string{}
	for i := range 8 {
		paths = append(paths, "/journal/"+slugify(postTitles[i]))
	}
	paths = append(paths, "/journal/articles/a-partnership-built-on-trust", "/journal/articles/close-your-books-faster", "/journal/rebuilding-data-pipelines/part-1", "/journal/operating-stream-processors/part-2")
	for i, p := range paths {
		fmt.Fprintf(&b, `<a class="card" href="%s"><h3>Example story number %d about calm software</h3></a>`, p, i)
	}
	b.WriteString(`</section></main>`)
	s.set("/journal", page("Journal", b.String()))
	res, err := Setup(context.Background(), s.url("/journal"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != len(paths) {
		t.Fatalf("items %d of %d, streams %+v", len(res.Items), len(paths), res.Recipe.Streams)
	}
	var tmpls []string
	for _, st := range res.Recipe.Streams {
		tmpls = append(tmpls, st.URLTemplate)
	}
	if !reflect.DeepEqual(tmpls, []string{s.host() + "/journal/*", s.host() + "/journal/*/*"}) {
		t.Fatalf("streams = %v", tmpls)
	}
}

func TestMinorityShapeOfDatedShortSlugsIsKept(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><ul class="posts">`)
	for i := range 5 {
		fmt.Fprintf(&b, `<li><a href="/%s">%s</a><time datetime="2025-0%d-01"></time></li>`, slugify(postTitles[i]), postTitles[i], i+1)
	}
	for i, slug := range []string{"debugger", "sliding-window", "arrays"} {
		fmt.Fprintf(&b, `<li><a href="https://archive.example.com/%s">An older interactive post %d</a><time datetime="2021-0%d-01"></time></li>`, slug, i, i+1)
	}
	b.WriteString(`</ul></main>`)
	s.set("/", page("Home", b.String()))
	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 8 {
		t.Fatalf("items %d, streams %+v", len(res.Items), res.Recipe.Streams)
	}
}

func TestSetupUnsureWithoutJudgeAsksForAPost(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><div class="highlights">`)
	for i := range 4 {
		fmt.Fprintf(&b, `<div><a href="https://social.example.com/status/%d">%s</a></div>`, 1000+i, postTitles[i])
	}
	b.WriteString(`</div></main>`)
	s.set("/", page("Home", b.String()))
	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe.Setup.Confidence >= judgeBelowConfidence {
		t.Fatalf("fixture is not unsure: confidence %.2f", res.Recipe.Setup.Confidence)
	}
	if res.Health != HealthNeedsAttention || !slices.Contains(res.HealthReasons, "unsure") {
		t.Fatalf("health %s %v", res.Health, res.HealthReasons)
	}
}

func TestMinorityShapesFromOtherSectionsAreGuarded(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><div class="updates">`)
	for i := range 6 {
		fmt.Fprintf(&b, `<a class="card" href="/now/%s"><h3>%s</h3></a>`, slugify(postTitles[i]), postTitles[i])
	}
	for i, c := range []string{"acme-builds-faster", "globex-ships-calmly"} {
		fmt.Fprintf(&b, `<a class="card" href="/customers/%s"><h3>Customer story number %d about calm teams</h3></a>`, c, i)
	}
	for i := range 2 {
		fmt.Fprintf(&b, `<a class="card" href="https://partner.example.com/press/release-%d-about-us"><h3>Partner press release number %d about us</h3></a>`, i, i)
	}
	b.WriteString(`</div></main>`)
	s.set("/now", page("Now", b.String()))
	res, err := Setup(context.Background(), s.url("/now"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if strings.Contains(it.URL, "/customers/") {
			t.Fatalf("customer story from another section joined: %v", itemURLs(res.Items))
		}
	}
	if len(res.Items) != 8 {
		t.Fatalf("items = %v, want the posts plus the off-site releases", itemURLs(res.Items))
	}
}

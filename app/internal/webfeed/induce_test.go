package webfeed

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var tones = []string{"ocean", "forest", "desert", "glacier", "meadow", "canyon"}

// tonedBlog defeats the extractor: every post link carries its own class and its own category path, so each is a list of one.
func tonedBlog(s *testSite) {
	var b strings.Builder
	b.WriteString(`<main><div class="feed">`)
	for i, tone := range tones {
		p := fmt.Sprintf("/blog/%s/%s", tone, slugify(postTitles[i]))
		fmt.Fprintf(&b, `<article class="post"><h2><a class="tone-%s" href="%s">%s</a></h2><time datetime="2024-03-%02d">March %d</time></article>`, tone, p, postTitles[i], 20-i, 20-i)
		s.set(p, article(postTitles[i], postTitles[i], "2024-03-01"))
	}
	b.WriteString(`</div></main>`)
	s.set("/blog", page("Blog | Example Publication", siteNav+b.String()+siteFooter))
}

var tonedRecipe = CSSRecipe{Item: "article.post", Link: "h2 a", Title: "h2", Date: "time"}

// inducer is a model whose induction replies with recipes; its judge has no script, so every vote falls back to the heuristic.
func inducer(t *testing.T, recipes ...CSSRecipe) *fakeModel {
	m := &fakeModel{replies: map[Effort][]string{}}
	for _, r := range recipes {
		m.replies[EffortMedium] = append(m.replies[EffortMedium], cssReply(t, r))
	}
	return m
}

func TestSetupInducesACSSRecipeWhenFewerThanThreeItemsSurvive(t *testing.T) {
	s := newTestSite(t)
	tonedBlog(s)
	ind := inducer(t, tonedRecipe)
	opts := s.opts()
	opts.Model = ind
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if in := ind.calls(EffortMedium); len(in) != 1 || in[0].System != inducePrompt || !strings.HasPrefix(in[0].User, "Page: "+s.url("/blog")) {
		t.Fatalf("induce prompts = %+v", ind.calls(EffortMedium))
	}
	r := res.Recipe
	if r.Mode != ModeCSS || r.CSS == nil || *r.CSS != tonedRecipe || len(r.Streams) != 0 {
		t.Fatalf("recipe mode %s css %+v streams %+v", r.Mode, r.CSS, r.Streams)
	}
	if !res.Healthy() || len(res.Items) != 6 || res.Items[0].Title != postTitles[0] || res.Items[0].Date.Format("2006-01-02") != "2024-03-20" {
		t.Fatalf("blocked %v reasons %v items %+v", res.Blocked, res.Reasons, res.Items)
	}
	wantTmpl := s.host() + "/blog/*/*"
	if r.Order != OrderDate || r.DateSource != DateSourceListing || r.Fingerprint.Count != 6 || !reflect.DeepEqual(r.Fingerprint.URLTemplates, []string{wantTmpl}) {
		t.Fatalf("order %s date source %s fingerprint %+v", r.Order, r.DateSource, r.Fingerprint)
	}
	rec := r.Setup.Induce
	if !slices.Contains(r.Setup.Stages, "induce") || rec == nil || !rec.Kept || rec.Calls != 1 || rec.Items != 6 || rec.InputTokens != 20000 || rec.OutputTokens != 900 || rec.USD != 0.00245 {
		t.Fatalf("setup record %+v induce %+v", r.Setup, rec)
	}
	listing, _ := s.get(t, "/blog")
	items, err := apply(r, listing, s.url("/blog"))
	if err != nil || !reflect.DeepEqual(itemURLs(items), itemURLs(res.Items)) {
		t.Fatalf("Apply = %v, %v; want the setup's items", itemURLs(items), err)
	}
}

func TestSetupSkipsInductionWhenTheExtractorSucceeds(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(6)
	ind := inducer(t, tonedRecipe)
	opts := s.opts()
	opts.Model = ind
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(ind.calls(EffortMedium)) != 0 || res.Recipe.Setup.Induce != nil || res.Recipe.Mode != ModeStreams {
		t.Fatalf("induced on a healthy page: calls %d record %+v", len(ind.calls(EffortMedium)), res.Recipe.Setup.Induce)
	}
}

func TestSetupRepairsAnInducedRecipeThatYieldsNothingOnce(t *testing.T) {
	s := newTestSite(t)
	tonedBlog(s)
	ind := inducer(t, CSSRecipe{Item: "li.entry", Link: "a"}, tonedRecipe)
	opts := s.opts()
	opts.Model = ind
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	in := ind.calls(EffortMedium)
	if len(in) != 2 {
		t.Fatalf("induce calls = %d, want one repair round", len(in))
	}
	repair := in[1].User
	if !strings.Contains(repair, `"item":"li.entry"`) || !strings.Contains(repair, "matched 0 elements") {
		t.Fatalf("repair message lacks the failed recipe and why:\n%s", repair)
	}
	if rec := res.Recipe.Setup.Induce; rec.Calls != 2 || !rec.Kept || rec.InputTokens != 40000 || res.Recipe.Mode != ModeCSS {
		t.Fatalf("induce record %+v mode %s", rec, res.Recipe.Mode)
	}
}

func TestSetupStopsAfterOneRepairRound(t *testing.T) {
	s := newTestSite(t)
	tonedBlog(s)
	ind := inducer(t, CSSRecipe{Item: "li.entry"})
	ind.errs = map[Effort][]error{EffortMedium: {errors.New("induce: no JSON object in reply")}}
	opts := s.opts()
	opts.Model = ind
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Recipe.Setup.Induce
	if len(ind.calls(EffortMedium)) != 2 || rec.Calls != 2 || rec.Failed != 1 || rec.Kept || rec.USD == 0 {
		t.Fatalf("calls %d record %+v", len(ind.calls(EffortMedium)), rec)
	}
	if res.Recipe.Mode != ModeStreams || len(res.Items) != 1 || (res.Healthy() || res.Blocked) {
		t.Fatalf("extractor result not kept: mode %s items %d reasons %v", res.Recipe.Mode, len(res.Items), res.Reasons)
	}
}

func TestSetupKeepsTheExtractorWhenTheInducedRecipeIsNotBetter(t *testing.T) {
	s := newTestSite(t)
	tonedBlog(s)
	// The site navigation: many links, but one-word labels fail the title-shape check.
	ind := inducer(t, CSSRecipe{Item: "header nav a"})
	opts := s.opts()
	opts.Model = ind
	res, err := Setup(context.Background(), s.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Recipe.Setup.Induce
	if rec == nil || rec.Kept || rec.Calls != 1 || rec.Items == 0 {
		t.Fatalf("induce record %+v", rec)
	}
	if res.Recipe.Mode != ModeStreams || res.Recipe.CSS != nil || len(res.Items) != 1 {
		t.Fatalf("mode %s css %+v items %d", res.Recipe.Mode, res.Recipe.CSS, len(res.Items))
	}
}

func TestSetupInductionHonorsCancellation(t *testing.T) {
	s := newTestSite(t)
	tonedBlog(s)
	ctx, cancel := context.WithCancel(context.Background())
	opts := s.opts()
	opts.Model = &fakeModel{before: func(_ context.Context, p Prompt) {
		if p.Effort == EffortMedium {
			cancel()
		}
	}}
	if _, err := Setup(ctx, s.url("/blog"), opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestSetupRecordModelCostSumsJudgeAndInduction(t *testing.T) {
	rec := SetupRecord{
		Judge:  &JudgeRecord{Calls: 3, USD: 0.0016},
		Induce: &InduceRecord{Calls: 2, USD: 0.005},
	}
	calls, usd := rec.ModelCost()
	if calls != 5 || usd != 0.0066 {
		t.Fatalf("ModelCost = %d, %v", calls, usd)
	}
	if calls, usd := (SetupRecord{}).ModelCost(); calls != 0 || usd != 0 {
		t.Fatalf("empty record cost = %d, %v", calls, usd)
	}
}

func TestParseCSSRecipe(t *testing.T) {
	got, err := parseCSSRecipe("Here you go:\n```json\n{\"item\":\" li.post \",\"link\":\"a\",\"title\":\"\",\"date\":\"time\",\"reason\":\"list\"}\n```")
	if err != nil || got != (CSSRecipe{Item: "li.post", Link: "a", Date: "time"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, bad := range []string{"no json", `{"item":""}`, `{"item":"li[","link":"a"}`, `{"item":"li","date":"::nope("}`} {
		if _, err := parseCSSRecipe(bad); err == nil {
			t.Errorf("parseCSSRecipe(%q) accepted", bad)
		}
	}
}

func TestApplyCSSExtractsLinksTitlesAndDates(t *testing.T) {
	html := page("News", `<base href="https://news.example.com/sub/"><main>
<div class="card"><a href="first-story"><span class="kicker">Launch</span><h3>The first story of the season</h3></a><span class="when">March 3, 2024</span></div>
<div class="card"><a href="/sub/second-story?utm_source=x">Read more</a><h3>A second story worth reading</h3><time datetime="2024-03-02">yesterday</time></div>
<div class="card"><a href="first-story">Duplicate link to the first story</a></div>
<div class="card"><a href="#top">Back to top</a></div>
<div class="card"><h3>A card without any link</h3></div>
</main>`)
	p := mustPage(t, html, "https://news.example.com/sub/")
	items, matched, err := applyCSS(p, CSSRecipe{Item: "div.card", Title: "h3", Date: "time, .when"})
	if err != nil || matched != 5 {
		t.Fatalf("matched %d, err %v", matched, err)
	}
	want := []Item{
		{URL: "https://news.example.com/sub/first-story", Title: "The first story of the season"},
		{URL: "https://news.example.com/sub/second-story", Title: "A second story worth reading"},
	}
	if len(items) != 2 || items[0].URL != want[0].URL || items[0].Title != want[0].Title || items[1].URL != want[1].URL || items[1].Title != want[1].Title {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Date.Format("2006-01-02") != "2024-03-03" || items[1].Date.Format("2006-01-02") != "2024-03-02" {
		t.Fatalf("dates = %v, %v", items[0].Date, items[1].Date)
	}

	itemIsLink, _, err := applyCSS(p, CSSRecipe{Item: "div.card > a"})
	if err != nil || len(itemIsLink) != 2 || itemIsLink[0].Title != "Launch The first story of the season" {
		t.Fatalf("item-is-link = %+v, %v", itemIsLink, err)
	}
	if _, _, err := applyCSS(p, CSSRecipe{Item: "div.card", Link: "a[href"}); err == nil {
		t.Fatal("invalid selector accepted")
	}
}

func TestApplyRunsCSSRecipes(t *testing.T) {
	s := newTestSite(t)
	tonedBlog(s)
	listing, _ := s.get(t, "/blog")
	r := newRecipe(s.url("/blog"), ModeCSS)
	r.CSS = &tonedRecipe
	r.Order = OrderDate
	r.ExcludeURLs = []string{s.url("/blog/ocean/" + slugify(postTitles[0]))}
	items, err := apply(r, listing, s.url("/blog"))
	if err != nil || len(items) != 5 || items[0].Title != postTitles[1] {
		t.Fatalf("Apply = %+v, %v", items, err)
	}
	r.CSS = nil
	if _, err := apply(r, listing, s.url("/blog")); err == nil {
		t.Fatal("css mode without selectors accepted")
	}
}

func TestSkeletonKeepsStructureAndCutsRepeats(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<script>var tracking = "secret";</script><style>.x{color:red}</style><svg><path d="M0"/></svg>
<div class="wrap"><div class="inner"><ul class="posts list-none">`)
	for i := range 30 {
		fmt.Fprintf(&b, `<li class="post"><a href="/blog/post-%d"><h2>Post number %d of the archive</h2></a><time datetime="2024-01-%02d">Jan %d</time><div class="spacer"></div></li>`, i, i, i%28+1, i%28+1)
	}
	b.WriteString(`</ul></div></div>`)
	p := mustPage(t, page("Archive", b.String()), "https://news.example.com/blog")
	sk := skeleton(p, maxSkeletonChars)
	for _, want := range []string{
		`div.wrap > div.inner > ul.posts.list-none`,
		`li.post`, `a [href=/blog/post-0]`, `h2 "Post number 0 of the archive"`, `time [datetime=2024-01-01] "Jan 1"`,
		`… 27 more li.post`,
	} {
		if !strings.Contains(sk, want) {
			t.Errorf("skeleton lacks %q:\n%s", want, sk)
		}
	}
	for _, banned := range []string{"tracking", "color:red", "path", "Post number 3 ", "spacer"} {
		if strings.Contains(sk, banned) {
			t.Errorf("skeleton keeps %q:\n%s", banned, sk)
		}
	}
}

func TestSkeletonStaysWithinItsBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<main>`)
	for i := range 4000 {
		fmt.Fprintf(&b, `<section class="block-%d"><p class="lede-%d">Paragraph %d with enough words to take some room in the skeleton</p><a href="/p/%d">Link %d</a></section>`, i, i, i, i, i)
	}
	b.WriteString(`</main>`)
	p := mustPage(t, page("Huge", b.String()), "https://news.example.com/")
	sk := skeleton(p, maxSkeletonChars)
	if len(sk) > maxSkeletonChars || !strings.Contains(sk, "section.block-0") {
		t.Fatalf("skeleton is %d chars (limit %d), starts %q", len(sk), maxSkeletonChars, sk[:min(200, len(sk))])
	}
}

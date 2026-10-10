package websource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"morgenblau/internal/webfeed"
)

type fixedJudge struct{}

func (fixedJudge) Judge(context.Context, webfeed.JudgeInput) (webfeed.Verdict, webfeed.Usage, error) {
	return webfeed.Verdict{Pick: []int{0}, Order: webfeed.OrderDate}, webfeed.Usage{InputTokens: 2500, OutputTokens: 600, USD: 0.00054}, nil
}

func TestWebfeedGeneratorSetupMapsTheResult(t *testing.T) {
	web := driftSite()
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client()}, nil)
	got, err := gen.Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != HealthOK || got.HealthReason != "" || got.NeedsAnchor || got.Title != "Blog | Example Publication" {
		t.Fatalf("generated = %+v", got)
	}
	if len(got.Items) != 8 || got.Items[0].URL != driftURL(2, "blog") || got.Items[0].Excerpt != "A short teaser for post 2 that tells readers what it is about." {
		t.Fatalf("first item = %+v of %d", got.Items[0], len(got.Items))
	}
	if got.Fingerprint.URLTemplate != "blog.example.com/blog/*" || got.Fingerprint.TitleShape != 1 || got.Fingerprint.DateFrac != 1 {
		t.Errorf("fingerprint = %+v", got.Fingerprint)
	}
	if !slices.Contains(got.Cost.Stages, "heuristic") || got.Cost.USD != 0 {
		t.Errorf("cost = %+v", got.Cost)
	}
	if !strings.Contains(string(got.Recipe), `"url_template":"blog.example.com/blog/*"`) {
		t.Errorf("recipe = %s", got.Recipe)
	}
}

func TestWebfeedGeneratorSetupWordsItsDoubtsForReaders(t *testing.T) {
	thin := driftSite()
	thin.set(driftListing, driftPage("Blog", driftList(2, 4, "blog")))
	got, err := NewWebfeedGenerator(webfeed.Options{Client: thin.client()}, nil).Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != HealthNeedsAttention || !got.NeedsAnchor || got.HealthReason != "only a few posts were found" {
		t.Errorf("thin listing = %s %v %q", got.Health, got.NeedsAnchor, got.HealthReason)
	}

	closed := driftSite()
	closed.setPage("https://blog.example.com/robots.txt", fakePage{ctype: "text/plain", body: "User-agent: *\nDisallow: /blog\n"})
	got, err = NewWebfeedGenerator(webfeed.Options{Client: closed.client()}, nil).Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != HealthBlockedBySite || got.NeedsAnchor || got.HealthReason != "this site blocks feed readers" {
		t.Errorf("robots-closed listing = %s %v %q", got.Health, got.NeedsAnchor, got.HealthReason)
	}

	for _, reasons := range [][]string{{"unsure"}, {"something new"}} {
		if got := setupReason(HealthNeedsAttention, reasons); got != "Morgenblau isn't sure which links are posts" {
			t.Errorf("setupReason(%v) = %q", reasons, got)
		}
	}
}

// twoListsPage has two undated lists of different URL shapes and nothing to tell them apart, so the heuristic wants the judge.
func twoListsPage() string {
	var b strings.Builder
	b.WriteString(`<main><div class="col-a">`)
	for i := range 5 {
		fmt.Fprintf(&b, `<div class="item"><a href="/guides/%s">%s</a></div>`, driftSlug(driftTitles[i]), driftTitles[i])
	}
	b.WriteString(`</div><div class="col-b">`)
	for i := 5; i < 10; i++ {
		fmt.Fprintf(&b, `<div class="item"><a href="/stories/%s">%s</a></div>`, driftSlug(driftTitles[i]), driftTitles[i])
	}
	b.WriteString(`</div></main>`)
	return driftPage("Resources", b.String())
}

func TestWebfeedGeneratorSetupLeavesTheModelOutWhenTold(t *testing.T) {
	web := driftSite()
	web.set(driftListing, twoListsPage())
	judge := &countingJudge{}
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Judge: judge}, nil)
	if _, err := gen.Setup(context.Background(), driftListing, "", true); err != nil {
		t.Fatal(err)
	}
	if judge.calls.Load() == 0 {
		t.Fatal("the fixture never asks the judge, so it proves nothing")
	}
	before := judge.calls.Load()
	got, err := gen.Setup(context.Background(), driftListing, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if judge.calls.Load() != before || got.Cost.USD != 0 {
		t.Errorf("judge calls %d -> %d, cost %+v; want no model call over budget", before, judge.calls.Load(), got.Cost)
	}
}

func TestWebfeedGeneratorPollNeverReachesTheModel(t *testing.T) {
	web := driftSite()
	judge := &countingJudge{}
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Judge: judge}, nil)
	setup, err := gen.Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	before := judge.calls.Load()
	web.set(driftListing, driftPage("Blog", `<main><h1>Topics</h1><div class="topics"><a href="/tags/design">Design</a><a href="/tags/culture">Culture</a></div></main>`))
	prev := PollState{History: []PollRecord{{URLs: urlsOf(2, 10, "blog"), Titles: driftTitles[2:10], Fingerprint: setup.Fingerprint}}}
	out, err := gen.Poll(context.Background(), setup.Recipe, prev)
	if err != nil {
		t.Fatal(err)
	}
	if judge.calls.Load() != before || out.Repair == nil || out.Repair.UsedModel || out.Verdict != VerdictSuspect {
		t.Fatalf("judge calls %d -> %d, outcome %+v, repair %+v", before, judge.calls.Load(), out, out.Repair)
	}
	prev.AllowModelRepair = true
	out, err = gen.Poll(context.Background(), setup.Recipe, prev)
	if err != nil {
		t.Fatal(err)
	}
	if out.Repair == nil || !out.Repair.UsedModel || out.Repair.Step != "judge" {
		t.Fatalf("with the daily model repair free: repair %+v", out.Repair)
	}
}

// countingJudge counts calls; the judge's three votes run in parallel.
type countingJudge struct{ calls atomic.Int32 }

func (j *countingJudge) Judge(ctx context.Context, in webfeed.JudgeInput) (webfeed.Verdict, webfeed.Usage, error) {
	j.calls.Add(1)
	return fixedJudge{}.Judge(ctx, in)
}

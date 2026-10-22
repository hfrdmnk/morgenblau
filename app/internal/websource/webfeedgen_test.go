package websource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/webfeed"
)

// scriptedModel answers by step: the judge (effort high) picks the first list, the weekly check (effort low) says
// broken when told to; it counts the calls per step.
type scriptedModel struct {
	mu     sync.Mutex
	broken bool
	calls  map[webfeed.Effort]int
}

func (m *scriptedModel) Complete(_ context.Context, p webfeed.Prompt) (string, webfeed.Usage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == nil {
		m.calls = map[webfeed.Effort]int{}
	}
	m.calls[p.Effort]++
	switch p.Effort {
	case webfeed.EffortHigh:
		return `{"pick":[0],"order":"date","none":false}`, webfeed.Usage{InputTokens: 2500, OutputTokens: 600, USD: 0.00054}, nil
	case webfeed.EffortLow:
		verdict := "healthy"
		if m.broken {
			verdict = "broken"
		}
		return `{"verdict":"` + verdict + `","reason":"the items look like category pages"}`, webfeed.Usage{InputTokens: 1500, OutputTokens: 100, USD: 0.0002}, nil
	}
	return "", webfeed.Usage{}, errors.New("no reply scripted for this step")
}

func (m *scriptedModel) count(e webfeed.Effort) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[e]
}

func (m *scriptedModel) setBroken(b bool) {
	m.mu.Lock()
	m.broken = b
	m.mu.Unlock()
}

func TestWebfeedGeneratorSetupMapsTheResult(t *testing.T) {
	web := driftSite()
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client()})
	got, err := gen.Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != HealthOK || got.HealthReason != "" || got.Title != "Blog | Example Publication" {
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
	got, err := NewWebfeedGenerator(webfeed.Options{Client: thin.client()}).Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != HealthNeedsAttention || got.HealthReason != "only a few posts were found" {
		t.Errorf("thin listing = %s %q", got.Health, got.HealthReason)
	}

	closed := driftSite()
	closed.setPage("https://blog.example.com/robots.txt", fakePage{ctype: "text/plain", body: "User-agent: *\nDisallow: /blog\n"})
	got, err = NewWebfeedGenerator(webfeed.Options{Client: closed.client()}).Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != HealthBlockedBySite || got.HealthReason != "this site blocks feed readers" {
		t.Errorf("robots-closed listing = %s %q", got.Health, got.HealthReason)
	}

	for _, reasons := range [][]webfeed.Reason{{webfeed.ReasonUnsure}, {"something new"}} {
		if got := setupReason(false, reasons); got != "Morgenblau isn't sure which links are posts" {
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
	judge := &scriptedModel{}
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Model: judge})
	if _, err := gen.Setup(context.Background(), driftListing, "", true); err != nil {
		t.Fatal(err)
	}
	if judge.count(webfeed.EffortHigh) == 0 {
		t.Fatal("the fixture never asks the judge, so it proves nothing")
	}
	before := judge.count(webfeed.EffortHigh)
	got, err := gen.Setup(context.Background(), driftListing, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if judge.count(webfeed.EffortHigh) != before || got.Cost.USD != 0 {
		t.Errorf("judge calls %d -> %d, cost %+v; want no model call over budget", before, judge.count(webfeed.EffortHigh), got.Cost)
	}
}

func TestWebfeedGeneratorPollNeverReachesTheModel(t *testing.T) {
	web := driftSite()
	judge := &scriptedModel{}
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Model: judge})
	setup, err := gen.Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	before := judge.count(webfeed.EffortHigh)
	web.set(driftListing, driftPage("Blog", `<main><h1>Topics</h1><div class="topics"><a href="/tags/design">Design</a><a href="/tags/culture">Culture</a></div></main>`))
	prev := PollState{History: []PollRecord{{URLs: urlsOf(2, 10, "blog"), Titles: driftTitles[2:10], Fingerprint: setup.Fingerprint}}}
	out, err := gen.Poll(context.Background(), setup.Recipe, prev)
	if err != nil {
		t.Fatal(err)
	}
	if judge.count(webfeed.EffortHigh) != before || out.Repair == nil || out.Repair.UsedModel || out.Verdict != VerdictSuspect {
		t.Fatalf("judge calls %d -> %d, outcome %+v, repair %+v", before, judge.count(webfeed.EffortHigh), out, out.Repair)
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

func TestWebfeedGeneratorSetupReportsAnUnreachableSite(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusBadGateway} {
		web := driftSite()
		web.setPage(driftListing, fakePage{status: status, body: "down"})
		_, err := NewWebfeedGenerator(webfeed.Options{Client: web.client()}).Setup(context.Background(), driftListing, "", true)
		if !errors.Is(err, ErrUnreachable) {
			t.Errorf("HTTP %d: err = %v, want ErrUnreachable", status, err)
		}
	}
}

func TestWebfeedGeneratorAnchoredSetupOffersOneModelCheck(t *testing.T) {
	web := driftSite()
	web.set(driftListing, twoListsPage())
	linked := "https://blog.example.com/stories/" + driftSlug(driftTitles[6])
	model := &scriptedModel{}
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Model: model})

	got, err := gen.Setup(context.Background(), driftListing, linked, false)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AnchorOnPage || got.Confirm == nil || len(model.calls) != 0 {
		t.Fatalf("on page %v, confirm %v, calls %v; want the check offered and nothing spent yet", got.AnchorOnPage, got.Confirm != nil, model.calls)
	}
	c, err := got.Confirm(context.Background())
	if err != nil || !c.Agreed || c.USD != 0.0002 || model.count(webfeed.EffortLow) != 1 {
		t.Errorf("confirmation = %+v, %v after %d low-effort calls", c, err, model.count(webfeed.EffortLow))
	}
	model.setBroken(true)
	if c, err := got.Confirm(context.Background()); err != nil || c.Agreed || c.Reason == "" {
		t.Errorf("doubted confirmation = %+v, %v", c, err)
	}

	elsewhere, err := gen.Setup(context.Background(), driftListing, "https://blog.example.com/stories/a-post-this-page-never-links", true)
	if err != nil {
		t.Fatal(err)
	}
	if elsewhere.AnchorOnPage {
		t.Error("a post of the right shape that the page never links counted as on the page")
	}
	plain, err := gen.Setup(context.Background(), driftListing, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Confirm != nil {
		t.Error("an unanchored setup offered an anchor check")
	}
	noModel, err := NewWebfeedGenerator(webfeed.Options{Client: web.client()}).Setup(context.Background(), driftListing, linked, true)
	if err != nil {
		t.Fatal(err)
	}
	if !noModel.AnchorOnPage || noModel.Confirm != nil {
		t.Errorf("without a model: on page %v, confirm %v", noModel.AnchorOnPage, noModel.Confirm != nil)
	}
}

func TestWebfeedGeneratorSetupReportsTheSectionItMovedTo(t *testing.T) {
	web := driftSite()
	web.set("https://blog.example.com/", driftPage("Example Publication", driftList(2, 5, "blog")))
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client()})
	got, err := gen.Setup(context.Background(), "https://blog.example.com/", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.ListingURL != driftListing || len(got.Items) != 8 {
		t.Fatalf("listing = %q with %d items, want the /blog section's eight", got.ListingURL, len(got.Items))
	}
}

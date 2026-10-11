package webfeed

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestRepairModelRungWithoutModelCallsIsNotAModelRepair(t *testing.T) {
	ctx := context.Background()
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	s.set("/blog", page("Blog", siteNav+renamedList(8)+siteFooter))
	polled, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil || polled.Verdict != PollSuspect {
		t.Fatalf("poll = %+v, %v", polled, err)
	}
	judge := countingJudge()
	opts := s.opts()
	opts.Model = judge
	got, err := Repair(ctx, polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res), AllowModel: true}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if calls := len(judge.prompts); calls != 0 {
		t.Fatalf("setup asked the model %d times; this case needs a confident setup", calls)
	}
	if got.UsedModel || got.ModelUSD != 0 {
		t.Errorf("used model %v usd %v with no model call, want the daily model slot left unspent", got.UsedModel, got.ModelUSD)
	}
	if got.Accepted && got.Rule == AcceptModel {
		t.Errorf("accepted by %q with no model call; the looser model rule needs a model pick", got.Rule)
	}
	if !slices.Equal(got.Tried, []RepairStep{RepairHeuristic, RepairJudge}) {
		t.Errorf("tried %v, want the setup rung tried after the free ones", got.Tried)
	}
}

// renamedBesideStories is the renamed list beside an unrelated one of the same shape, so setup cannot choose without the judge.
func renamedBesideStories() string {
	var b strings.Builder
	b.WriteString(`<main><div class="col-a">`)
	for i := range 5 {
		t := fmt.Sprintf("A completely different headline number %d", i)
		if i == 0 {
			t = postTitles[0]
		}
		fmt.Fprintf(&b, `<div class="item"><a href="/articles/%s">%s</a></div>`, slugify(t), t)
	}
	b.WriteString(`</div><div class="col-b">`)
	for i := range 5 {
		t := fmt.Sprintf("Another unrelated story from the archive %d", i)
		fmt.Fprintf(&b, `<div class="item"><a href="/stories/%s">%s</a></div>`, slugify(t), t)
	}
	b.WriteString(`</div></main>`)
	return page("Blog", b.String())
}

func TestRepairModelRungThatAsksTheJudgeIsAModelRepair(t *testing.T) {
	ctx := context.Background()
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	s.set("/blog", renamedBesideStories())
	polled, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil || polled.Verdict != PollSuspect {
		t.Fatalf("poll = %+v, %v", polled, err)
	}
	judge := &fakeModel{replies: map[Effort][]string{EffortHigh: {judgeReply(OrderPage, 0)}}}
	opts := s.opts()
	opts.Model = judge
	got, err := Repair(ctx, polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res), AllowModel: true}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(judge.prompts) == 0 {
		t.Fatal("setup never asked the judge; this case needs an unsure setup")
	}
	if !got.Accepted || got.Step != RepairJudge || got.Rule != AcceptModel || !got.UsedModel || got.ModelUSD == 0 {
		t.Fatalf("repair accepted %v step %q rule %q model %v usd %v items %v, want the judge's pick accepted by the model rule",
			got.Accepted, got.Step, got.Rule, got.UsedModel, got.ModelUSD, itemURLs(got.Items))
	}
}

func TestRepairModelRungCutByItsDeadlineReportsWhatItPaid(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	s.set("/blog", renamedBesideStories())
	polled, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil || polled.Verdict != PollSuspect {
		t.Fatalf("poll = %+v, %v", polled, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	judge := &fakeModel{replies: map[Effort][]string{EffortHigh: {judgeReply(OrderPage, 0)}}, before: func(context.Context, Prompt) { cancel() }}
	opts := s.opts()
	opts.Model = judge
	got, err := Repair(ctx, polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res), AllowModel: true}, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the cancellation", err)
	}
	if got == nil || !got.UsedModel || got.ModelUSD == 0 || got.Accepted {
		t.Fatalf("repair = %+v, want the judge calls it paid for reported and nothing accepted", got)
	}
	if n := len(got.Tried); n == 0 || got.Tried[n-1] != RepairJudge {
		t.Errorf("tried %v, want the setup rung last", got.Tried)
	}
}

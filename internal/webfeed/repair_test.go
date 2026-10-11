package webfeed

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// countingJudge is a model that always picks the first candidate.
func countingJudge() *fakeModel {
	return &fakeModel{replies: map[Effort][]string{EffortHigh: {judgeReply(OrderPage, 0)}}}
}

// shoutedTitles upper-cases every post title, which trips the title-shape rule without moving a single post.
func shoutedTitles(list string) string {
	for _, t := range postTitles {
		list = strings.ReplaceAll(list, ">"+t+"<", ">"+strings.ToUpper(t)+"<")
	}
	return list
}

func archiveList(class string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<ul class="%s">`, class)
	for i := range n {
		fmt.Fprintf(&b, `<li class="old"><h2><a href="/blog/older-post-number-%d">Older post number %d from the archive</a></h2></li>`, i, i)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// renamedList is the blog list with every title rewritten except the first, under a new URL scheme.
func renamedList(n int) string {
	var b strings.Builder
	b.WriteString(`<main><h1>Articles</h1><ul class="articles">`)
	for i := range n {
		t := fmt.Sprintf("A completely different headline number %d", i)
		if i == 0 {
			t = postTitles[0]
		}
		fmt.Fprintf(&b, `<li class="article"><h2><a href="/articles/%s">%s</a></h2><time datetime="2024-03-%02d">March %d, 2024</time></li>`, slugify(t), t, 20-i, 20-i)
	}
	b.WriteString(`</ul></main>`)
	return b.String()
}

func tagCloud() string {
	var b strings.Builder
	b.WriteString(`<main><h1>Topics</h1><div class="topics">`)
	for _, t := range []string{"design", "engineering", "culture", "product", "news", "events"} {
		fmt.Fprintf(&b, `<a class="topic" href="/tags/%s">%s</a>`, t, t)
	}
	b.WriteString(`</div></main>`)
	return b.String()
}

func TestRepairLadder(t *testing.T) {
	listing := siteNav + blogList(8) + siteFooter
	cases := []struct {
		name       string
		after      string
		allowModel bool
		judge      bool
		accepted   bool
		step       RepairStep
		rule       string
		falseAlarm bool
		usedModel  bool
		tried      []RepairStep
		items      int
		itemPrefix string
	}{
		{name: "list cut to three", after: siteNav + blogList(3) + siteFooter,
			accepted: true, step: RepairKnownURLs, rule: AcceptKnownURL, tried: []RepairStep{RepairKnownURLs}, items: 3, itemPrefix: "/blog/"},
		{name: "titles restyled in capitals", after: siteNav + shoutedTitles(blogList(8)) + siteFooter,
			accepted: true, step: RepairKnownURLs, rule: AcceptKnownURL, falseAlarm: true, tried: []RepairStep{RepairKnownURLs}, items: 8, itemPrefix: "/blog/"},
		{name: "url scheme changed, titles carry it", after: siteNav + movedList(8, "posts") + siteFooter,
			accepted: true, step: RepairHeuristic, rule: AcceptTitles, tried: []RepairStep{RepairHeuristic}, items: 8, itemPrefix: "/posts/"},
		{name: "titles rewritten and moved, no model allowed", after: siteNav + renamedList(8) + siteFooter, judge: true,
			tried: []RepairStep{RepairHeuristic}},
		{name: "titles rewritten and moved, a confident setup re-run earns no model leniency", after: siteNav + renamedList(8) + siteFooter, judge: true, allowModel: true,
			tried: []RepairStep{RepairHeuristic, RepairJudge}},
		{name: "only topic links left", after: siteNav + tagCloud() + siteFooter, allowModel: true,
			tried: []RepairStep{RepairHeuristic}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSite(t)
			s.addBlog(8)
			s.set("/blog", page("Blog", listing))
			res := setupRecipe(t, s, "/blog")
			after := tc.after
			if !strings.HasPrefix(after, "<!doctype") {
				after = page("Blog", after)
			}
			s.set("/blog", after)
			ctx := context.Background()
			polled, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
			if err != nil {
				t.Fatal(err)
			}
			if polled.Verdict != PollSuspect {
				t.Fatalf("poll verdict %q (%s), want suspect", polled.Verdict, polled.Reason)
			}
			opts := s.opts()
			judge := countingJudge()
			if tc.judge {
				opts.Model = judge
			}
			got, err := Repair(ctx, polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res), AllowModel: tc.allowModel}, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got.Accepted != tc.accepted || got.Rule != tc.rule || got.FalseAlarm != tc.falseAlarm || got.UsedModel != tc.usedModel {
				t.Fatalf("repair accepted %v rule %q false alarm %v model %v, want %v %q %v %v (tried %v)",
					got.Accepted, got.Rule, got.FalseAlarm, got.UsedModel, tc.accepted, tc.rule, tc.falseAlarm, tc.usedModel, got.Tried)
			}
			if tc.accepted && got.Step != tc.step {
				t.Errorf("accepted step %q, want %q", got.Step, tc.step)
			}
			if !slices.Equal(got.Tried, tc.tried) {
				t.Errorf("tried %v, want %v", got.Tried, tc.tried)
			}
			if len(got.Items) != tc.items {
				t.Fatalf("items %v, want %d", itemURLs(got.Items), tc.items)
			}
			for _, it := range got.Items {
				if !strings.Contains(it.URL, tc.itemPrefix) {
					t.Errorf("item %s outside %s", it.URL, tc.itemPrefix)
				}
			}
			if !tc.accepted {
				return
			}
			again, err := apply(got.Recipe, []byte(after), s.url("/blog"))
			if err != nil || !slices.Equal(itemURLs(again), itemURLs(got.Items)) {
				t.Errorf("the repaired recipe does not reproduce its items: %v, %v", itemURLs(again), err)
			}
			if got.Recipe.Fingerprint.Count != len(got.Items) {
				t.Errorf("fingerprint count %d", got.Recipe.Fingerprint.Count)
			}
		})
	}
}

func TestRepairAnchorsOnTheHistoryTemplateWhenNoKnownPostRemains(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	fresh := strings.NewReplacer("/blog/older-post-number-", "/blog/fresh-post-number-", "Older post number", "Fresh post number").Replace(archiveList("latest", 6))
	after := page("Blog", siteNav+`<main><h1>Blog</h1>`+fresh+`</main>`+siteFooter)
	s.set("/blog", after)
	polled, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil || polled.Verdict != PollSuspect {
		t.Fatalf("poll = %+v, %v", polled, err)
	}
	got, err := Repair(context.Background(), polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted || got.Step != RepairTemplate || got.Rule != AcceptTemplate || len(got.Items) != 6 {
		t.Fatalf("repair = step %q rule %q accepted %v items %v (tried %v)", got.Step, got.Rule, got.Accepted, itemURLs(got.Items), got.Tried)
	}
}

func TestAcceptanceRule(t *testing.T) {
	last := postItems(8, "blog")
	history := []GoodPoll{goodPoll(last)}
	moved := postItems(8, "posts")
	renamed := postItems(8, "posts")
	for i := range renamed {
		renamed[i].Title = fmt.Sprintf("Entirely new headline %d", i)
	}
	renamed[0].Title = last[0].Title
	cases := []struct {
		name  string
		items []Item
		model bool
		want  string
	}{
		{"empty is never accepted", nil, true, ""},
		{"one known post", append(postItems(3, "tags"), last[5]), false, AcceptKnownURL},
		{"history template with three items", postItems(12, "blog")[9:], false, AcceptTemplate},
		{"history template with two items is not enough", postItems(11, "blog")[9:], false, ""},
		{"half the titles under a new scheme", moved[:4], false, AcceptTitles},
		{"one title under a new scheme", renamed, false, ""},
		{"model step with three items and some overlap", renamed, true, AcceptModel},
		{"model step without overlap", renamed[1:], true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := acceptRepair(tc.items, DominantTemplate(tc.items, nil), history, tc.model); got != tc.want {
				t.Fatalf("accept = %q, want %q", got, tc.want)
			}
		})
	}
}

// doubtedPoll is the page with the recipe's current extraction, as a poll hands it to repair after a weekly check doubted it.
func doubtedPoll(t *testing.T, s *testSite, rec Recipe, body string) *PollResult {
	t.Helper()
	p := mustPage(t, body, s.url("/blog"))
	items, err := applyPage(rec, p)
	if err != nil {
		t.Fatal(err)
	}
	return &PollResult{Verdict: PollOK, Items: items, page: &fetchedPage{body: []byte(body), doc: p}}
}

func TestRepairOfADoubtedListNeedsADifferentList(t *testing.T) {
	archive := `<section class="more">` + archiveList("archive", 8) + `</section>`
	posts := page("Blog", siteNav+blogList(8)+siteFooter)
	t.Run("a different list closes the doubt", func(t *testing.T) {
		s := newTestSite(t)
		s.addBlog(8)
		s.set("/blog", page("Blog", siteNav+archive+siteFooter))
		wrong := setupRecipe(t, s, "/blog")
		body := page("Blog", siteNav+blogList(8)+archive+siteFooter)
		polled := doubtedPoll(t, s, wrong.Recipe, body)
		if len(polled.Items) == 0 || slices.Contains(itemURLs(polled.Items), s.url("/blog/"+slugify(postTitles[0]))) {
			t.Fatalf("doubted extraction = %v, want the archive only", itemURLs(polled.Items))
		}
		got, err := Repair(context.Background(), polled, RepairInput{Recipe: wrong.Recipe, History: historyFrom(wrong), Doubted: true}, s.opts())
		if err != nil {
			t.Fatal(err)
		}
		if !got.Accepted || got.Step != RepairHeuristic || got.Rule != AcceptDifferent || got.FalseAlarm {
			t.Fatalf("repair accepted %v step %q rule %q (tried %v)", got.Accepted, got.Step, got.Rule, got.Tried)
		}
		if !slices.Contains(itemURLs(got.Items), s.url("/blog/"+slugify(postTitles[0]))) {
			t.Errorf("items = %v, want the posts list", itemURLs(got.Items))
		}
	})
	t.Run("the same list again does not", func(t *testing.T) {
		s := newTestSite(t)
		s.addBlog(8)
		s.set("/blog", posts)
		res := setupRecipe(t, s, "/blog")
		judge := countingJudge()
		opts := s.opts()
		opts.Model = judge
		got, err := Repair(context.Background(), doubtedPoll(t, s, res.Recipe, posts), RepairInput{Recipe: res.Recipe, History: historyFrom(res), Doubted: true, AllowModel: true}, opts)
		if err != nil {
			t.Fatal(err)
		}
		if got.Accepted {
			t.Fatalf("repair reproduced the doubted list and was accepted by %q at %q", got.Rule, got.Step)
		}
		if !slices.Equal(got.Tried, []RepairStep{RepairHeuristic, RepairJudge}) || got.UsedModel {
			t.Errorf("tried %v (model %v), want the doubted posts skipped as anchors and the setup rung tried without a model call", got.Tried, got.UsedModel)
		}
	})
}

func TestModelAcceptanceNeedsThreeItems(t *testing.T) {
	last := postItems(8, "blog")
	history := []GoodPoll{goodPoll(last)}
	renamed := retitled(postItems(3, "posts"), func(i int) string { return fmt.Sprintf("Entirely new headline %d", i) })
	renamed[0].Title = last[0].Title
	if got := acceptRepair(renamed[:2], DominantTemplate(renamed[:2], nil), history, true); got != "" {
		t.Fatalf("two items with one known title accepted by %q", got)
	}
	if got := acceptRepair(renamed, DominantTemplate(renamed, nil), history, true); got != AcceptModel {
		t.Fatalf("three items with one known title = %q, want %q", got, AcceptModel)
	}
}

func TestRepairRejectsAModelStepWhoseSetupIsBlocked(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	s.set("/blog", page("Blog", siteNav+renamedList(8)+siteFooter))
	ctx := context.Background()
	polled, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
	if err != nil || polled.Verdict != PollSuspect {
		t.Fatalf("poll = %+v, %v", polled, err)
	}
	s.mu.Lock()
	s.status["/blog"] = 403
	s.mu.Unlock()
	opts := s.opts()
	opts.Model = countingJudge()
	got, err := Repair(ctx, polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res), AllowModel: true}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Accepted || len(got.Items) != 0 {
		t.Fatalf("repair accepted %v by %q with items %v, want the blocked setup rejected", got.Accepted, got.Rule, itemURLs(got.Items))
	}
	if len(got.Tried) == 0 || got.Tried[len(got.Tried)-1] != RepairJudge || got.UsedModel {
		t.Fatalf("tried %v (model %v), want the judge step tried last without a model call", got.Tried, got.UsedModel)
	}
}

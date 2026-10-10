package webfeed

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestRenderUserMessageMatchesTheSpecFormat(t *testing.T) {
	long := strings.Repeat("a", 95)
	var many []Item
	for i := range 10 {
		many = append(many, Item{URL: fmt.Sprintf("https://news.example.com/posts/post-%d", i), Title: fmt.Sprintf("Post number %d here", i)})
	}
	many[3].Date = day("2024-01-03")
	many[9].Date = day("2023-12-31")
	cands := []Candidate{
		{Template: "news.example.com/posts/*", Entries: []Item{
			{URL: "https://news.example.com/posts/a", Title: "Alpha beta gamma", Date: day("2024-02-01")},
			{URL: "https://news.example.com/posts/" + strings.Repeat("b", 120), Title: long},
		}},
		{Template: "news.example.com/about/*", InNav: true, Entries: []Item{{URL: "https://news.example.com/about/team", Title: "Team"}}},
		{Template: "news.example.com/posts/*", Entries: many},
	}
	got := renderUserMessage("https://news.example.com/", "Example Publication", cands)
	want := "Page: https://news.example.com/\n" +
		"Title: Example Publication\n" +
		"[0] url shape: news.example.com/posts/* | entries: 2 | dated: 50% (2024-02-01 to 2024-02-01) | median title: 3 words | in nav/header/footer: false | same shape as: [2]\n" +
		"  2024-02-01 | Alpha beta gamma | https://news.example.com/posts/a\n" +
		"  - | " + strings.Repeat("a", 90) + "… | https://news.example.com/posts/" + strings.Repeat("b", 110-len("https://news.example.com/posts/")) + "…\n" +
		"[1] url shape: news.example.com/about/* | entries: 1 | dated: 0% | median title: 1 words | in nav/header/footer: true | same shape as: none\n" +
		"  - | Team | https://news.example.com/about/team\n" +
		"[2] url shape: news.example.com/posts/* | entries: 10 | dated: 20% (2023-12-31 to 2024-01-03) | median title: 4 words | in nav/header/footer: false | same shape as: [0]\n" +
		"  - | Post number 0 here | https://news.example.com/posts/post-0\n" +
		"  - | Post number 1 here | https://news.example.com/posts/post-1\n" +
		"  - | Post number 2 here | https://news.example.com/posts/post-2\n" +
		"  2024-01-03 | Post number 3 here | https://news.example.com/posts/post-3\n" +
		"  - | Post number 5 here | https://news.example.com/posts/post-5\n" +
		"  2023-12-31 | Post number 9 here | https://news.example.com/posts/post-9\n"
	if got != want {
		t.Fatalf("message mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderUserMessageShowsSmallListsInFullAndDedupesIndexes(t *testing.T) {
	var eight []Item
	for i := range 8 {
		eight = append(eight, Item{URL: fmt.Sprintf("https://news.example.com/p/%d", i), Title: "T"})
	}
	if got := strings.Count(renderUserMessage("u", "t", []Candidate{{Entries: eight}}), "\n  "); got != 8 {
		t.Fatalf("n=8 shows %d entries, want all 8", got)
	}
	if got := sampleIndexes(9); !reflect.DeepEqual(got, []int{0, 1, 2, 3, 4, 8}) {
		t.Fatalf("sampleIndexes(9) = %v", got)
	}
}

func TestSystemPromptIsTheFrozenV2Prompt(t *testing.T) {
	if !strings.HasPrefix(SystemPrompt, "You choose which lists on a web page form the stream of posts") ||
		!strings.HasSuffix(SystemPrompt, "none=true when no candidate is a post stream.") ||
		strings.Count(SystemPrompt, "\n") != 6 ||
		!strings.Contains(SystemPrompt, `Return JSON only: {"pick":[ids],"order":"date"|"page","mixed":true|false,"none":true|false,"reason":"one short sentence"}`) {
		t.Fatalf("system prompt drifted from v2:\n%s", SystemPrompt)
	}
}

func TestParseVerdictToleratesFencesAndProse(t *testing.T) {
	cases := []string{
		`{"pick":[0,2],"order":"date","mixed":false,"none":false,"reason":"posts"}`,
		"```json\n{\"pick\":[0,2],\"order\":\"date\",\"mixed\":false,\"none\":false,\"reason\":\"posts\"}\n```",
		"Here is my answer:\n```\n{\"pick\":[\"0\",2],\"order\":\"date\",\"reason\":\"posts\"}\n```\nThanks.",
	}
	for _, c := range cases {
		v, err := ParseVerdict(c, 3)
		if err != nil || !reflect.DeepEqual(v.Pick, []int{0, 2}) || v.Order != OrderDate {
			t.Errorf("ParseVerdict(%q) = %+v, %v", c, v, err)
		}
	}
	if v, err := ParseVerdict(`{"pick":[],"none":true,"order":"page"}`, 3); err != nil || !v.None {
		t.Errorf("none verdict = %+v, %v", v, err)
	}
	for _, bad := range []string{"no json here", `{"pick":[7],"order":"date"}`, `{"pick":[],"none":false}`} {
		if _, err := ParseVerdict(bad, 3); err == nil {
			t.Errorf("ParseVerdict(%q) accepted", bad)
		}
	}
}

func TestAggregateVotes(t *testing.T) {
	fail := errors.New("timeout")
	cases := []struct {
		name      string
		votes     []vote
		wantPick  []int
		wantOrder Order
		wantNone  bool
	}{
		{"majority per candidate", []vote{
			{v: Verdict{Pick: []int{0, 2}, Order: OrderDate}},
			{v: Verdict{Pick: []int{0}, Order: OrderDate}},
			{v: Verdict{Pick: []int{2, 0}, Order: OrderPage}},
		}, []int{0, 2}, OrderDate, false},
		{"failed call votes for the heuristic top", []vote{
			{err: fail},
			{v: Verdict{Pick: []int{1}, Order: OrderPage}},
			{v: Verdict{Pick: []int{0, 1}, Order: OrderPage}},
		}, []int{0, 1}, OrderPage, false},
		{"two say none", []vote{
			{v: Verdict{None: true, Order: OrderPage}},
			{v: Verdict{None: true, Order: OrderPage}},
			{v: Verdict{Pick: []int{1}, Order: OrderDate}},
		}, nil, OrderPage, true},
		{"no majority falls back to most votes then rank", []vote{
			{v: Verdict{Pick: []int{2}, Order: OrderDate}},
			{v: Verdict{Pick: []int{1}, Order: OrderDate}},
			{err: fail},
		}, []int{0}, OrderDate, false},
	}
	for _, c := range cases {
		pick, order, none := aggregateVotes(c.votes, OrderDate)
		if !reflect.DeepEqual(pick, c.wantPick) || order != c.wantOrder || none != c.wantNone {
			t.Errorf("%s: got %v %s %v, want %v %s %v", c.name, pick, order, none, c.wantPick, c.wantOrder, c.wantNone)
		}
	}
}

type fakeJudge struct {
	calls          atomic.Int32
	replies        []string
	inflight, peak atomic.Int32
}

func (f *fakeJudge) Judge(ctx context.Context, in JudgeInput) (Verdict, Usage, error) {
	n := f.calls.Add(1)
	cur := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		p := f.peak.Load()
		if cur <= p || f.peak.CompareAndSwap(p, cur) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	if in.System != SystemPrompt || !strings.HasPrefix(in.User, "Page: ") {
		return Verdict{}, Usage{}, errors.New("unexpected input")
	}
	usage := Usage{InputTokens: 2500, OutputTokens: 600, USD: 0.00054}
	v, err := ParseVerdict(f.replies[int(n-1)%len(f.replies)], in.Candidates)
	return v, usage, err
}

func TestRunJudgeCallsThreeTimesInParallelAndMetersCost(t *testing.T) {
	j := &fakeJudge{replies: []string{`{"pick":[0,1],"order":"date","none":false}`}}
	cands := []Candidate{{Template: "news.example.com/a/*"}, {Template: "news.example.com/a/*"}}
	rec, err := runJudge(context.Background(), j, JudgeInput{System: SystemPrompt, User: "Page: x", Candidates: len(cands)}, len(cands), OrderPage)
	if err != nil {
		t.Fatal(err)
	}
	if j.calls.Load() != 3 || j.peak.Load() != 3 {
		t.Fatalf("calls %d, peak parallel %d; want 3 and 3", j.calls.Load(), j.peak.Load())
	}
	if !reflect.DeepEqual(rec.Pick, []int{0, 1}) || rec.Order != OrderDate || rec.Calls != 3 || rec.InputTokens != 7500 || rec.OutputTokens != 1800 {
		t.Fatalf("record = %+v", rec)
	}
	if rec.USD < 0.00161 || rec.USD > 0.00163 {
		t.Fatalf("usd = %v", rec.USD)
	}
}

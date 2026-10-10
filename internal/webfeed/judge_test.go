package webfeed

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
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

func TestJudgePromptIsTheFrozenV2Prompt(t *testing.T) {
	if !strings.HasPrefix(judgePrompt, "You choose which lists on a web page form the stream of posts") ||
		!strings.HasSuffix(judgePrompt, "none=true when no candidate is a post stream.") ||
		strings.Count(judgePrompt, "\n") != 6 ||
		!strings.Contains(judgePrompt, `Return JSON only: {"pick":[ids],"order":"date"|"page","mixed":true|false,"none":true|false,"reason":"one short sentence"}`) {
		t.Fatalf("system prompt drifted from v2:\n%s", judgePrompt)
	}
}

func TestParseVerdictToleratesFencesAndProse(t *testing.T) {
	cases := []string{
		`{"pick":[0,2],"order":"date","mixed":false,"none":false,"reason":"posts"}`,
		"```json\n{\"pick\":[0,2],\"order\":\"date\",\"mixed\":false,\"none\":false,\"reason\":\"posts\"}\n```",
		"Here is my answer:\n```\n{\"pick\":[\"0\",2],\"order\":\"date\",\"reason\":\"posts\"}\n```\nThanks.",
	}
	for _, c := range cases {
		v, err := parseVerdict(c, 3)
		if err != nil || !reflect.DeepEqual(v.Pick, []int{0, 2}) || v.Order != OrderDate {
			t.Errorf("parseVerdict(%q) = %+v, %v", c, v, err)
		}
	}
	if v, err := parseVerdict(`{"pick":[],"none":true,"order":"page"}`, 3); err != nil || !v.None {
		t.Errorf("none verdict = %+v, %v", v, err)
	}
	for _, bad := range []string{"no json here", `{"pick":[7],"order":"date"}`, `{"pick":[],"none":false}`} {
		if _, err := parseVerdict(bad, 3); err == nil {
			t.Errorf("parseVerdict(%q) accepted", bad)
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

// parallelBarrier holds each call until n calls are in flight at once, and fails the test if they never are.
func parallelBarrier(t *testing.T, n int) func(context.Context, Prompt) {
	var mu sync.Mutex
	arrived := 0
	all := make(chan struct{})
	return func(ctx context.Context, _ Prompt) {
		mu.Lock()
		arrived++
		if arrived == n {
			close(all)
		}
		mu.Unlock()
		select {
		case <-all:
		case <-time.After(2 * time.Second):
			t.Errorf("only %d of %d calls were in flight together", arrived, n)
		}
	}
}

func TestRunJudgeCallsThreeTimesInParallelAndMetersCost(t *testing.T) {
	m := &fakeModel{replies: map[Effort][]string{EffortHigh: {`{"pick":[0,1],"order":"date","none":false}`}}}
	m.before = parallelBarrier(t, judgeVotes)
	rec, err := runJudge(context.Background(), m, "Page: x", 2, OrderPage)
	if err != nil {
		t.Fatal(err)
	}
	prompts := m.calls(EffortHigh)
	if len(prompts) != 3 || prompts[0].System != judgePrompt || prompts[0].User != "Page: x" {
		t.Fatalf("prompts = %+v", prompts)
	}
	if !reflect.DeepEqual(rec.Pick, []int{0, 1}) || rec.Order != OrderDate || rec.Calls != 3 || rec.InputTokens != 7500 || rec.OutputTokens != 1800 {
		t.Fatalf("record = %+v", rec)
	}
	if rec.USD < 0.00161 || rec.USD > 0.00163 {
		t.Fatalf("usd = %v", rec.USD)
	}
}

func TestRunJudgeVoteEdgeCases(t *testing.T) {
	const none = `{"pick":[],"none":true,"order":"page"}`
	cases := []struct {
		name    string
		replies []string
		want    JudgeRecord
	}{
		{"every call fails and the heuristic top stands", nil,
			JudgeRecord{Pick: []int{0}, Order: OrderPage, Calls: 3, Failed: 3}},
		{"one none against two picks of the same list", []string{none, judgeReply(OrderDate, 2), judgeReply(OrderDate, 2)},
			JudgeRecord{Pick: []int{2}, Order: OrderDate, Calls: 3}},
		{"an out-of-range pick fails its vote, which goes to the heuristic top", []string{judgeReply(OrderDate, 9), judgeReply(OrderDate, 0), judgeReply(OrderDate, 1)},
			JudgeRecord{Pick: []int{0}, Order: OrderDate, Calls: 3, Failed: 1}},
		{"a three-way tie goes to the lowest index", []string{judgeReply(OrderDate, 3), judgeReply(OrderDate, 1), judgeReply(OrderDate, 2)},
			JudgeRecord{Pick: []int{1}, Order: OrderDate, Calls: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &fakeModel{replies: map[Effort][]string{EffortHigh: tc.replies}}
			rec, err := runJudge(context.Background(), m, "Page: x", 4, OrderPage)
			if err != nil {
				t.Fatal(err)
			}
			got := JudgeRecord{Pick: rec.Pick, Order: rec.Order, None: rec.None, Calls: rec.Calls, Failed: rec.Failed}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("record = %+v, want %+v", got, tc.want)
			}
		})
	}
}

package webfeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// judgePrompt is the frozen, benchmarked v2 judge prompt; change it only with a new benchmark.
const judgePrompt = `You choose which lists on a web page form the stream of posts a reader subscribes to when they follow this page.
You get the page URL and title, then numbered candidate lists found by a DOM heuristic. Each shows its URL shape, size, how many entries are dated and their date range, the median title length, whether it sits in navigation/header/footer, which other candidates share its URL shape, and sample entries (date | title | url). Lists of up to 8 entries are shown in full.
The reader wants every post this page publishes. Sites often split one stream over several blocks: a hero or featured story, a "latest" strip, the main list, a second grid further down, or a list of another post type (episodes, videos, press releases). Each block becomes its own candidate, often small, undated or with a different URL shape. Pick every candidate whose entries are this site's own individual posts, articles, episodes, releases or announcements, that is headline-like titles linking to single content pages. Leaving out such a block loses posts and is the most common mistake.
Reject candidates whose entries are not individual posts: navigation and section links, category/tag/topic pages, author lists, pagination, product or marketing pages, help/legal/social links, and aggregator meta-links (on a link aggregator the entry is the story link, not the domain or comments link). Also reject "popular"/"related"/sidebar lists. The in nav/header/footer flag is a hint, not a verdict: judge by the entries.
order: "date" when entries carry per-entry dates and newest-first is meaningful; "page" when dates are missing or the page curates its own order.
Return JSON only: {"pick":[ids],"order":"date"|"page","mixed":true|false,"none":true|false,"reason":"one short sentence"}
none=true when no candidate is a post stream.`

// judgeVotes is how many independent calls a judged setup makes; a candidate needs a majority.
const judgeVotes = 3

// Verdict is the judge's answer; Pick indexes the candidates in the user message.
type Verdict struct {
	Pick   []int  `json:"pick"`
	Order  Order  `json:"order"`
	Mixed  bool   `json:"mixed"`
	None   bool   `json:"none"`
	Reason string `json:"reason"`
}

// parseVerdict reads the judge's reply; picks must index one of n candidates.
func parseVerdict(text string, n int) (Verdict, error) {
	var raw struct {
		Pick   []json.RawMessage `json:"pick"`
		Order  string            `json:"order"`
		Mixed  bool              `json:"mixed"`
		None   bool              `json:"none"`
		Reason string            `json:"reason"`
	}
	if err := decodeReply(text, &raw); err != nil {
		return Verdict{}, fmt.Errorf("judge: %w", err)
	}
	v := Verdict{Order: OrderPage, Mixed: raw.Mixed, None: raw.None, Reason: raw.Reason}
	if Order(raw.Order) == OrderDate {
		v.Order = OrderDate
	}
	for _, p := range raw.Pick {
		i, err := strconv.Atoi(strings.Trim(string(p), `"`))
		if err != nil || i < 0 || i >= n {
			return Verdict{}, fmt.Errorf("judge: pick %s out of range", p)
		}
		if !slices.Contains(v.Pick, i) {
			v.Pick = append(v.Pick, i)
		}
	}
	if len(v.Pick) == 0 && !v.None {
		return Verdict{}, errors.New("judge: empty pick without none")
	}
	return v, nil
}

type vote struct {
	v   Verdict
	err error
}

// aggregateVotes applies the majority rule; a failed or unparsable call votes for the heuristic's top candidate.
func aggregateVotes(votes []vote, heuristicOrder Order) (pick []int, order Order, none bool) {
	counts := map[int]int{}
	noneVotes, dateVotes, pageVotes := 0, 0, 0
	for _, vt := range votes {
		v := vt.v
		if vt.err != nil {
			v = Verdict{Pick: []int{0}, Order: heuristicOrder}
		}
		if v.None {
			noneVotes++
		}
		for _, i := range v.Pick {
			counts[i]++
		}
		switch v.Order {
		case OrderDate:
			dateVotes++
		case OrderPage:
			pageVotes++
		}
	}
	order = heuristicOrder
	switch {
	case dateVotes > pageVotes:
		order = OrderDate
	case pageVotes > dateVotes:
		order = OrderPage
	}
	majority := len(votes)/2 + 1
	if noneVotes >= majority {
		return nil, order, true
	}
	for i, c := range counts {
		if c >= majority {
			pick = append(pick, i)
		}
	}
	if len(pick) == 0 {
		best, bestN := 0, 0
		for i, c := range counts {
			if c > bestN || (c == bestN && i < best) {
				best, bestN = i, c
			}
		}
		pick = []int{best}
		if counts[0] == bestN {
			pick = []int{0}
		}
	}
	slices.Sort(pick)
	return pick, order, false
}

// runJudge makes the three votes in parallel and meters their cost.
func runJudge(ctx context.Context, m Model, user string, n int, heuristicOrder Order) (JudgeRecord, error) {
	votes := make([]vote, judgeVotes)
	usages := make([]Usage, judgeVotes)
	var wg sync.WaitGroup
	for i := range judgeVotes {
		wg.Go(func() {
			text, u, err := m.Complete(ctx, Prompt{System: judgePrompt, User: user, Effort: EffortHigh})
			var v Verdict
			if err == nil {
				v, err = parseVerdict(text, n)
			}
			votes[i], usages[i] = vote{v: v, err: err}, u
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return JudgeRecord{}, err
	}
	rec := JudgeRecord{Calls: judgeVotes}
	for i, u := range usages {
		rec.InputTokens += u.InputTokens
		rec.OutputTokens += u.OutputTokens
		rec.USD += u.USD
		if votes[i].err != nil {
			rec.Failed++
		}
	}
	rec.USD = roundUSD(rec.USD)
	rec.Pick, rec.Order, rec.None = aggregateVotes(votes, heuristicOrder)
	if rec.Pick == nil {
		rec.Pick = []int{}
	}
	return rec, nil
}

// renderUserMessage builds the judge's user message in the benchmarked format.
func renderUserMessage(listingURL, pageTitle string, cands []Candidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Page: %s\nTitle: %s\n", listingURL, pageTitle)
	for i, c := range cands {
		n := len(c.Entries)
		var dates []string
		var words []int
		for _, e := range c.Entries {
			if !e.Date.IsZero() {
				dates = append(dates, isoDay(e))
			}
			words = append(words, wordCount(e.Title))
		}
		slices.Sort(dates)
		slices.Sort(words)
		dated := 0
		if n > 0 {
			dated = int(math.Round(float64(len(dates)) / float64(n) * 100))
		}
		fmt.Fprintf(&b, "[%d] url shape: %s | entries: %d | dated: %d%%", i, c.Template, n, dated)
		if len(dates) > 0 {
			fmt.Fprintf(&b, " (%s to %s)", dates[0], dates[len(dates)-1])
		}
		median := 0
		if len(words) > 0 {
			median = words[len(words)/2]
		}
		var same []string
		for k, o := range cands {
			if k != i && o.Template == c.Template {
				same = append(same, fmt.Sprintf("[%d]", k))
			}
		}
		sameShape := "none"
		if len(same) > 0 {
			sameShape = strings.Join(same, ", ")
		}
		fmt.Fprintf(&b, " | median title: %d words | in nav/header/footer: %t | same shape as: %s\n", median, c.InNav, sameShape)
		for _, k := range sampleIndexes(n) {
			e := c.Entries[k]
			d := "-"
			if !e.Date.IsZero() {
				d = isoDay(e)
			}
			fmt.Fprintf(&b, "  %s | %s | %s\n", d, clip(e.Title, 90), clip(e.URL, 110))
		}
	}
	return b.String()
}

func isoDay(e Item) string { return e.Date.Format("2006-01-02") }

// sampleIndexes shows every entry of small lists and a spread of six from larger ones.
func sampleIndexes(n int) []int {
	if n <= 8 {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	var out []int
	for _, i := range []int{0, 1, 2, 3, n / 2, n - 1} {
		if !slices.Contains(out, i) {
			out = append(out, i)
		}
	}
	return out
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

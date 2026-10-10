package webfeed

import (
	"fmt"
	"testing"
	"time"
)

func postItems(n int, path string) []Item {
	items := make([]Item, n)
	for i := range n {
		t := fmt.Sprintf("%s number %d", postTitles[i%len(postTitles)], i)
		items[i] = Item{URL: "https://news.example.com/" + path + "/" + slugify(t), Title: t, Date: time.Date(2026, 9, 30-i, 8, 0, 0, 0, time.UTC)}
	}
	return items
}

func goodPoll(items []Item) GoodPoll {
	g := GoodPoll{URLTemplate: "news.example.com/blog/*", TitleShape: ComputeSignals(items, nil).TitleShape}
	for _, it := range items {
		g.URLs = append(g.URLs, it.URL)
		g.Titles = append(g.Titles, it.Title)
	}
	return g
}

func TestDriftRulesFirstMatchWins(t *testing.T) {
	base := postItems(10, "blog")
	history := []GoodPoll{goodPoll(base), goodPoll(base)}
	templates := []string{"news.example.com/blog/*"}
	undated := func(items []Item) []Item {
		out := append([]Item(nil), items...)
		for i := range out {
			out[i].Date = time.Time{}
		}
		return out
	}
	shouting := func(items []Item) []Item {
		out := append([]Item(nil), items...)
		for i := range out {
			out[i].Title = "READ"
		}
		return out
	}
	allNew := postItems(20, "blog")[10:]
	for i := range allNew {
		allNew[i].URL += "-new"
	}
	cases := []struct {
		name    string
		items   []Item
		history []GoodPoll
		want    string
	}{
		{"unchanged", base, history, ""},
		{"two new posts on top", append(postItems(12, "blog")[10:], base[:8]...), history, ""},
		{"falling date share is recorded, never an alarm", undated(base), history, ""},
		{"empty", nil, history, ReasonEmpty},
		{"list cut to three", base[:3], history, ReasonCountRatio},
		{"three times as many", append(append(append(append([]Item(nil), base...), allNew...), postItems(12, "blog")[10:]...), postItems(20, "news")...), history, ReasonCountRatio},
		{"most posts replaced", allNew, history, ReasonContinuity},
		{"titles turned into labels", shouting(base), history, ReasonTitleShape},
		{"no history skips history rules", base[:2], nil, ""},
		{"duplicate titles without history", shouting(base), nil, ReasonDupTitles},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drift(tc.items, DominantTemplate(tc.items, templates), tc.history); got != tc.want {
				t.Fatalf("drift = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDriftTemplateShareNeedsAChangedTemplate(t *testing.T) {
	base := postItems(10, "blog")
	history := []GoodPoll{goodPoll(base)}
	moved := postItems(10, "posts")
	for i := range moved {
		moved[i].URL = base[i].URL
	}
	// Same URLs, so continuity holds; only the template rule can see a stream whose links now point elsewhere.
	elsewhere := append([]Item(nil), base[:4]...)
	elsewhere = append(elsewhere, postItems(10, "tags")[:6]...)
	history[0].URLs = append(history[0].URLs, itemURLs(elsewhere[4:])...)
	if got := drift(elsewhere, DominantTemplate(elsewhere, nil), history); got != ReasonTemplateShare {
		t.Fatalf("drift = %q, want %q", got, ReasonTemplateShare)
	}
	if got := drift(moved, DominantTemplate(moved, nil), history); got != "" {
		t.Fatalf("same template = %q, want ok", got)
	}
}

func TestDominantTemplatePrefersTheRecipePattern(t *testing.T) {
	items := []Item{
		{URL: "https://news.example.com/fr/blog/money/tax/a-post"},
		{URL: "https://news.example.com/fr/blog/start/ideas/b-post"},
		{URL: "https://news.example.com/about/team"},
	}
	if got := DominantTemplate(items, []string{"news.example.com/fr/blog/*/*/*"}); got != "news.example.com/fr/blog/*/*/*" {
		t.Fatalf("got %q", got)
	}
	if got := DominantTemplate(items[2:], nil); got != "news.example.com/about/*" {
		t.Fatalf("got %q", got)
	}
	if got := DominantTemplate(nil, nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

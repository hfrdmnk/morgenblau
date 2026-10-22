package webfeed

import (
	"fmt"
	"slices"
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
		want    Reason
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

// retitled copies items with each title replaced by title(i).
func retitled(items []Item, title func(i int) string) []Item {
	out := slices.Clone(items)
	for i := range out {
		out[i].Title = title(i)
	}
	return out
}

// freshPosts are the blog posts numbered from..to-1 under URLs no earlier poll saw.
func freshPosts(from, to int) []Item {
	out := postItems(to, "blog")[from:]
	for i := range out {
		out[i].URL += "-fresh"
	}
	return out
}

func joined(lists ...[]Item) []Item {
	var out []Item
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

func TestDriftSuspectRulesInIsolation(t *testing.T) {
	const blog = "news.example.com/blog/*"
	base := postItems(10, "blog")
	history := []GoodPoll{goodPoll(base)}
	withShape := func(shape float64) []GoodPoll {
		g := goodPoll(base)
		g.TitleShape = shape
		return []GoodPoll{g}
	}
	labels := func(good int) []Item {
		return retitled(base, func(i int) string {
			if i < good {
				return base[i].Title
			}
			return fmt.Sprintf("Label%d", i)
		})
	}
	moved := joined(base[:4], postItems(10, "news")[4:])
	mixed := joined(base[:4], postItems(7, "news")[4:], postItems(10, "tags")[7:])
	half := joined(base[:5], postItems(10, "news")[5:])
	recent := joined(base[:4], freshPosts(4, 10))
	cases := []struct {
		name    string
		items   []Item
		history []GoodPoll
		want    Reason
	}{
		{"count below half the median", base[:4], history, ReasonCountRatio},
		{"count at exactly half the median", base[:5], history, ""},
		{"count above three times the median", postItems(31, "blog"), history, ReasonCountRatio},
		{"count at exactly three times the median", postItems(30, "blog"), history, ""},
		{"most known posts gone and most items new", recent, history, ReasonContinuity},
		{"many new posts while every known post stays", joined(base, freshPosts(10, 22)), history, ""},
		{"most known posts gone but few items new", joined(base[:4], freshPosts(4, 7)), history, ""},
		{"exactly half kept and half new", joined(base[:5], freshPosts(5, 10)), history, ""},
		{"continuity compares with the newest poll", recent, []GoodPoll{goodPoll(recent), goodPoll(base)}, ""},
		{"continuity ignores an older poll it would fail", recent, []GoodPoll{goodPoll(base), goodPoll(recent)}, ReasonContinuity},
		{"template share low and a new template dominates", moved, []GoodPoll{goodPoll(moved)}, ReasonTemplateShare},
		{"template share low across mixed shapes but the old one still dominates", mixed, []GoodPoll{goodPoll(mixed)}, ""},
		{"template share at exactly half", half, []GoodPoll{goodPoll(half)}, ""},
		{"titles lose their shape after a well-shaped history", labels(0), history, ReasonTitleShape},
		{"title shape history at exactly 0.8", labels(0), withShape(0.8), ReasonTitleShape},
		{"title shape history below 0.8 never alarms", labels(0), withShape(0.75), ""},
		{"current title shape at exactly 0.7", labels(7), history, ""},
		{"most titles duplicated", retitled(base, func(i int) string { return fmt.Sprintf("A shared headline about topic %d", i%4) }), history, ReasonDupTitles},
		{"exactly half the titles duplicated", retitled(base, func(i int) string { return fmt.Sprintf("A shared headline about topic %d", i%5) }), history, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drift(tc.items, DominantTemplate(tc.items, []string{blog}), tc.history); got != tc.want {
				t.Fatalf("drift = %q, want %q", got, tc.want)
			}
		})
	}
}

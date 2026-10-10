package webfeed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDigestPageSummarizesWhatAReaderSees(t *testing.T) {
	html := page("Blog | Example Publication", `<script>var hidden = "do not show";</script>`+siteNav+blogList(4)+
		`<a href="https://elsewhere.example.org/x">Partner</a>`+siteFooter)
	d, err := digestPage([]byte(html), "https://news.example.com/blog")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Blog | Example Publication" {
		t.Fatalf("title = %q", d.Title)
	}
	if len(d.Headings) != 5 || d.Headings[0] != "Blog" || d.Headings[1] != postTitles[0] {
		t.Fatalf("headings = %q", d.Headings)
	}
	if strings.Contains(d.Text, "do not show") || !strings.Contains(d.Text, postTitles[0]) || len([]rune(d.Text)) > digestTextChars+1 {
		t.Fatalf("text = %q", d.Text)
	}
	// 13 header links, 4 posts, 1 partner, 6 footer links of which 3 point at social hosts.
	if d.Links != 24 || d.SameSiteLinks != 20 {
		t.Fatalf("links = %d same-site %d", d.Links, d.SameSiteLinks)
	}
}

func TestRenderWatchMessage(t *testing.T) {
	d := pageDigest{Title: "News", Headings: []string{"News", "Latest"}, Text: "Latest stories from Example Publication", Links: 40, SameSiteLinks: 30}
	day := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	var good []Item
	for i := range 25 {
		good = append(good, Item{URL: fmt.Sprintf("https://news.example.com/p/%d", i), Title: fmt.Sprintf("Story %d", i), Date: day})
	}
	cur := []Item{{URL: "https://news.example.com/tag/a", Title: "Topics"}}
	msg := renderWatchMessage("https://news.example.com/", d, good, cur)
	for _, want := range []string{
		"Page: https://news.example.com/\n", "Title: News\n", "Headings: News | Latest\n", "Text: Latest stories from Example Publication\n",
		"Links: 40 (30 on this site)\n", "Last good items (25, showing 20):\n", "  2024-03-01 | Story 0 | https://news.example.com/p/0\n",
		"Current items (1):\n", "  - | Topics | https://news.example.com/tag/a\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "Story 20 ") {
		t.Errorf("message shows more than 20 good items:\n%s", msg)
	}
}

func TestParseWatchVerdict(t *testing.T) {
	cases := map[string]watchVerdict{
		`{"verdict":"healthy","reason":"new posts"}`:                {Broken: false, Reason: "new posts"},
		"```json\n{\"verdict\":\"BROKEN\",\"reason\":\"nav\"}\n```": {Broken: true, Reason: "nav"},
	}
	for in, want := range cases {
		if got, err := parseWatchVerdict(in); err != nil || got != want {
			t.Errorf("parseWatchVerdict(%q) = %+v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"healthy", `{"verdict":"maybe"}`, `{"reason":"x"}`} {
		if _, err := parseWatchVerdict(bad); err == nil {
			t.Errorf("parseWatchVerdict(%q) accepted", bad)
		}
	}
}

func TestWeeklyCheckMetersTheCallAndOnlyRaisesAlarms(t *testing.T) {
	body := []byte(page("Blog", blogList(3)))
	cur := []Item{{URL: "https://news.example.com/blog/a", Title: "A post"}}
	m := &fakeModel{replies: map[Effort][]string{EffortLow: {`{"verdict":"broken","reason":"category pages"}`}}}
	rec, err := WeeklyCheck(context.Background(), m, "https://news.example.com/blog", body, cur, cur)
	if err != nil {
		t.Fatal(err)
	}
	if in := m.calls(EffortLow); len(in) != 1 || in[0].System != watchPrompt || !strings.HasPrefix(in[0].User, "Page: https://news.example.com/blog\n") {
		t.Fatalf("prompts = %+v", in)
	}
	if !rec.Broken || rec.Reason != "category pages" || rec.InputTokens != 1500 || rec.OutputTokens != 120 || rec.USD != 0.00021 {
		t.Fatalf("record = %+v", rec)
	}

	for name, m := range map[string]*fakeModel{
		"call failed":    {errs: map[Effort][]error{EffortLow: {errors.New("overloaded")}}},
		"reply unusable": {replies: map[Effort][]string{EffortLow: {"It looks broken to me."}}},
	} {
		rec, err = WeeklyCheck(context.Background(), m, "https://news.example.com/blog", body, cur, cur)
		if err == nil || rec.Broken || rec.InputTokens != 1500 {
			t.Fatalf("%s: check = %+v, %v; a failed check must not raise an alarm but must still be metered", name, rec, err)
		}
	}
}

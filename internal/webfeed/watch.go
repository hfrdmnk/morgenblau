package webfeed

import (
	"context"
	"fmt"
	"strings"
)

// watchPrompt drives the weekly check. Not benchmarked; change it only with a new benchmark.
const watchPrompt = `You check once a week whether a generated feed still follows the right posts on a web page.
You get the page URL, a digest of the page as fetched now (title, headings, a visible-text excerpt, link counts), the last items the feed accepted as good, and the items its recipe extracts from the page now (date | title | url).
Answer broken when the current items are no longer this page's stream of posts: navigation, category or tag links, a "popular" or "related" list, another section of the site, a cookie or consent wall, an error, login or bot-check page, a parked or moved site, or a list cut down to a few entries while the page plainly shows many more posts. Also answer broken when the page shows newer posts than any current item.
Answer healthy when the current items are individual posts from this page, even when they differ from the last good items because new posts were published or old ones dropped off.
Return JSON only: {"verdict":"healthy"|"broken","reason":"one short sentence"}`

const (
	digestHeadings  = 15
	digestTextChars = 1500
	watchItems      = 20
)

// watchVerdict is the weekly check's answer.
type watchVerdict struct {
	Broken bool
	Reason string
}

// WatchRecord is a weekly check's outcome and metered cost.
type WatchRecord struct {
	Broken       bool    `json:"broken"`
	Reason       string  `json:"reason,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	USD          float64 `json:"usd,omitempty"`
}

// pageDigest is what the weekly check sees of the page as fetched now.
type pageDigest struct {
	Title         string
	Headings      []string
	Text          string
	Links         int
	SameSiteLinks int
}

// WeeklyCheck is the weekly second opinion on a generated source: one call that can raise an alarm, never close one.
// On error the record carries the cost and Broken stays false.
func WeeklyCheck(ctx context.Context, m Model, listingURL string, page []byte, lastGood, current []Item) (WatchRecord, error) {
	d, err := digestPage(page, listingURL)
	if err != nil {
		return WatchRecord{}, err
	}
	text, u, err := m.Complete(ctx, Prompt{System: watchPrompt, User: renderWatchMessage(listingURL, d, lastGood, current), Effort: EffortLow})
	rec := WatchRecord{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, USD: roundUSD(u.USD)}
	if err != nil {
		return rec, err
	}
	v, err := parseWatchVerdict(text)
	if err != nil {
		return rec, err
	}
	rec.Broken, rec.Reason = v.Broken, v.Reason
	return rec, nil
}

// digestPage reads the title, headings, a visible-text excerpt and link counts from a fetched page.
func digestPage(body []byte, pageURL string) (pageDigest, error) {
	p, err := parsePage(body, pageURL)
	if err != nil {
		return pageDigest{}, err
	}
	d := pageDigest{Title: p.title}
	for _, n := range p.doc.Find("body h1, body h2, body h3").Nodes {
		if t := nodeText(n); t != "" && len(d.Headings) < digestHeadings {
			d.Headings = append(d.Headings, clip(t, 90))
		}
	}
	d.Text = clip(visibleText(p), digestTextChars)
	for _, n := range p.doc.Find("body a[href]").Nodes {
		u, err := p.base.Parse(strings.TrimSpace(attr(n, "href")))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		d.Links++
		if sameSite(strings.ToLower(u.Hostname()), strings.ToLower(p.url.Hostname())) {
			d.SameSiteLinks++
		}
	}
	return d, nil
}

// renderWatchMessage builds the weekly check's user message; item lists show their first 20 entries.
func renderWatchMessage(listingURL string, d pageDigest, lastGood, current []Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Page: %s\nTitle: %s\nHeadings: %s\nText: %s\nLinks: %d (%d on this site)\n", listingURL, d.Title, strings.Join(d.Headings, " | "), d.Text, d.Links, d.SameSiteLinks)
	writeItems := func(label string, items []Item) {
		fmt.Fprintf(&b, "%s (%d", label, len(items))
		if len(items) > watchItems {
			fmt.Fprintf(&b, ", showing %d", watchItems)
		}
		b.WriteString("):\n")
		for _, it := range items[:min(len(items), watchItems)] {
			date := "-"
			if !it.Date.IsZero() {
				date = isoDay(it)
			}
			fmt.Fprintf(&b, "  %s | %s | %s\n", date, clip(it.Title, 90), clip(it.URL, 110))
		}
	}
	writeItems("Last good items", lastGood)
	writeItems("Current items", current)
	return b.String()
}

// parseWatchVerdict reads the weekly check's reply.
func parseWatchVerdict(text string) (watchVerdict, error) {
	var raw struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := decodeReply(text, &raw); err != nil {
		return watchVerdict{}, fmt.Errorf("watch: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(raw.Verdict)) {
	case "healthy":
		return watchVerdict{Reason: raw.Reason}, nil
	case "broken":
		return watchVerdict{Broken: true, Reason: raw.Reason}, nil
	}
	return watchVerdict{}, fmt.Errorf("watch: verdict %q is neither healthy nor broken", raw.Verdict)
}

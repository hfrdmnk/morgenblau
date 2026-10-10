package webfeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// WatchSystemPrompt drives the weekly check; it was not benchmarked in this wording, since the research prompt was deleted with the prototype.
const WatchSystemPrompt = `You check once a week whether a generated feed still follows the right posts on a web page.
You get the page URL, a digest of the page as fetched now (title, headings, a visible-text excerpt, link counts), the last items the feed accepted as good, and the items its recipe extracts from the page now (date | title | url).
Answer broken when the current items are no longer this page's stream of posts: navigation, category or tag links, a "popular" or "related" list, another section of the site, a cookie or consent wall, an error, login or bot-check page, a parked or moved site, or a list cut down to a few entries while the page plainly shows many more posts. Also answer broken when the page shows newer posts than any current item.
Answer healthy when the current items are individual posts from this page, even when they differ from the last good items because new posts were published or old ones dropped off.
Return JSON only: {"verdict":"healthy"|"broken","reason":"one short sentence"}`

const (
	digestHeadings  = 15
	digestTextChars = 1500
	watchItems      = 20
)

// Watcher is the weekly second opinion on a generated feed: one model call that can raise an alarm, never close one.
type Watcher interface {
	Watch(ctx context.Context, in WatchInput) (WatchVerdict, Usage, error)
}

// WatchInput is one weekly-check call.
type WatchInput struct {
	System string
	User   string
}

// WatchVerdict is the watcher's answer.
type WatchVerdict struct {
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

// PageDigest is what the weekly check sees of the page as fetched now.
type PageDigest struct {
	Title         string
	Headings      []string
	Text          string
	Links         int
	SameSiteLinks int
}

// WeeklyCheck asks the watcher about one generated source; on error the record carries the cost and Broken stays false.
func WeeklyCheck(ctx context.Context, w Watcher, listingURL string, page []byte, lastGood, current []Item) (WatchRecord, error) {
	d, err := DigestPage(page, listingURL)
	if err != nil {
		return WatchRecord{}, err
	}
	v, u, err := w.Watch(ctx, WatchInput{System: WatchSystemPrompt, User: RenderWatchMessage(listingURL, d, lastGood, current)})
	rec := WatchRecord{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, USD: roundUSD(u.USD)}
	if err != nil {
		return rec, err
	}
	rec.Broken, rec.Reason = v.Broken, v.Reason
	return rec, nil
}

// DigestPage reads the title, headings, a visible-text excerpt and link counts from a fetched page.
func DigestPage(body []byte, pageURL string) (PageDigest, error) {
	p, err := parsePage(body, pageURL)
	if err != nil {
		return PageDigest{}, err
	}
	d := PageDigest{Title: p.title}
	for _, n := range p.doc.Find("body h1, body h2, body h3").Nodes {
		if t := nodeText(n); t != "" && len(d.Headings) < digestHeadings {
			d.Headings = append(d.Headings, clip(t, 90))
		}
	}
	if body := p.doc.Find("body"); body.Length() > 0 {
		d.Text = clip(nodeText(body.Nodes[0]), digestTextChars)
	}
	base := pageBase(p)
	for _, n := range p.doc.Find("body a[href]").Nodes {
		u, err := base.Parse(strings.TrimSpace(attr(n, "href")))
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

// RenderWatchMessage builds the weekly check's user message; item lists show their first 20 entries.
func RenderWatchMessage(listingURL string, d PageDigest, lastGood, current []Item) string {
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

// ParseWatchVerdict reads the watcher's JSON, tolerating code fences and prose around it.
func ParseWatchVerdict(text string) (WatchVerdict, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return WatchVerdict{}, errors.New("watch: no JSON object in reply")
	}
	var raw struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return WatchVerdict{}, fmt.Errorf("watch: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(raw.Verdict)) {
	case "healthy":
		return WatchVerdict{Reason: raw.Reason}, nil
	case "broken":
		return WatchVerdict{Broken: true, Reason: raw.Reason}, nil
	}
	return WatchVerdict{}, fmt.Errorf("watch: verdict %q is neither healthy nor broken", raw.Verdict)
}

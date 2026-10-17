package webfeed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"

	"morgenblau/internal/postkey"
)

// inducePrompt asks for a CSS recipe. Not benchmarked; change it only with a new benchmark.
const inducePrompt = `You write CSS selectors that pull the stream of posts out of a web page, so a feed reader can follow a page that has no feed.
You get the page URL, its title and a compressed skeleton of its HTML: one element per line, indented by depth, written as tag#id.class [attribute=value] "own text". A chain of single wrappers is joined with " > " on one line. Runs of similar siblings are cut to a few examples followed by a line "… N more tag.class"; your selectors must match all of them.
Find this site's own individual posts, articles, episodes, releases or announcements: headline-like titles that link to single content pages. Ignore navigation, category and tag links, pagination, author lists, "popular"/"related"/sidebar lists, product and marketing pages, and social links. Sites often split one stream over several blocks (a featured story, a latest strip, the main list); cover all of them, with a comma-separated selector list when needed.
Prefer tag names, stable class names and structure over hashed class names such as css-1x2y3z or _a8f3k.
Return JSON only: {"item":"...","link":"...","title":"...","date":"...","reason":"one short sentence"}
item: matches exactly one element per post. link: inside an item, the <a> whose href is the post; "" when the item element is that link itself. title: inside an item, the element whose text is the headline; "" to use the link's text. date: inside an item, the element holding the publish date, such as a <time>; "" when posts show no date.`

const (
	induceBelowItems = 3
	maxSkeletonChars = 60_000
	induceRounds     = 2
)

// InduceRecord is the induction step's outcome and metered cost.
type InduceRecord struct {
	Calls        int     `json:"calls"`
	Failed       int     `json:"failed,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	USD          float64 `json:"usd,omitempty"`
	// Items is how many posts the last induced recipe yielded; Kept says whether it replaced the extractor's result.
	Items int  `json:"items"`
	Kept  bool `json:"kept"`
}

// ModelUsage is every model call a setup paid for, including the judge on a page a section move left behind.
type ModelUsage struct {
	Calls, JudgeCalls, InputTokens, OutputTokens int
	USD                                          float64
}

func (r SetupRecord) ModelUsage() ModelUsage {
	var u ModelUsage
	judges := []*JudgeRecord{r.Judge}
	if r.Section != nil {
		judges = append(judges, r.Section.Judge)
	}
	for _, j := range judges {
		if j != nil {
			u.JudgeCalls += j.Calls
			u.InputTokens += j.InputTokens
			u.OutputTokens += j.OutputTokens
			u.USD += j.USD
		}
	}
	u.Calls = u.JudgeCalls
	if r.Induce != nil {
		u.Calls += r.Induce.Calls
		u.InputTokens += r.Induce.InputTokens
		u.OutputTokens += r.Induce.OutputTokens
		u.USD += r.Induce.USD
	}
	u.USD = roundUSD(u.USD)
	return u
}

// ModelCost sums every model call setup made, for logging and storing per source.
func (r SetupRecord) ModelCost() (calls int, usd float64) {
	u := r.ModelUsage()
	return u.Calls, u.USD
}

// parseCSSRecipe reads the inducer's reply; every selector must compile.
func parseCSSRecipe(text string) (CSSRecipe, error) {
	var r CSSRecipe
	if err := decodeReply(text, &r); err != nil {
		return CSSRecipe{}, fmt.Errorf("induce: %w", err)
	}
	r = CSSRecipe{Item: strings.TrimSpace(r.Item), Link: strings.TrimSpace(r.Link), Title: strings.TrimSpace(r.Title), Date: strings.TrimSpace(r.Date)}
	if err := r.validate(); err != nil {
		return CSSRecipe{}, err
	}
	return r, nil
}

func (r CSSRecipe) validate() error {
	if r.Item == "" {
		return errors.New("induce: empty item selector")
	}
	for _, sel := range []string{r.Item, r.Link, r.Title, r.Date} {
		if sel == "" {
			continue
		}
		if _, err := cascadia.ParseGroup(sel); err != nil {
			return fmt.Errorf("induce: selector %q: %w", sel, err)
		}
	}
	return nil
}

// induce asks for a CSS recipe, repairs it once when it yields nothing, and keeps it only when it beats the extractor on the self-check.
func induce(ctx context.Context, res *Result, p *pageDoc, m Model) error {
	rec := &InduceRecord{}
	setup := &res.Recipe.Setup
	setup.Stages = append(setup.Stages, "induce")
	setup.Induce = rec
	skel := skeleton(p, maxSkeletonChars)
	var css CSSRecipe
	var items []Item
	problem := ""
	for range induceRounds {
		text, u, err := m.Complete(ctx, Prompt{System: inducePrompt, User: renderInduceMessage(res.Recipe.ListingURL, p.title, skel, css, problem), Effort: EffortMedium})
		var c CSSRecipe
		if err == nil {
			c, err = parseCSSRecipe(text)
		}
		rec.Calls++
		rec.InputTokens += u.InputTokens
		rec.OutputTokens += u.OutputTokens
		rec.USD = roundUSD(rec.USD + u.USD)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			rec.Failed++
			css, problem = CSSRecipe{}, "Your reply was not a usable recipe ("+err.Error()+")."
			continue
		}
		css = c
		var matched int
		items, matched, err = applyCSS(p, css)
		switch {
		case err != nil:
			problem = "It failed: " + err.Error() + "."
		case len(items) == 0:
			problem = fmt.Sprintf("The item selector matched %d elements and none of them gave a post link.", matched)
		default:
			problem = ""
		}
		if problem == "" {
			break
		}
	}
	rec.Items = len(items)
	if problem != "" || len(items) == 0 {
		return nil
	}

	templates := dominantTemplates(items)
	sig := ComputeSignals(items, templates)
	reasons := sig.doubts()
	if len(reasons) >= len(ComputeSignals(res.Items, res.Recipe.Fingerprint.URLTemplates).doubts()) {
		return nil
	}
	rec.Kept = true
	r := &res.Recipe
	r.Mode, r.CSS = ModeCSS, &css
	r.Streams, r.ExcludeURLs = []Stream{}, []string{}
	r.Order = orderFor(sig.DateFrac)
	r.DateSource = DateSourceNone
	if sig.DateFrac > 0 {
		r.DateSource = DateSourceListing
	}
	sortItems(items, r.Order)
	r.Fingerprint = fingerprintOf(sig, templates)
	res.Items, res.Reasons = items, reasons
	return nil
}

// renderInduceMessage shows the page skeleton and, on a repair round, the recipe that failed and how.
func renderInduceMessage(listingURL, title, skel string, failed CSSRecipe, problem string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Page: %s\nTitle: %s\nSkeleton:\n%s\n", listingURL, title, skel)
	if problem != "" {
		b.WriteString("\nYour previous recipe did not work.")
		if failed.Item != "" {
			raw, _ := json.Marshal(failed)
			fmt.Fprintf(&b, " Recipe: %s.", raw)
		}
		fmt.Fprintf(&b, " %s Write a different recipe.\n", problem)
	}
	return b.String()
}

// applyCSS runs induced selectors on a parsed page; matched counts the item elements, for repair feedback.
func applyCSS(p *pageDoc, css CSSRecipe) (items []Item, matched int, err error) {
	if err := css.validate(); err != nil {
		return nil, 0, err
	}
	sel := p.doc.Find(css.Item)
	seen := map[string]bool{}
	sel.Each(func(_ int, it *goquery.Selection) {
		link := within(it, css.Link)
		if css.Link == "" && goquery.NodeName(it) != "a" {
			link = it.Find("a[href]").First()
		}
		href, ok := link.Attr("href")
		if !ok {
			return
		}
		u, ok := resolveLink(p.base, href)
		if !ok || sameDoc(u, p.url) || seen[postkey.URL(u)] {
			return
		}
		seen[postkey.URL(u)] = true
		item := Item{URL: u}
		titleNode := link
		if css.Title != "" {
			titleNode = within(it, css.Title)
		}
		if titleNode.Length() > 0 {
			item.Title = cleanTitle(nodeText(titleNode.Nodes[0]))
		}
		if isGeneric(item.Title) {
			item.Title = slugTitle(u)
		}
		if css.Date != "" {
			if d := within(it, css.Date); d.Length() > 0 {
				if t, ok := parseDateAttr(attr(d.Nodes[0], "datetime")); ok {
					item.Date = t
				} else if t, ok := parseDateText(nodeText(d.Nodes[0])); ok {
					item.Date = t
				}
			}
		}
		items = append(items, item)
	})
	return items, sel.Length(), nil
}

// within is the first match of sel inside an item, or the item itself when it matches.
func within(it *goquery.Selection, sel string) *goquery.Selection {
	if sel == "" || it.Is(sel) {
		return it
	}
	return it.Find(sel).First()
}

// dominantTemplates is the URL shape most items share, generalized over a varying category segment when that covers more.
func dominantTemplates(items []Item) []string {
	counts := map[string]int{}
	var ts []string
	for _, it := range items {
		t := URLTemplate(it.URL)
		counts[t]++
		ts = append(ts, t)
	}
	best := ""
	for t, n := range counts {
		if n > counts[best] || (n == counts[best] && t < best) {
			best = t
		}
	}
	depth := strings.Count(best, "/")
	host, _, _ := strings.Cut(best, "/")
	same := slices.DeleteFunc(slices.Clone(ts), func(t string) bool {
		h, _, _ := strings.Cut(t, "/")
		return h != host || strings.Count(t, "/") != depth
	})
	if g := generalizeTemplates(same); g != "" && len(same) > counts[best] {
		return []string{g}
	}
	return []string{best}
}

// skelLevel is one compression setting; skeleton tries them from most to least detailed until one fits.
type skelLevel struct {
	text, repeats, classes int
}

var skelLevels = []skelLevel{{80, 3, 4}, {40, 2, 3}, {24, 1, 2}}

// skeleton compresses the listing's body for the inducer: no scripts, styles or empty decoration, one wrapper chain per line,
// repeated siblings cut to examples, and a hard cut at limit as the last resort.
func skeleton(p *pageDoc, limit int) string {
	body := p.doc.Find("body")
	if body.Length() == 0 {
		return ""
	}
	content := map[*html.Node]bool{}
	markContent(body.Nodes[0], content)
	var out string
	for _, lv := range skelLevels {
		var b strings.Builder
		writeChildren(&b, body.Nodes[0], 0, lv, content)
		out = b.String()
		if len(out) <= limit {
			return out
		}
	}
	const marker = "… (skeleton cut here)\n"
	cut := out[:limit-len(marker)]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i+1]
	}
	return cut + marker
}

var skelSkip = map[string]bool{
	"script": true, "style": true, "template": true, "svg": true, "head": true, "link": true, "meta": true, "iframe": true,
	"img": true, "picture": true, "source": true, "video": true, "audio": true, "canvas": true, "input": true, "select": true, "path": true,
}

// markContent records which elements hold visible text or a link somewhere below them.
func markContent(n *html.Node, content map[*html.Node]bool) bool {
	has := false
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				has = true
			}
		case html.ElementNode:
			if skelSkip[c.Data] {
				continue
			}
			if markContent(c, content) || (c.Data == "a" && attr(c, "href") != "") {
				content[c] = true
				has = true
			}
		}
	}
	return has
}

func writeChildren(b *strings.Builder, n *html.Node, depth int, lv skelLevel, content map[*html.Node]bool) {
	var run []*html.Node
	flush := func() {
		for _, c := range run[:min(len(run), lv.repeats)] {
			writeNode(b, c, depth, lv, content)
		}
		if extra := len(run) - lv.repeats; extra > 0 {
			fmt.Fprintf(b, "%s… %d more %s\n", strings.Repeat(" ", depth), extra, skelLabel(run[0], 64))
		}
		run = run[:0]
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || !content[c] {
			continue
		}
		if len(run) > 0 && skelLabel(run[0], 64) != skelLabel(c, 64) {
			flush()
		}
		run = append(run, c)
	}
	if len(run) > 0 {
		flush()
	}
}

func writeNode(b *strings.Builder, n *html.Node, depth int, lv skelLevel, content map[*html.Node]bool) {
	b.WriteString(strings.Repeat(" ", depth))
	for {
		b.WriteString(skelLabel(n, lv.classes))
		if href := attr(n, "href"); n.Data == "a" && href != "" {
			fmt.Fprintf(b, " [href=%s]", clip(href, 100))
		}
		if dt := attr(n, "datetime"); dt != "" {
			fmt.Fprintf(b, " [datetime=%s]", clip(dt, 40))
		}
		own := ownText(n)
		next := soleContentChild(n, content)
		if own != "" || next == nil {
			if own != "" {
				fmt.Fprintf(b, " %q", clip(own, lv.text))
			}
			b.WriteByte('\n')
			writeChildren(b, n, depth+1, lv, content)
			return
		}
		b.WriteString(" > ")
		n = next
	}
}

// soleContentChild is the only child element with content, when the element holds nothing else worth a line.
func soleContentChild(n *html.Node, content map[*html.Node]bool) *html.Node {
	var only *html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && content[c] {
			if only != nil {
				return nil
			}
			only = c
		}
	}
	return only
}

func ownText(n *html.Node) string {
	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			parts = append(parts, c.Data)
		}
	}
	return collapse(strings.Join(parts, " "))
}

// skelLabel writes an element as tag#id.class, keeping at most maxClasses classes of bounded length.
func skelLabel(n *html.Node, maxClasses int) string {
	var b strings.Builder
	b.WriteString(n.Data)
	if id := attr(n, "id"); id != "" && len(id) <= 40 {
		b.WriteString("#" + id)
	}
	for i, c := range classes(n) {
		if i == maxClasses {
			break
		}
		if len(c) <= 40 {
			b.WriteString("." + c)
		}
	}
	return b.String()
}

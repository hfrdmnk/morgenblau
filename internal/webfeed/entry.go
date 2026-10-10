package webfeed

import (
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"

	"morgenblau/internal/postkey"
)

// segment is one visually separate run of text inside an entry.
type segment struct {
	text    string
	owner   *html.Node
	inLink  bool // inside a link to the entry's own URL
	inOther bool // inside a link elsewhere, such as a category
}

func (p *pageDoc) anchorFor(n *html.Node) *anchor {
	if p.byNode == nil {
		p.byNode = make(map[*html.Node]*anchor, len(p.anchors))
		for _, a := range p.anchors {
			p.byNode[a.node] = a
		}
	}
	return p.byNode[n]
}

func (p *pageDoc) linkKey(n *html.Node, stop *html.Node) (key string, linked bool) {
	for x := n; x != nil; x = x.Parent {
		if x.Type == html.ElementNode && x.Data == "a" {
			if a := p.anchorFor(x); a != nil {
				return a.key, true
			}
			return "", true
		}
		if x == stop {
			break
		}
	}
	return "", false
}

func (p *pageDoc) segments(e *entry) []segment {
	key := postkey.URL(e.URL)
	var out []segment
	byOwner := map[*html.Node]int{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "template", "svg", "button", "select":
				return
			}
		}
		if n.Type == html.TextNode {
			t := collapse(n.Data)
			if t == "" {
				return
			}
			owner := n.Parent
			for owner != e.node && owner.Parent != nil && inlineTags[owner.Data] && owner.Data != "a" {
				owner = owner.Parent
			}
			if i, ok := byOwner[owner]; ok {
				out[i].text = collapse(out[i].text + " " + t)
				return
			}
			k, linked := p.linkKey(n.Parent, e.node)
			byOwner[owner] = len(out)
			out = append(out, segment{text: t, owner: owner, inLink: linked && k == key, inOther: linked && k != key})
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(e.node)
	// An entry that is itself the link has every segment inside it.
	if e.node.Data == "a" {
		for i := range out {
			out[i].inLink = !out[i].inOther || out[i].inLink
		}
	}
	return out
}

var reTrailingArrow = regexp.MustCompile(`[\s→›»↗→⟶➜➔\-–—>|]+$`)

func cleanTitle(s string) string {
	s = collapse(s)
	s = reTrailingArrow.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

var genericPhrases = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range []string{
		"read more", "read", "more", "read article", "read the article", "read post", "read the post", "read story", "continue reading",
		"learn more", "view more", "see more", "view", "view all", "see all", "all posts", "show more", "load more", "keep reading",
		"read full story", "full story", "listen", "watch", "download", "open", "details", "discover more", "find out more",
		"mehr lesen", "weiterlesen", "mehr erfahren", "mehr", "zum artikel", "zum beitrag", "lire la suite", "en savoir plus", "lire",
		"leer más", "leer mas", "ver más", "saiba mais", "ler mais", "leggi di più", "leggi tutto", "scopri di più", "lees meer",
		"lees verder", "läs mer", "les mer", "læs mere", "czytaj więcej", "続きを読む", "詳しく見る", "詳細", "もっと見る", "자세히 보기", "더 보기", "阅读更多", "查看更多",
	} {
		m[s] = true
	}
	return m
}()

func isGeneric(s string) bool {
	t := strings.ToLower(strings.Trim(cleanTitle(s), " .…:!"))
	return t == "" || genericPhrases[t]
}

func isDateLike(s string) bool {
	if utf8.RuneCountInString(s) > 40 {
		return false
	}
	if _, ok := parseDateText(s); ok {
		return true
	}
	return reRelativeDate.MatchString(s)
}

var reRelativeDate = regexp.MustCompile(`(?i)^\s*(\d+\s*(min|mins|minutes?|h|hours?|d|days?|w|weeks?|mo|months?|y|years?)\s*(ago)?|yesterday|today|gestern|heute|vor \d+ \S+)\s*$`)

var reReadingTime = regexp.MustCompile(`(?i)^\s*\d+\s*(min|mins|minute|minutes|minuten|分)\b`)

func wordCount(s string) int {
	n := 0
	for _, w := range strings.Fields(s) {
		if strings.IndexFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
			continue
		}
		// Scripts without spaces count one word per four characters, so long Japanese press titles still fit 30 words.
		if cjk := countCJK(w); cjk > 0 {
			n += max(1, cjk/4)
			continue
		}
		n++
	}
	return n
}

func countCJK(s string) int {
	n := 0
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			n++
		}
	}
	return n
}

// fillEntries picks each entry's title and date, using repetition across entries to recognise category labels.
func (p *pageDoc) fillEntries(c *cluster) {
	segs := make([][]segment, len(c.entries))
	freq := map[string]int{}
	for i, e := range c.entries {
		segs[i] = p.segments(e)
		e.text = nodeText(e.node)
		seen := map[string]bool{}
		for _, s := range segs[i] {
			k := strings.ToLower(s.text)
			if !seen[k] {
				seen[k] = true
				freq[k]++
			}
		}
	}
	n := len(c.entries)
	isLabel := func(s string) bool {
		f := freq[strings.ToLower(s)]
		return n >= 3 && f >= 2 && float64(f) >= 0.2*float64(n)
	}
	reject := func(s string) bool {
		return isGeneric(s) || isLabel(s) || isDateLike(s) || reReadingTime.MatchString(s) || !hasLetter(s)
	}
	for i, e := range c.entries {
		p.chooseTitle(e, segs[i], reject)
		e.Date = p.entryDate(e, segs[i])
		e.seeAll = (e.TitleSource == TitleSlug || e.TitleSource == TitleAttr) && p.onlyGenericLinkText(e)
	}
}

// onlyGenericLinkText reports an entry whose links to its URL say only "See all" or "Read more": a section link, not a post.
func (p *pageDoc) onlyGenericLinkText(e *entry) bool {
	key := postkey.URL(e.URL)
	worded := false
	for _, n := range descendants(e.node, func(n *html.Node) bool { return n.Data == "a" }) {
		if a := p.anchorFor(n); a == nil || a.key != key {
			continue
		}
		t := nodeText(n)
		if t == "" {
			continue
		}
		if !isGeneric(t) {
			return false
		}
		worded = true
	}
	return worded
}

func hasLetter(s string) bool { return strings.IndexFunc(s, unicode.IsLetter) >= 0 }

func (p *pageDoc) chooseTitle(e *entry, segs []segment, reject func(string) bool) {
	key := postkey.URL(e.URL)
	set := func(t string, src TitleSource, tag string) {
		e.Title, e.TitleSource, e.titleTag = cleanTitle(t), src, tag
	}
	for _, h := range descendants(e.node, isHeading) {
		if k, linked := p.linkKey(h, e.node); linked && k != key {
			continue
		}
		if links := descendants(h, func(n *html.Node) bool { return n.Data == "a" }); len(links) > 0 {
			if a := p.anchorFor(links[0]); a == nil || a.key != key {
				continue
			}
		}
		t := nodeText(h)
		if !reject(t) {
			set(t, TitleHeading, h.Data)
			return
		}
	}
	for _, n := range descendants(e.node, isTitleClass) {
		if k, linked := p.linkKey(n, e.node); linked && k != key {
			continue
		}
		t := nodeText(n)
		if !reject(t) && wordCount(t) <= 40 {
			set(t, TitleClass, n.Data)
			return
		}
	}
	pick := func(ok func(segment) bool) bool {
		for _, s := range segs {
			if s.inOther || reject(s.text) || !ok(s) {
				continue
			}
			set(s.text, TitleAnchor, s.owner.Data)
			return true
		}
		return false
	}
	headline := func(s segment) bool { w := wordCount(s.text); return w >= 2 && w <= 30 }
	if pick(func(s segment) bool { return s.inLink && headline(s) }) ||
		pick(headline) ||
		pick(func(s segment) bool { return s.inLink && wordCount(s.text) <= 30 }) {
		return
	}
	for _, a := range descendants(e.node, func(n *html.Node) bool { return n.Data == "a" || n.Data == "img" }) {
		for _, name := range []string{"aria-label", "title", "alt"} {
			if t := attr(a, name); t != "" && !reject(t) && !genericAlt(t) {
				set(t, TitleAttr, a.Data)
				return
			}
		}
	}
	set(slugTitle(e.URL), TitleSlug, "")
}

func genericAlt(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "image", "thumbnail", "cover", "cover image", "featured image", "photo", "picture", "logo", "icon", "avatar", "hero", "banner":
		return true
	}
	return false
}

// slugTitle humanizes the last path segment, the title of last resort.
func slugTitle(raw string) string {
	last, _ := lastSegment(raw)
	if dec, err := url.PathUnescape(last); err == nil {
		last = dec
	}
	last = strings.TrimSuffix(last, ".html")
	last = strings.TrimSuffix(last, ".htm")
	words := strings.FieldsFunc(last, func(r rune) bool { return r == '-' || r == '_' || r == '+' })
	if len(words) == 0 {
		return last
	}
	s := strings.Join(words, " ")
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func (p *pageDoc) entryDate(e *entry, segs []segment) (t time.Time) {
	for _, n := range descendants(e.node, func(n *html.Node) bool { return n.Data == "time" }) {
		if d, ok := parseDateAttr(attr(n, "datetime")); ok {
			return d
		}
		if d, ok := parseDateText(nodeText(n)); ok {
			return d
		}
	}
	for _, n := range descendants(e.node, func(n *html.Node) bool { return attr(n, "itemprop") == "datePublished" }) {
		for _, v := range []string{attr(n, "content"), attr(n, "datetime"), nodeText(n)} {
			if d, ok := parseDateAttr(v); ok {
				return d
			}
		}
	}
	for _, s := range segs {
		if s.text == e.Title || utf8.RuneCountInString(s.text) > 40 {
			continue
		}
		if d, ok := parseDateText(s.text); ok {
			return d
		}
	}
	if d, ok := dateFromURL(e.URL); ok {
		return d
	}
	return t
}

func isHeading(n *html.Node) bool {
	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return attr(n, "role") == "heading"
}

func isTitleClass(n *html.Node) bool {
	if ip := attr(n, "itemprop"); ip == "headline" || ip == "name" {
		return true
	}
	if v := attr(n, "data-title"); v != "" && v != "false" {
		return true
	}
	for _, c := range append(classes(n), attr(n, "id")) {
		// Tailwind variants and arbitrary values ([&_[data-title]]:x) style children; they do not name this element.
		if strings.ContainsAny(c, "[:/") {
			continue
		}
		for _, tok := range strings.FieldsFunc(strings.ToLower(c), func(r rune) bool { return r == '-' || r == '_' }) {
			switch tok {
			case "title", "headline", "heading":
				return true
			}
		}
	}
	return false
}

// descendants lists matching elements under n in document order, n included.
func descendants(n *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode {
			if match(x) {
				out = append(out, x)
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

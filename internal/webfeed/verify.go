package webfeed

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// articleInfo is what one fetched post page says about itself.
type articleInfo struct {
	url       string
	ok        bool
	ogTitle   string
	h1        string
	siteName  string
	published time.Time
	isArticle bool
	// page is the parsed post, held only while extracting; lists and prose keep what isHub needs.
	page  *pageDoc
	lists [][]string
	prose int
	clean string // ogTitle or h1 without the site-name affix
}

func (s *session) article(ctx context.Context, rawURL string) *articleInfo {
	info := &articleInfo{url: rawURL}
	r := s.fetchOnce(ctx, rawURL, maxArticleBody)
	if r.err != nil || r.status >= 400 || !r.isHTML() || isChallenge(r.body) {
		return info
	}
	p, err := parsePage(r.body, r.url)
	if err != nil {
		return info
	}
	info.ok, info.page = true, p
	defer func() { info.page = nil }()
	doc := p.doc
	meta := func(names ...string) string {
		for _, n := range names {
			for _, attrName := range []string{"property", "name", "itemprop"} {
				if v, ok := doc.Find(`meta[` + attrName + `="` + n + `"]`).First().Attr("content"); ok && strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
		return ""
	}
	info.ogTitle = collapse(meta("og:title", "twitter:title"))
	info.siteName = collapse(meta("og:site_name", "application-name"))
	info.h1 = collapse(doc.Find("h1").First().Text())
	ogType := strings.ToLower(meta("og:type"))
	info.isArticle = strings.Contains(ogType, "article") || ogType == "blog" || doc.Find("article").Length() == 1
	if t, ok := parseDateAttr(meta("article:published_time", "og:published_time", "datePublished", "pubdate", "publish-date", "publish_date", "date", "dc.date.issued", "DC.date.issued", "sailthru.date", "parsely-pub-date")); ok {
		info.published = t
	}
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		var v any
		if json.Unmarshal([]byte(sel.Text()), &v) != nil {
			return true
		}
		if reArticleType.MatchString(sel.Text()) {
			info.isArticle = true
		}
		if info.published.IsZero() {
			if d, ok := parseDateAttr(findJSONLD(v, "datePublished")); ok {
				info.published = d
			}
		}
		return true
	})
	// A page with many <time> elements is a list, not one dated post.
	if times := doc.Find("time[datetime]"); info.published.IsZero() && times.Length() <= 2 {
		scope := doc.Find("article time[datetime], main time[datetime]").First()
		if scope.Length() == 0 {
			scope = times.First()
		}
		if v, ok := scope.Attr("datetime"); ok {
			if d, ok := parseDateAttr(v); ok {
				info.published = d
			}
		}
	}
	if !info.isArticle && info.published.IsZero() {
		info.summarizeLists()
	}
	return info
}

// summarizeLists keeps the URL shapes of the page's main lists and its prose length, all isHub needs once the page is dropped.
func (a *articleInfo) summarizeLists() {
	self := urlKey(a.url)
	for _, c := range a.page.clusters() {
		if c.sideList() {
			continue
		}
		var ts []string
		for _, e := range c.entries {
			if urlKey(e.URL) != self {
				ts = append(ts, URLTemplate(e.URL))
			}
		}
		a.lists = append(a.lists, ts)
	}
	a.page.doc.Find("p").Each(func(_ int, sel *goquery.Selection) {
		if sel.Closest("a").Length() == 0 && sel.Find("a").Length() == 0 {
			a.prose += len(collapse(sel.Text()))
		}
	})
}

var reArticleType = regexp.MustCompile(`"@type"\s*:\s*\[?\s*"[A-Za-z]*(Article|Posting)"`)

// findJSONLD returns the first string value under key anywhere in a JSON-LD tree, including @graph.
func findJSONLD(v any, key string) string {
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x[key].(string); ok {
			return s
		}
		if arr, ok := x[key].([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok {
				return s
			}
		}
		for _, k := range []string{"@graph", "mainEntity", "mainEntityOfPage"} {
			if s := findJSONLD(x[k], key); s != "" {
				return s
			}
		}
	case []any:
		for _, e := range x {
			if s := findJSONLD(e, key); s != "" {
				return s
			}
		}
	}
	return ""
}

// isHub reports an item that is itself a listing of the stream's posts: one big list of them and little prose of its own.
func (a *articleInfo) isHub(template string) bool {
	if !a.ok || a.isArticle || !a.published.IsZero() {
		return false
	}
	biggest := 0
	for _, ts := range a.lists {
		n := 0
		for _, t := range ts {
			if matchTemplate(template, t) {
				n++
			}
		}
		biggest = max(biggest, n)
	}
	// prose counts paragraph text outside links, the body an article has and a listing lacks.
	return biggest >= 6 && (biggest >= 12 || a.prose < 1500)
}

// fetchArticles fetches post pages at most maxArticleConcurrency at a time; a page asked for again is not refetched.
func (s *session) fetchArticles(ctx context.Context, urls []string) map[string]*articleInfo {
	out := make(map[string]*articleInfo, len(urls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxArticleConcurrency)
	for _, u := range urls {
		s.mu.Lock()
		cached, ok := s.articles[u]
		s.mu.Unlock()
		if ok {
			out[u] = cached
			continue
		}
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			info := s.article(ctx, u)
			mu.Lock()
			out[u] = info
			mu.Unlock()
			s.mu.Lock()
			s.articles[u] = info
			s.mu.Unlock()
		})
	}
	wg.Wait()
	cleanArticleTitles(out)
	return out
}

var titleSeparators = []string{" | ", " – ", " — ", " - ", " · ", " :: ", " • ", " « ", " » "}

// cleanArticleTitles strips the site-name affix that repeats across a site's page titles.
func cleanArticleTitles(infos map[string]*articleInfo) {
	affixes := map[string]int{}
	split := func(t string) (head, tail string, ok bool) {
		best := -1
		sep := ""
		for _, s := range titleSeparators {
			if i := strings.LastIndex(t, s); i > best {
				best, sep = i, s
			}
		}
		if best <= 0 {
			return t, "", false
		}
		return t[:best], t[best+len(sep):], true
	}
	splitFirst := func(t string) (head, tail string, ok bool) {
		best := len(t)
		sep := ""
		for _, s := range titleSeparators {
			if i := strings.Index(t, s); i >= 0 && i < best {
				best, sep = i, s
			}
		}
		if sep == "" {
			return t, "", false
		}
		return t[:best], t[best+len(sep):], true
	}
	for _, info := range infos {
		if _, tail, ok := split(info.ogTitle); ok {
			affixes[strings.ToLower(tail)]++
		}
		if head, _, ok := splitFirst(info.ogTitle); ok {
			affixes[strings.ToLower(head)]++
		}
		// Pages whose whole title is the site name vouch for that name as an affix elsewhere.
		if _, _, ok := split(info.ogTitle); !ok && info.ogTitle != "" {
			affixes[strings.ToLower(info.ogTitle)]++
		}
	}
	for _, info := range infos {
		t := info.ogTitle
		if t == "" {
			t = info.h1
		}
		site := strings.ToLower(info.siteName)
		if head, tail, ok := split(t); ok && (affixes[strings.ToLower(tail)] >= 2 || strings.EqualFold(tail, site)) {
			t = head
		} else if head, tail, ok := splitFirst(t); ok && (affixes[strings.ToLower(head)] >= 2 || strings.EqualFold(head, site)) {
			t = tail
		}
		info.clean = strings.TrimSpace(t)
	}
	// A title several different pages share is the site's name, not any one post's.
	seen := map[string]int{}
	for _, info := range infos {
		if info.clean != "" {
			seen[strings.ToLower(info.clean)]++
		}
	}
	for _, info := range infos {
		if seen[strings.ToLower(info.clean)] >= 2 {
			info.clean = ""
		}
	}
}

// titleSimilarity is the share of the shorter title's words found in the longer one.
func titleSimilarity(a, b string) float64 {
	ta, tb := titleTokens(a), titleTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	set := map[string]bool{}
	for _, t := range tb {
		set[t] = true
	}
	hit := 0
	for _, t := range ta {
		if set[t] {
			hit++
		}
	}
	return float64(hit) / float64(len(ta))
}

func titleTokens(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul):
			flush()
			out = append(out, string(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

// segmentMatching finds the entry element whose text is the article's own title.
func segmentMatching(e *entry, title string) *html.Node {
	want := strings.ToLower(collapse(title))
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && !inlineTags[n.Data] && strings.ToLower(nodeText(n)) == want {
			found = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(e.node)
	// Prefer the innermost element with exactly that text.
	for found != nil {
		var inner *html.Node
		for c := found.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && strings.ToLower(nodeText(c)) == want {
				inner = c
			}
		}
		if inner == nil {
			break
		}
		found = inner
	}
	return found
}

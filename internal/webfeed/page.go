package webfeed

import (
	"bytes"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// pageDoc is one parsed listing page with its usable links.
type pageDoc struct {
	url     *url.URL
	doc     *goquery.Document
	title   string
	lang    string
	order   map[*html.Node]int
	anchors []*anchor
	byNode  map[*html.Node]*anchor
	mixed   map[*html.Node]bool // elements holding links to more than one URL
}

// anchor is one <a href> that could be a post link.
type anchor struct {
	node    *html.Node
	url     string
	key     string
	tmpl    string
	chain   []*html.Node // root first, ending at node
	tagPath string
}

func parsePage(body []byte, pageURL string) (*pageDoc, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	expandNoscript(root)
	doc := goquery.NewDocumentFromNode(root)
	p := &pageDoc{url: u, doc: doc, order: map[*html.Node]int{}}
	p.title = collapse(doc.Find("head title").First().Text())
	if lang, ok := doc.Find("html").Attr("lang"); ok {
		p.lang = strings.TrimSpace(lang)
	}
	base := u
	if href, ok := doc.Find("head base[href]").Attr("href"); ok {
		if b, err := u.Parse(strings.TrimSpace(href)); err == nil {
			base = b
		}
	}
	i := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			p.order[n] = i
			i++
			switch n.Data {
			case "head", "script", "style", "template", "svg":
				return
			}
			if n.Data == "a" {
				if a := p.newAnchor(n, base); a != nil {
					p.anchors = append(p.anchors, a)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range doc.Nodes {
		walk(n)
	}
	return p, nil
}

func (p *pageDoc) newAnchor(n *html.Node, base *url.URL) *anchor {
	abs, ok := resolveLink(base, attr(n, "href"))
	if !ok || sameDoc(abs, p.url) {
		return nil
	}
	a := &anchor{node: n, url: abs, key: urlKey(abs), tmpl: URLTemplate(abs)}
	var tags []string
	for x := n; x != nil && x.Type == html.ElementNode; x = x.Parent {
		a.chain = append(a.chain, x)
	}
	for i, j := 0, len(a.chain)-1; i < j; i, j = i+1, j-1 {
		a.chain[i], a.chain[j] = a.chain[j], a.chain[i]
	}
	for _, x := range a.chain {
		tags = append(tags, x.Data)
	}
	a.tagPath = strings.Join(tags, ">")
	return a
}

// resolveLink makes an href absolute and drops fragments, tracking parameters and links that are never posts.
func resolveLink(base *url.URL, href string) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return "", false
	}
	u, err := base.Parse(href)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	u.Fragment = ""
	u.RawFragment = ""
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if strings.HasPrefix(strings.ToLower(k), "utm_") || k == "ref" || k == "fbclid" || k == "gclid" {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}
	if (u.Path == "" || u.Path == "/") && u.RawQuery == "" {
		return "", false
	}
	if ext := strings.ToLower(path.Ext(u.Path)); nonPostExtensions[ext] {
		return "", false
	}
	return u.String(), true
}

// urlKey identifies a URL for dedupe: scheme, www. and a trailing slash do not make a different post.
func urlKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	key := host + strings.TrimSuffix(u.EscapedPath(), "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

func sameDoc(abs string, page *url.URL) bool {
	return urlKey(abs) == urlKey(page.String())
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func classes(n *html.Node) []string {
	return strings.Fields(attr(n, "class"))
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		switch x.Type {
		case html.TextNode:
			b.WriteString(x.Data)
			return
		case html.ElementNode:
			switch x.Data {
			case "script", "style", "template", "svg":
				return
			}
		}
		block := x.Type == html.ElementNode && !inlineTags[x.Data]
		if block {
			b.WriteByte(' ')
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if block {
			b.WriteByte(' ')
		}
	}
	walk(n)
	return collapse(b.String())
}

// inlineTags never start a new visual text segment.
var inlineTags = map[string]bool{
	"a": true, "b": true, "i": true, "em": true, "strong": true, "code": true, "mark": true, "abbr": true,
	"sup": true, "sub": true, "u": true, "s": true, "q": true, "cite": true, "kbd": true, "wbr": true, "bdi": true, "bdo": true,
}

// expandNoscript parses each <noscript> body as markup, since we fetch without JavaScript and see what it holds.
// Each one is parsed on its own: a tracking iframe left open inside one must not swallow the rest of the page.
func expandNoscript(root *html.Node) {
	for _, n := range descendants(root, func(n *html.Node) bool { return n.Data == "noscript" }) {
		text := n.FirstChild
		if text == nil || text.Type != html.TextNode || text.NextSibling != nil || !strings.Contains(text.Data, "<") {
			continue
		}
		context := n.Parent
		if context == nil || context.Type != html.ElementNode {
			continue
		}
		nodes, err := html.ParseFragment(strings.NewReader(text.Data), context)
		if err != nil {
			continue
		}
		n.RemoveChild(text)
		for _, c := range nodes {
			n.AppendChild(c)
		}
	}
}

// nonPostExtensions are feeds and images; documents such as PDFs stay, since papers and reports are posts.
var nonPostExtensions = map[string]bool{
	".xml": true, ".rss": true, ".atom": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".avif": true, ".ico": true,
}

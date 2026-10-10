package webfeed

import (
	"fmt"
	"strings"
	"testing"
)

const siteNav = `<header class="site"><a href="/">Example Publication</a><nav>
<a href="/about">About</a><a href="/blog">Blog</a><a href="/pricing">Pricing</a><a href="/careers">Careers</a>
<a href="/docs">Docs</a><a href="/contact">Contact</a><a href="/login">Log in</a><a href="/signup">Sign up</a>
<a href="/customers">Customers</a><a href="/changelog">Changelog</a><a href="/security">Security</a><a href="/status">Status</a>
</nav></header>`

const siteFooter = `<footer><ul>
<li><a href="https://x.com/example">X</a></li><li><a href="https://github.com/example">GitHub</a></li>
<li><a href="https://www.linkedin.com/company/example">LinkedIn</a></li><li><a href="/privacy">Privacy policy</a></li>
<li><a href="/terms">Terms of service</a></li><li><a href="/imprint">Imprint</a></li></ul></footer>`

var postTitles = []string{
	"How we rebuilt the reading queue from scratch",
	"A quieter way to follow the news",
	"Shipping the offline mode for every reader",
	"What we learned from a year of daily digests",
	"Designing calm notifications that respect attention",
	"The case for fewer, better feeds",
	"Inside the new archive search engine",
	"Why our servers now sleep at night",
	"Small teams and slow software in practice",
	"Notes on typography for long reading sessions",
	"Building trust with transparent changelogs",
	"An honest look at our infrastructure costs",
}

func slugify(s string) string {
	return strings.Trim(strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		}
		return '-'
	}, s), "-")
}

func page(title, body string) string {
	return `<!doctype html><html lang="en"><head><title>` + title + `</title></head><body>` + body + `</body></html>`
}

// blogList is a plain list of n posts under /blog/<slug> with headings and <time>.
func blogList(n int) string {
	var b strings.Builder
	b.WriteString(`<main><h1>Blog</h1><ul class="posts">`)
	for i := range n {
		t := postTitles[i%len(postTitles)]
		fmt.Fprintf(&b, `<li class="post"><h2><a href="/blog/%s">%s</a></h2><time datetime="2024-03-%02d">March %d, 2024</time><p>A short excerpt that explains the post in one or two sentences for readers.</p></li>`, slugify(t), t, 20-i, 20-i)
	}
	b.WriteString(`</ul></main>`)
	return b.String()
}

func entryURLs(c *cluster) []string {
	out := make([]string, len(c.entries))
	for i, e := range c.entries {
		out[i] = e.URL
	}
	return out
}

func mustPage(t *testing.T, html, pageURL string) *pageDoc {
	t.Helper()
	p, err := parsePage([]byte(html), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

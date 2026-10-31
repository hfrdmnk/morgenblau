// Package socialspec reads a site's SocialSpec profile (socialspec.org): /.well-known/about.md, contact.vcf and following.opml.
package socialspec

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	goldtext "github.com/yuin/goldmark/text"
	"go.yaml.in/yaml/v3"
)

const (
	maxFileBytes = 256 << 10
	maxFollowing = 500
)

// Profile is what a site says about its author; every part is optional.
type Profile struct {
	Name      string    `json:"name,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	AboutHTML string    `json:"aboutHtml,omitempty"`
	Contact   []Contact `json:"contact,omitempty"`
	Following []Follow  `json:"following,omitempty"`
}

// Contact is an email address or a web link from contact.vcf.
type Contact struct {
	Kind      string `json:"kind"` // "email" | "url"
	Value     string `json:"value"`
	Preferred bool   `json:"preferred,omitempty"`
}

// Follow is one feed from following.opml.
type Follow struct {
	Title   string `json:"title"`
	FeedURL string `json:"feedUrl"`
	SiteURL string `json:"siteUrl,omitempty"`
}

// Fetch reads origin's three files concurrently; nil, nil means the site publishes none. client must be the safehttp client: origin comes from the reader.
func Fetch(ctx context.Context, client *http.Client, origin string) (*Profile, error) {
	base := strings.TrimSuffix(origin, "/") + "/.well-known/"
	var (
		wg                  sync.WaitGroup
		about, card, follow []byte
	)
	for name, dst := range map[string]*[]byte{"about.md": &about, "contact.vcf": &card, "following.opml": &follow} {
		wg.Go(func() { *dst = get(ctx, client, base+name) })
	}
	wg.Wait()

	// The vCard and OPML parsers reject an HTML fallback page by shape; plain Markdown has no shape to check.
	if bytes.HasPrefix(bytes.TrimSpace(about), []byte("<")) {
		about = nil
	}
	p := &Profile{Following: following(follow)}
	p.Summary, p.AboutHTML = aboutPage(about, base+"about.md")
	p.Name, p.Contact = contact(card)
	if p.Name == "" && p.Summary == "" && p.AboutHTML == "" && len(p.Contact) == 0 && len(p.Following) == 0 {
		return nil, nil
	}
	return p, nil
}

// get treats every failure as absence: each file is optional and a broken one shouldn't hide the others.
func get(ctx context.Context, client *http.Client, u string) []byte {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes))
	if err != nil {
		return nil
	}
	return body
}

func webURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func contact(card []byte) (string, []Contact) {
	text := string(card)
	if !strings.Contains(text, "BEGIN:VCARD") {
		return "", nil
	}
	// RFC 6350 folds long lines; a continuation starts with one space or tab.
	text = vcardFold.ReplaceAllString(text, "")
	var (
		name     string
		contacts []Contact
	)
	for line := range strings.Lines(text) {
		head, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ":")
		if !ok {
			continue
		}
		params := strings.Split(head, ";")
		prop := strings.ToUpper(params[0])
		if _, after, grouped := strings.Cut(prop, "."); grouped {
			prop = after
		}
		value = strings.TrimSpace(value)
		preferred := strings.Contains(strings.ToUpper(strings.Join(params[1:], ";")), "PREF")
		switch {
		case prop == "FN":
			name = value
		case prop == "EMAIL" && strings.Contains(value, "@"):
			contacts = append(contacts, Contact{Kind: "email", Value: value, Preferred: preferred})
		case prop == "URL" && webURL(value):
			contacts = append(contacts, Contact{Kind: "url", Value: value, Preferred: preferred})
		}
	}
	return name, contacts
}

type outline struct {
	Text     string    `xml:"text,attr"`
	Title    string    `xml:"title,attr"`
	XMLURL   string    `xml:"xmlUrl,attr"`
	HTMLURL  string    `xml:"htmlUrl,attr"`
	Children []outline `xml:"outline"`
}

func following(doc []byte) []Follow {
	var parsed struct {
		XMLName  xml.Name  `xml:"opml"`
		Outlines []outline `xml:"body>outline"`
	}
	if len(doc) == 0 || xml.Unmarshal(doc, &parsed) != nil {
		return nil
	}
	var (
		out  []Follow
		seen = map[string]bool{}
		walk func([]outline)
	)
	walk = func(items []outline) {
		for _, o := range items {
			if len(out) >= maxFollowing {
				return
			}
			if feed := strings.TrimSpace(o.XMLURL); webURL(feed) && !seen[feed] {
				seen[feed] = true
				title := strings.TrimSpace(o.Text)
				if title == "" {
					title = strings.TrimSpace(o.Title)
				}
				site := strings.TrimSpace(o.HTMLURL)
				if !webURL(site) {
					site = ""
				}
				out = append(out, Follow{Title: title, FeedURL: feed, SiteURL: site})
			}
			walk(o.Children)
		}
	}
	walk(parsed.Outlines)
	return out
}

var (
	vcardFold   = regexp.MustCompile(`\r?\n[ \t]`)
	frontmatter = regexp.MustCompile(`(?s)\A---[ \t]*\r?\n(?:(.*?)\r?\n)?---[ \t]*(?:\r?\n|\z)`)
	markdown    = goldmark.New(goldmark.WithExtensions(extension.GFM))
	// Links open outside the app, which would otherwise navigate away from the reader's sheet.
	aboutPolicy = bluemonday.UGCPolicy().AddTargetBlankToFullyQualifiedLinks(true)
)

// aboutPage splits about.md into its optional YAML frontmatter summary and its body rendered to sanitized HTML, without H1s.
// Relative links and images resolve against the file's own URL, so the spec's root-relative /img/... point at the site.
func aboutPage(md []byte, fileURL string) (summary, html string) {
	md = bytes.TrimPrefix(md, []byte("\ufeff"))
	if m := frontmatter.FindSubmatch(md); m != nil {
		var meta struct {
			Summary string `yaml:"summary"`
		}
		if yaml.Unmarshal(m[1], &meta) == nil {
			summary = strings.TrimSpace(meta.Summary)
		}
		md = md[len(m[0]):]
	}
	base, err := url.Parse(fileURL)
	if err != nil {
		return summary, ""
	}
	doc := markdown.Parser().Parse(goldtext.NewReader(md))
	var titles []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			if n.Level == 1 {
				titles = append(titles, n)
			}
		case *ast.Link:
			n.Destination = resolve(base, n.Destination)
		case *ast.Image:
			n.Destination = resolve(base, n.Destination)
		}
		return ast.WalkContinue, nil
	})
	// An H1 titles the page, which the profile header already does with the author's name.
	for _, n := range titles {
		n.Parent().RemoveChild(n.Parent(), n)
	}
	var out bytes.Buffer
	if err := markdown.Renderer().Render(&out, md, doc); err != nil {
		return summary, ""
	}
	html = aboutPolicy.Sanitize(out.String())
	if strings.TrimSpace(html) == "" {
		html = ""
	}
	return summary, html
}

func resolve(base *url.URL, dest []byte) []byte {
	u, err := base.Parse(string(dest))
	if err != nil {
		return dest
	}
	return []byte(u.String())
}

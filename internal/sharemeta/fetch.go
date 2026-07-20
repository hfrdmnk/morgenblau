package sharemeta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"morgenblau/internal/safehttp"
	"morgenblau/internal/standardfeed"
)

const (
	maxHTMLBytes  = 256 << 10
	maxTitleRunes = 512
)

type Target struct {
	ItemURL  string
	Document string
}

type Metadata struct {
	Title     string
	TargetURL string
	EntrySlug string
}

type StandardfeedClient interface {
	GetDocument(ctx context.Context, rawURI string) (*standardfeed.Document, error)
	GetPublication(ctx context.Context, rawURI string) (*standardfeed.Publication, error)
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Fetcher struct {
	standard StandardfeedClient
	http     HTTPDoer
}

func NewFetcher(standard StandardfeedClient, httpClient HTTPDoer) *Fetcher {
	return &Fetcher{standard: standard, http: httpClient}
}

func (f *Fetcher) Fetch(ctx context.Context, target Target) (Metadata, error) {
	document := strings.TrimSpace(target.Document)
	if document == "" && strings.HasPrefix(strings.TrimSpace(target.ItemURL), "at://") {
		document = strings.TrimSpace(target.ItemURL)
	}
	if document != "" {
		return f.fetchDocument(ctx, document)
	}
	return f.fetchWebPage(ctx, strings.TrimSpace(target.ItemURL))
}

func (f *Fetcher) fetchDocument(ctx context.Context, document string) (Metadata, error) {
	if f.standard == nil {
		return Metadata{}, errors.New("sharemeta: no Standardfeed client")
	}
	doc, err := f.standard.GetDocument(ctx, document)
	if err != nil {
		return Metadata{}, err
	}
	title := normalizeTitle(doc.Title)
	if title == "" {
		return Metadata{}, errors.New("sharemeta: document has no title")
	}
	if doc.Path == "" {
		return Metadata{Title: title}, nil
	}

	base := strings.TrimSpace(doc.Site)
	if strings.HasPrefix(base, "at://") {
		pub, err := f.standard.GetPublication(ctx, base)
		if err != nil {
			return Metadata{}, err
		}
		base = pub.URL
	}
	return Metadata{Title: title, TargetURL: documentURL(base, doc.Path)}, nil
}

func (f *Fetcher) fetchWebPage(ctx context.Context, rawURL string) (Metadata, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Metadata{}, fmt.Errorf("sharemeta: invalid item URL %q", rawURL)
	}
	if f.http == nil {
		return Metadata{}, errors.New("sharemeta: no HTTP client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.http.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, fmt.Errorf("sharemeta: upstream returned %d", resp.StatusCode)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "html") && !strings.Contains(contentType, "xhtml") {
		return Metadata{}, fmt.Errorf("sharemeta: unsupported content type %q", contentType)
	}

	limited := io.LimitReader(resp.Body, maxHTMLBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Metadata{}, err
	}
	if len(body) > maxHTMLBytes {
		return Metadata{}, errors.New("sharemeta: HTML exceeds size limit")
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return Metadata{}, err
	}
	title := extractTitle(doc)
	if title == "" {
		return Metadata{}, errors.New("sharemeta: page has no title")
	}
	targetURL := parsed.String()
	if resp.Request != nil && resp.Request.URL != nil {
		targetURL = resp.Request.URL.String()
	}
	return Metadata{Title: title, TargetURL: targetURL}, nil
}

func extractTitle(doc *html.Node) string {
	var openGraph, twitter, document string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(node.Data) {
			case "meta":
				property := attribute(node, "property")
				name := attribute(node, "name")
				content := attribute(node, "content")
				switch {
				case openGraph == "" && strings.EqualFold(property, "og:title"):
					openGraph = content
				case twitter == "" && strings.EqualFold(name, "twitter:title"):
					twitter = content
				}
			case "title":
				if document == "" && node.FirstChild != nil {
					document = node.FirstChild.Data
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	for _, candidate := range []string{openGraph, twitter, document} {
		if title := normalizeTitle(candidate); title != "" {
			return title
		}
	}
	return ""
}

func attribute(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func normalizeTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	runes := []rune(title)
	if len(runes) > maxTitleRunes {
		runes = runes[:maxTitleRunes]
	}
	return string(runes)
}

func documentURL(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path = strings.TrimLeft(strings.TrimSpace(path), "/")
	if base == "" || path == "" {
		return ""
	}
	return base + "/" + path
}

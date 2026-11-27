package favicon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	maxHTMLBytes = 256 << 10
	maxIconBytes = 256 << 10
	PoliteUA     = "Mozilla/5.0 (compatible; Morgenblau/0.1; +https://app.morgen.blue/about)"
)

func Discover(ctx context.Context, client HTTPDoer, siteURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("favicon: invalid siteURL %q", siteURL)
	}

	candidates, finalURL := collectCandidates(ctx, client, base)
	fallbackBase := base
	if finalURL != nil {
		fallbackBase = finalURL
	}
	fallback := *fallbackBase
	fallback.Path = "/favicon.ico"
	fallback.RawQuery = ""
	fallback.Fragment = ""
	candidates = append(candidates, candidate{href: fallback.String(), rel: "icon"})

	rank(candidates)
	var lastErr error
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if seen[c.href] {
			continue
		}
		seen[c.href] = true
		if err := validate(ctx, client, c.href); err != nil {
			lastErr = err
			continue
		}
		return c.href, nil
	}
	if lastErr == nil {
		lastErr = errors.New("favicon: no candidates")
	}
	return "", fmt.Errorf("favicon: discovery failed for %s: %w", base.String(), lastErr)
}

type candidate struct {
	href     string
	rel      string
	mimeType string
	size     int
}

func collectCandidates(ctx context.Context, client HTTPDoer, site *url.URL) ([]candidate, *url.URL) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, site.String(), nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("User-Agent", PoliteUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, finalURL
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "xml") {
		return nil, finalURL
	}

	doc, err := html.Parse(io.LimitReader(resp.Body, maxHTMLBytes))
	if err != nil {
		return nil, finalURL
	}
	return extract(doc, finalURL), finalURL
}

func extract(n *html.Node, base *url.URL) []candidate {
	var out []candidate
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			if c, ok := candidateFromLink(n, base); ok {
				out = append(out, c)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func candidateFromLink(n *html.Node, base *url.URL) (candidate, bool) {
	var rel, href, mimeType, sizes string
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "rel":
			rel = a.Val
		case "href":
			href = a.Val
		case "type":
			mimeType = a.Val
		case "sizes":
			sizes = a.Val
		}
	}
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(strings.ToLower(href), "data:") {
		return candidate{}, false
	}
	matched := normalizedRel(rel)
	if matched == "" {
		return candidate{}, false
	}
	ref, err := url.Parse(href)
	if err != nil {
		return candidate{}, false
	}
	abs := base.ResolveReference(ref).String()
	return candidate{
		href:     abs,
		rel:      matched,
		mimeType: strings.ToLower(strings.TrimSpace(mimeType)),
		size:     parseSizes(sizes),
	}, true
}

func normalizedRel(rel string) string {
	for tok := range strings.FieldsSeq(strings.ToLower(rel)) {
		switch tok {
		case "icon":
			return "icon"
		case "shortcut":
			continue
		case "apple-touch-icon":
			return "apple-touch-icon"
		case "apple-touch-icon-precomposed":
			return "apple-touch-icon-precomposed"
		}
	}
	return ""
}

func parseSizes(s string) int {
	if s == "" {
		return 0
	}
	best := 0
	for tok := range strings.FieldsSeq(strings.ToLower(s)) {
		if tok == "any" {
			continue
		}
		parts := strings.SplitN(tok, "x", 2)
		if len(parts) != 2 {
			continue
		}
		w, err1 := strconv.Atoi(parts[0])
		h, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}
		dim := max(w, h)
		if dim > best {
			best = dim
		}
	}
	return best
}

func rank(cs []candidate) {
	relWeight := func(r string) int {
		switch r {
		case "apple-touch-icon-precomposed":
			return 40
		case "apple-touch-icon":
			return 30
		case "icon":
			return 20
		default:
			return 0
		}
	}
	isSVG := func(c candidate) bool {
		return c.mimeType == "image/svg+xml" || strings.HasSuffix(strings.ToLower(c.href), ".svg")
	}
	sort.SliceStable(cs, func(i, j int) bool { return less(cs[i], cs[j], isSVG, relWeight) })
}

func less(a, b candidate, isSVG func(candidate) bool, weight func(string) int) bool {
	aSVG, bSVG := isSVG(a), isSVG(b)
	if aSVG != bSVG {
		return aSVG
	}
	if a.size != b.size {
		return a.size > b.size
	}
	return weight(a.rel) > weight(b.rel)
}

func validate(ctx context.Context, client HTTPDoer, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", PoliteUA)
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxIconBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("favicon: %s returned %d", rawURL, resp.StatusCode)
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	if !strings.HasPrefix(ct, "image/") && ct != "application/octet-stream" {
		return fmt.Errorf("favicon: %s wrong content-type %q", rawURL, ct)
	}
	return nil
}

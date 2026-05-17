// Package favicon resolves a website's best favicon URL by parsing its HTML
// <head> for <link rel=icon|...> hints, with a /favicon.ico fallback. It only
// returns URLs — it does not download or cache image bytes. The returned URL
// has been validated to return a 2xx image response at discovery time.
package favicon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// HTTPDoer is the minimal slice of *http.Client this package needs. Tests
// inject a stub; production passes a configured *http.Client.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	maxHTMLBytes = 256 << 10
	maxIconBytes = 256 << 10
	// PoliteUA uses Mozilla compatibility on purpose: a number of CDNs (notably
	// Cloudflare) gate HTML and image responses on the absence of "Go-http-client"
	// in the UA, while keeping /feed/ paths open. Favicon discovery loads a page
	// the same way a browser would, so it advertises itself as browser-compatible
	// while still naming the bot and a contact URL.
	PoliteUA = "Mozilla/5.0 (compatible; Morgenblau/0.1; +https://morgen.blue/about)"
)

// Discover walks the site at siteURL and returns the best favicon URL it can
// validate. Returns an error if no candidate (including the /favicon.ico
// fallback) returns a 2xx image response.
func Discover(ctx context.Context, client HTTPDoer, siteURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("favicon: invalid siteURL %q", siteURL)
	}

	candidates, finalURL := collectCandidates(ctx, client, base)
	// The fallback origin is the final URL after redirects when available,
	// otherwise the original site URL.
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
	size     int // largest dimension parsed from sizes attribute; 0 = unknown
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

// extract walks the parsed tree for <link> elements whose rel mentions any
// icon variant, resolving hrefs against base.
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

// normalizedRel returns the recognized icon rel token (canonical form) or "".
// rel is a space-separated token list; matching is case-insensitive.
//
// `mask-icon` is intentionally NOT recognized: Safari pinned-tab icons are
// monochrome silhouettes designed to be tinted by the browser via the link's
// `color=` attribute. Rendered as-is in an <img>, they collapse to solid black
// (e.g. theverge.com, mitchellh.com). Matches Miniflux's rel allow-list.
func normalizedRel(rel string) string {
	// Order here doesn't determine ranking — that's in rank(). We just need to
	// recognize that at least one token is an icon variant.
	for tok := range strings.FieldsSeq(strings.ToLower(rel)) {
		switch tok {
		case "icon":
			return "icon"
		case "shortcut":
			// "shortcut icon" appears as two tokens; the icon token catches it.
			continue
		case "apple-touch-icon":
			return "apple-touch-icon"
		case "apple-touch-icon-precomposed":
			return "apple-touch-icon-precomposed"
		}
	}
	return ""
}

// parseSizes extracts the largest dimension from a sizes attribute like
// "32x32" or "192x192 16x16" or "any". Returns 0 if unknown.
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

// rank sorts candidates best-first using: SVG → larger size → rel preference.
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
	// insertion sort — n is tiny in practice
	for i := 1; i < len(cs); i++ {
		j := i
		for j > 0 && less(cs[j], cs[j-1], isSVG, relWeight) {
			cs[j-1], cs[j] = cs[j], cs[j-1]
			j--
		}
	}
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

// validate confirms the URL returns a 2xx response with an image-shaped
// Content-Type (or application/octet-stream, which some CDNs return for .ico).
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
	// Drain capped body so the connection can be reused.
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

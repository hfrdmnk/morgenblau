package webfeed

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"morgenblau/internal/safehttp"
)

// Doer is the HTTP client the pipeline fetches through; the server passes a safehttp client.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	defaultMaxBody        = 5 << 20
	maxArticleConcurrency = 4
)

// response is one fetched URL, cached for the length of a setup so no page is fetched twice.
type response struct {
	url    string
	status int
	ctype  string
	body   []byte
	err    error
}

type session struct {
	client    Doer
	userAgent string
	maxBody   int64
	mu        sync.Mutex
	lang      string
	cache     map[string]*response
}

func newSession(opts Options) *session {
	s := &session{client: opts.Client, userAgent: opts.UserAgent, maxBody: opts.MaxBody, cache: map[string]*response{}}
	if s.userAgent == "" {
		s.userAgent = safehttp.UserAgent
	}
	if s.maxBody <= 0 {
		s.maxBody = defaultMaxBody
	}
	return s
}

func (s *session) setLang(lang string) {
	s.mu.Lock()
	s.lang = lang
	s.mu.Unlock()
}

func (s *session) get(ctx context.Context, rawURL string) *response {
	s.mu.Lock()
	if r, ok := s.cache[rawURL]; ok {
		s.mu.Unlock()
		return r
	}
	lang := s.lang
	s.mu.Unlock()
	r := s.fetch(ctx, rawURL, lang)
	s.mu.Lock()
	s.cache[rawURL] = r
	s.mu.Unlock()
	return r
}

func (s *session) fetch(ctx context.Context, rawURL, lang string) *response {
	r := &response{url: rawURL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		r.err = err
		return r
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		r.err = err
		return r
	}
	defer resp.Body.Close()
	r.status = resp.StatusCode
	r.ctype = resp.Header.Get("Content-Type")
	if resp.Request != nil && resp.Request.URL != nil {
		r.url = resp.Request.URL.String()
	}
	r.body, r.err = io.ReadAll(io.LimitReader(resp.Body, s.maxBody))
	return r
}

// challengeTitles are bot-wall page titles; matched on the title only, since the words appear in ordinary pages' scripts.
var challengeTitles = []string{
	"just a moment", "attention required", "access denied", "verify you are human", "are you a robot", "security check",
	"pardon our interruption", "please wait", "ddos protection", "bot verification", "checking your browser", "request blocked",
	"403 forbidden", "human verification", "captcha",
}

// challengeMarkers are vendor challenge fingerprints in the first 3k bytes.
var challengeMarkers = []string{
	"/cdn-cgi/challenge-platform", "cf-browser-verification", "_cf_chl_opt", "cf_chl_", "geo.captcha-delivery.com",
	"px-captcha", "_incapsula_resource", "please enable js and disable any ad blocker", "awswaf", "challenge-form",
}

func isChallenge(body []byte) bool {
	head := body[:min(len(body), 3000)]
	lower := strings.ToLower(string(head))
	for _, m := range challengeMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	title := ""
	if i := strings.Index(lower, "<title"); i >= 0 {
		if j := strings.Index(lower[i:], ">"); j >= 0 {
			rest := lower[i+j+1:]
			if k := strings.Index(rest, "</title>"); k >= 0 {
				title = collapse(rest[:k])
			}
		}
	}
	for _, t := range challengeTitles {
		if strings.Contains(title, t) {
			return true
		}
	}
	return false
}

// blocked reports a listing fetch the site refused us; other errors are not a block.
func (r *response) blocked() bool {
	if r.err != nil {
		return true
	}
	switch r.status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable, 999:
		return true
	}
	return r.status < 400 && isChallenge(r.body)
}

func (r *response) isHTML() bool {
	ct := strings.ToLower(r.ctype)
	if strings.Contains(ct, "html") {
		return true
	}
	if ct != "" && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "octet-stream") {
		return false
	}
	head := bytes.ToLower(r.body[:min(len(r.body), 1024)])
	return bytes.Contains(head, []byte("<html")) || bytes.Contains(head, []byte("<!doctype html"))
}

// robots holds the rules of the group that applies to us plus the Sitemap lines.
type robots struct {
	rules    []robotsRule
	sitemaps []string
}

type robotsRule struct {
	allow   bool
	pattern string
}

const robotsToken = "morgenblau"

func parseRobots(txt string) robots {
	type group struct {
		agents []string
		rules  []robotsRule
	}
	var groups []*group
	var cur *group
	lastWasAgent := false
	var out robots
	for line := range strings.SplitSeq(txt, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(v))
			lastWasAgent = true
		case "allow", "disallow":
			lastWasAgent = false
			if cur != nil && (k == "allow" || v != "") {
				cur.rules = append(cur.rules, robotsRule{allow: k == "allow", pattern: v})
			}
		case "sitemap":
			out.sitemaps = append(out.sitemaps, v)
		default:
			lastWasAgent = false
		}
	}
	var star *group
	for _, g := range groups {
		for _, a := range g.agents {
			if a != "*" && a != "" && strings.Contains(robotsToken, a) {
				out.rules = g.rules
				return out
			}
			if a == "*" && star == nil {
				star = g
			}
		}
	}
	if star != nil {
		out.rules = star.rules
	}
	return out
}

// allowed applies RFC 9309: the longest matching rule wins and Allow wins ties.
func (r robots) allowed(path string) bool {
	best, allow := -1, true
	for _, rule := range r.rules {
		if rule.pattern == "" {
			continue
		}
		if robotsMatch(rule.pattern, path) {
			if l := len(rule.pattern); l > best || (l == best && rule.allow) {
				best, allow = l, rule.allow
			}
		}
	}
	return allow
}

func robotsMatch(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if i == 0 {
			if !strings.HasPrefix(path, part) {
				return false
			}
			pos = len(part)
			continue
		}
		j := strings.Index(path[pos:], part)
		if j < 0 {
			return false
		}
		pos += j + len(part)
	}
	if anchored {
		last := parts[len(parts)-1]
		return strings.HasSuffix(path, last) && (len(parts) > 1 || pos == len(path))
	}
	return true
}

// robotsFor fetches robots.txt; an unreachable or failing file allows the fetch, since a reader asked for this one page.
func (s *session) robotsFor(ctx context.Context, origin string) robots {
	r := s.get(ctx, origin+"/robots.txt")
	if r.err != nil || r.status != http.StatusOK || r.isHTML() {
		return robots{}
	}
	return parseRobots(string(r.body))
}

type sitemapURL struct {
	loc     string
	lastmod time.Time
}

type sitemap struct {
	urls     []sitemapURL
	children []string
}

func parseSitemap(body []byte) (sitemap, error) {
	var doc struct {
		XMLName xml.Name
		URLs    []struct {
			Loc     string `xml:"loc"`
			Lastmod string `xml:"lastmod"`
		} `xml:"url"`
		Sitemaps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return sitemap{}, fmt.Errorf("sitemap: %w", err)
	}
	var sm sitemap
	switch doc.XMLName.Local {
	case "urlset":
		for _, u := range doc.URLs {
			su := sitemapURL{loc: strings.TrimSpace(u.Loc)}
			if t, ok := parseDateAttr(u.Lastmod); ok {
				su.lastmod = t
			}
			sm.urls = append(sm.urls, su)
		}
	case "sitemapindex":
		for _, c := range doc.Sitemaps {
			sm.children = append(sm.children, strings.TrimSpace(c.Loc))
		}
	default:
		return sitemap{}, errors.New("sitemap: not a urlset or sitemapindex")
	}
	return sm, nil
}

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

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	maxBody               = 5 << 20
	maxArticleBody        = 512 << 10
	maxRobotsBody         = 512 << 10
	defaultMaxSetupBytes  = 24 << 20
	maxArticleConcurrency = 4
	challengeMarkerBytes  = 3000
)

type response struct {
	url    string
	status int
	header http.Header
	body   []byte
	err    error
}

type session struct {
	client    Doer
	mu        sync.Mutex
	lang      string
	cache     map[string]*response
	articles  map[string]*articleInfo
	robots    map[string]robots
	bytesLeft int64
}

var ErrUnreachable = errors.New("webfeed: site unreachable")

var errBudget = errors.New("webfeed: setup byte budget spent")

func newSession(opts Options) *session {
	s := &session{client: opts.Client, bytesLeft: opts.MaxSetupBytes, cache: map[string]*response{}, articles: map[string]*articleInfo{}, robots: map[string]robots{}}
	if s.bytesLeft <= 0 {
		s.bytesLeft = defaultMaxSetupBytes
	}
	return s
}

func (s *session) setLang(lang string) {
	s.mu.Lock()
	s.lang = safehttp.LanguageTag(lang)
	s.mu.Unlock()
}

func cacheKey(lang, rawURL string) string { return lang + " " + rawURL }

func (s *session) get(ctx context.Context, rawURL string) *response {
	s.mu.Lock()
	lang := s.lang
	if r, ok := s.cache[cacheKey(lang, rawURL)]; ok {
		s.mu.Unlock()
		return r
	}
	s.mu.Unlock()
	r := s.fetch(ctx, rawURL, lang, maxBody, nil)
	s.mu.Lock()
	s.cache[cacheKey(lang, rawURL)] = r
	s.mu.Unlock()
	return r
}

func (s *session) fetchOnce(ctx context.Context, rawURL string, limit int64) *response {
	s.mu.Lock()
	lang := s.lang
	s.mu.Unlock()
	return s.fetch(ctx, rawURL, lang, limit, nil)
}

func (s *session) fetch(ctx context.Context, rawURL, lang string, limit int64, header http.Header) *response {
	r := &response{url: rawURL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		r.err = err
		return r
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if lang := safehttp.LanguageTag(lang); lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	if s.remaining() <= 0 {
		r.err = errBudget
		return r
	}
	resp, err := s.client.Do(req)
	if err != nil {
		r.err = err
		return r
	}
	defer resp.Body.Close()
	r.status, r.header = resp.StatusCode, resp.Header
	if resp.Request != nil && resp.Request.URL != nil {
		r.url = resp.Request.URL.String()
	}
	r.body, r.err = s.readBody(resp.Body, limit)
	return r
}

func (s *session) readBody(body io.Reader, limit int64) ([]byte, error) {
	s.mu.Lock()
	n := min(limit, s.bytesLeft)
	s.bytesLeft -= n
	s.mu.Unlock()
	if n <= 0 {
		return nil, errBudget
	}
	b, err := io.ReadAll(io.LimitReader(body, n))
	s.mu.Lock()
	s.bytesLeft += n - int64(len(b))
	s.mu.Unlock()
	return b, err
}

func (s *session) remaining() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytesLeft
}

var challengeTitles = []string{
	"just a moment", "attention required", "access denied", "verify you are human", "are you a robot", "security check",
	"pardon our interruption", "please wait", "ddos protection", "bot verification", "checking your browser", "request blocked",
	"403 forbidden", "human verification", "captcha",
}

var challengeMarkers = []string{
	"/cdn-cgi/challenge-platform", "cf-browser-verification", "_cf_chl_opt", "cf_chl_", "geo.captcha-delivery.com",
	"px-captcha", "_incapsula_resource", "please enable js and disable any ad blocker", "awswaf", "challenge-form",
}

func isChallenge(body []byte) bool {
	head := body[:min(len(body), challengeMarkerBytes)]
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

func (r *response) refusal() (reason Reason, blocked bool) {
	switch {
	case r.err != nil:
		return ReasonUnreachable, false
	case r.status >= 400:
		switch r.status {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, 999:
			return ReasonHTTPStatus, true
		}
		return ReasonHTTPStatus, false
	case isChallenge(r.body):
		return ReasonChallenge, true
	}
	return "", false
}

func (r *response) unreachable() bool { return r.err != nil || r.status >= 500 }

func (r *response) isHTML() bool {
	ct := strings.ToLower(r.header.Get("Content-Type"))
	if strings.Contains(ct, "html") {
		return true
	}
	if ct != "" && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "octet-stream") {
		return false
	}
	head := bytes.ToLower(r.body[:min(len(r.body), 1024)])
	return bytes.Contains(head, []byte("<html")) || bytes.Contains(head, []byte("<!doctype html"))
}

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

// RFC 9309: the most specific match, the longest rule, wins, and Allow wins a tie.
// https://www.rfc-editor.org/rfc/rfc9309#section-2.2.2
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

func (s *session) robotsFor(ctx context.Context, origin string) robots {
	s.mu.Lock()
	rb, ok := s.robots[origin]
	s.mu.Unlock()
	if ok {
		return rb
	}
	if r := s.fetchOnce(ctx, origin+"/robots.txt", maxRobotsBody); r.err == nil && r.status == http.StatusOK && !r.isHTML() {
		rb = parseRobots(string(r.body))
	}
	s.mu.Lock()
	s.robots[origin] = rb
	s.mu.Unlock()
	return rb
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
			if URLTemplate(su.loc) == "" {
				continue
			}
			if t, ok := parseDateAttr(u.Lastmod); ok {
				su.lastmod = t
			}
			sm.urls = append(sm.urls, su)
		}
	case "sitemapindex":
		for _, c := range doc.Sitemaps {
			if loc := strings.TrimSpace(c.Loc); URLTemplate(loc) != "" {
				sm.children = append(sm.children, loc)
			}
		}
	default:
		return sitemap{}, errors.New("sitemap: not a urlset or sitemapindex")
	}
	return sm, nil
}

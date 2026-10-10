package webfeed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"morgenblau/internal/postkey"
)

// PollInput is what one poll needs from earlier ones: the recipe, the conditional-GET validators, the link-set hash and the good-poll history, newest first.
type PollInput struct {
	Recipe       Recipe
	ETag         string
	LastModified string
	LinkSetHash  string
	History      []GoodPoll
	// Known are the posts already in the feed with their stored dates, zero when they have none; they are never refetched for a date.
	Known []Item
	// Unconditional fetches and extracts even an unchanged page, for the weekly check.
	Unconditional bool
}

// PollResult is one deterministic poll: the detector's verdict and the full, uncapped extraction.
type PollResult struct {
	Verdict PollVerdict
	Reason  Reason
	// Blocked marks a broken poll the site refused, rather than one where the site broke.
	Blocked bool
	// NotModified means the page or its link set did not change, so nothing was extracted.
	NotModified  bool
	Items        []Item
	Excerpts     map[string]string
	ETag         string
	LastModified string
	LinkSetHash  string
	URLTemplate  string

	page *fetchedPage
}

// fetchedPage keeps the polled listing so repair and the weekly check work on what the detector saw, without a second
// fetch or parse; doc is nil for a sitemap.
type fetchedPage struct {
	body []byte
	doc  *pageDoc
}

// Poll fetches the listing once and runs the detector; it never calls a model.
func Poll(ctx context.Context, in PollInput, opts Options) (*PollResult, error) {
	if opts.Client == nil {
		return nil, errors.New("webfeed: Options.Client is required")
	}
	r := in.Recipe
	target, wantHTML := r.ListingURL, true
	switch r.Mode {
	case ModeSitemap:
		target, wantHTML = r.FeedURL, false
	case ModeStreams, ModeCSS:
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedMode, r.Mode)
	}
	s := newSession(opts)
	if !s.robotsAllow(ctx, target, opts.Robots) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return broken(&PollResult{}, ReasonRobots, true), nil
	}
	resp := s.pollFetch(ctx, target, r.Lang, in)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := &PollResult{ETag: resp.header.Get("ETag"), LastModified: resp.header.Get("Last-Modified")}
	if resp.err == nil && resp.status == http.StatusNotModified {
		out.Verdict, out.NotModified = PollOK, true
		out.ETag, out.LastModified, out.LinkSetHash = in.ETag, in.LastModified, in.LinkSetHash
		return out, nil
	}
	if reason, blocked := responseTier0(resp, wantHTML); reason != "" {
		return broken(out, reason, blocked), nil
	}

	var p *pageDoc
	var items []Item
	if r.Mode == ModeSitemap {
		var err error
		if items, err = applySitemap(r, resp.body); err != nil {
			return broken(out, ReasonNotHTML, false), nil
		}
		out.LinkSetHash = hashKeys(itemURLKeys(items))
	} else {
		var err error
		if p, err = parsePage(resp.body, resp.url); err != nil {
			return broken(out, ReasonNotHTML, false), nil
		}
		if reason := pageTier0(p); reason != "" {
			return broken(out, reason, false), nil
		}
		out.LinkSetHash = linkSetHash(p)
	}
	if !in.Unconditional && in.LinkSetHash != "" && out.LinkSetHash == in.LinkSetHash {
		out.Verdict, out.NotModified = PollOK, true
		return out, nil
	}

	if p != nil {
		var err error
		if items, err = applyPage(r, p); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			if reason := emptyTier0(p, resp.body); reason != "" {
				return broken(out, reason, false), nil
			}
		}
	}
	templates := streamTemplates(r.Streams)
	out.Items = items
	out.URLTemplate = DominantTemplate(items, templates)
	out.page = &fetchedPage{body: resp.body, doc: p}
	if reason := drift(items, out.URLTemplate, in.History); reason != "" {
		out.Verdict, out.Reason = PollSuspect, reason
		return out, nil
	}
	s.datePosts(ctx, r, items, in.Known)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sortItems(items, r.Order)
	out.Verdict = PollOK
	if p != nil {
		out.Excerpts = listingExcerpts(p, items)
	}
	return out, nil
}

// datePosts dates undated posts as setup did: a known post keeps its stored date, and a new one, when the recipe dates
// posts from their own pages, gets the date its page states.
func (s *session) datePosts(ctx context.Context, r Recipe, items, known []Item) {
	stored := make(map[string]time.Time, len(known))
	for _, k := range known {
		stored[postkey.URL(k.URL)] = k.Date
	}
	var fetch []string
	for i := range items {
		if !items[i].Date.IsZero() {
			continue
		}
		if d, ok := stored[postkey.URL(items[i].URL)]; ok {
			items[i].Date = d
			continue
		}
		if r.DateSource == DateSourceArticle && len(fetch) < calibrateItems {
			fetch = append(fetch, items[i].URL)
		}
	}
	if len(fetch) == 0 {
		return
	}
	s.setLang(r.Lang)
	infos := s.fetchArticles(ctx, fetch)
	if !trustArticleDates(infos) {
		return
	}
	for i := range items {
		if info := infos[items[i].URL]; info != nil && items[i].Date.IsZero() {
			items[i].Date = info.published
		}
	}
}

// Page is the listing this poll extracted from, for the weekly check; nil when the poll stopped before extraction.
func (r *PollResult) Page() []byte {
	if r.page == nil {
		return nil
	}
	return r.page.body
}

func broken(out *PollResult, reason Reason, blocked bool) *PollResult {
	out.Verdict, out.Reason, out.Blocked = PollBroken, reason, blocked
	return out
}

// pollFetch is one GET, conditional unless the poll must see the page.
func (s *session) pollFetch(ctx context.Context, rawURL, lang string, in PollInput) *response {
	h := http.Header{}
	if !in.Unconditional {
		if in.ETag != "" {
			h.Set("If-None-Match", in.ETag)
		}
		if in.LastModified != "" {
			h.Set("If-Modified-Since", in.LastModified)
		}
	}
	return s.fetch(ctx, rawURL, lang, maxBody, h)
}

// linkSetHash fingerprints the page's links, which stay put when only markup, scripts or timestamps change.
func linkSetHash(p *pageDoc) string {
	keys := make([]string, 0, len(p.anchors))
	for _, a := range p.anchors {
		keys = append(keys, a.key)
	}
	return hashKeys(keys)
}

func itemURLKeys(items []Item) []string {
	urls := make([]string, len(items))
	for i, it := range items {
		urls[i] = it.URL
	}
	return urlKeys(urls)
}

func hashKeys(keys []string) string {
	sum := sha256.Sum256([]byte(strings.Join(keySet(keys), "\n")))
	return hex.EncodeToString(sum[:16])
}

// robotsAllow reports whether robots.txt lets us fetch rawURL, through cache when one is given.
func (s *session) robotsAllow(ctx context.Context, rawURL string, cache *RobotsCache) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return true
	}
	origin := u.Scheme + "://" + u.Host
	var rb robots
	if cache != nil {
		rb = cache.get(ctx, s, origin)
	} else {
		rb = s.robotsFor(ctx, origin)
	}
	return rb.allowed(u.RequestURI())
}

const robotsTTL = 24 * time.Hour

// RobotsCache holds each origin's robots.txt rules for a day; it is safe for concurrent use.
type RobotsCache struct {
	mu      sync.Mutex
	entries map[string]robotsEntry
	now     func() time.Time
}

type robotsEntry struct {
	rules   robots
	fetched time.Time
}

func NewRobotsCache() *RobotsCache {
	return &RobotsCache{entries: map[string]robotsEntry{}, now: time.Now}
}

func (c *RobotsCache) get(ctx context.Context, s *session, origin string) robots {
	now := c.now()
	c.mu.Lock()
	e, ok := c.entries[origin]
	c.mu.Unlock()
	if ok && now.Sub(e.fetched) < robotsTTL {
		return e.rules
	}
	rules := s.robotsFor(ctx, origin)
	if ctx.Err() != nil {
		return rules
	}
	c.mu.Lock()
	c.entries[origin] = robotsEntry{rules: rules, fetched: now}
	// Expired entries go on write, so the map stays as small as the set of sites still polled.
	for k, old := range c.entries {
		if now.Sub(old.fetched) >= robotsTTL {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
	return rules
}

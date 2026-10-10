package feedfinder

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

const (
	// probeConcurrency keeps one paste from hammering a single site.
	probeConcurrency = 4
	// probeBudget caps the whole probe phase because a paste is interactive.
	probeBudget = 10 * time.Second
	// maxHintProbes bounds page-supplied URLs, since a page can mention "feed" in hundreds of links.
	maxHintProbes = 12
)

var (
	listingFeedPaths  = []string{"/rss.xml", "/feed", "/feed.xml", "/atom.xml", "/index.xml", "/rss"}
	rootOnlyFeedPaths = []string{"/feed/", "/blog/rss.xml", "/news/rss.xml"}
)

// feedWord needs rss, atom or feed as a whole word so "feedback" and "anatomy" never trigger a probe.
var feedWord = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:rss|atom|feeds?)(?:[^a-z]|$)`)

// languageTag rejects lang values that are not a BCP 47 shape, which would otherwise ride into a request header.
var languageTag = regexp.MustCompile(`^[A-Za-z]{1,8}(?:-[A-Za-z0-9]{1,8})*$`)

type probeSource int

// Order matters: on an overlap tie a feed the page itself points at beats a guessed path.
const (
	fromPage probeSource = iota
	fromPath
)

type probe struct {
	url    string
	source probeSource
}

type probedFeed struct {
	probe
	finalURL    string
	contentType string
	title       string
	itemKeys    map[string]struct{}
}

// discoverFeed finds a feed the page does not advertise with rel=alternate, or returns nil.
func (f *Finder) discoverFeed(ctx context.Context, doc *goquery.Document, base *url.URL, listingReadable bool) *Candidate {
	ctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()

	lang := strings.TrimSpace(doc.Find("html").First().AttrOr("lang", ""))
	if !languageTag.MatchString(lang) {
		lang = ""
	}
	feeds := f.probeAll(ctx, feedProbes(doc, base), lang)
	if len(feeds) == 0 {
		return nil
	}

	listing := listingLinkKeys(doc, base)
	overlap := func(p *probedFeed) int {
		n := 0
		for k := range p.itemKeys {
			if _, ok := listing[k]; ok {
				n++
			}
		}
		return n
	}
	best := slices.MinFunc(feeds, func(a, b *probedFeed) int {
		return cmp.Or(
			cmp.Compare(overlap(b), overlap(a)),
			cmp.Compare(a.source, b.source),
			cmp.Compare(len(a.finalURL), len(b.finalURL)),
			strings.Compare(a.finalURL, b.finalURL),
		)
	})
	// A guessed path whose posts never appear on a readable listing is another stream, such as a site-wide press feed.
	if listingReadable && best.source == fromPath && len(best.itemKeys) > 0 && overlap(best) == 0 {
		return nil
	}
	return &Candidate{
		FeedURL:     best.finalURL,
		Title:       best.title,
		ContentType: best.contentType,
		SiteURL:     base.String(),
	}
}

// feedProbes lists page hints before guessed paths so dedupe keeps the stronger source.
func feedProbes(doc *goquery.Document, base *url.URL) []probe {
	var out []probe
	seen := map[string]struct{}{}
	add := func(href string, source probeSource) {
		abs, err := base.Parse(strings.TrimSpace(href))
		if err != nil || (abs.Scheme != "http" && abs.Scheme != "https") || abs.Host == "" {
			return
		}
		abs.Fragment = ""
		key := abs.String()
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, probe{url: key, source: source})
	}

	for _, href := range feedHints(doc) {
		if len(out) == maxHintProbes {
			break
		}
		add(href, fromPage)
	}
	origin := &url.URL{Scheme: base.Scheme, Host: base.Host}
	if dir := strings.TrimSuffix(base.Path, "/"); dir != "" {
		for _, p := range listingFeedPaths {
			add(origin.String()+dir+p, fromPath)
		}
	}
	for _, p := range slices.Concat(listingFeedPaths, rootOnlyFeedPaths) {
		add(origin.String()+p, fromPath)
	}
	return out
}

// feedHints collects hrefs the page labels as feeds: meta rss-feed, then link elements, then anchors.
func feedHints(doc *goquery.Document) []string {
	var out []string
	doc.Find("meta[name][content]").Each(func(_ int, s *goquery.Selection) {
		if strings.EqualFold(strings.TrimSpace(s.AttrOr("name", "")), "rss-feed") {
			out = append(out, s.AttrOr("content", ""))
		}
	})
	doc.Find("link[href]").Each(func(_ int, s *goquery.Selection) {
		rel := strings.Fields(strings.ToLower(s.AttrOr("rel", "")))
		href := s.AttrOr("href", "")
		switch {
		case slices.Contains(rel, "feed"):
			out = append(out, href)
		case slices.ContainsFunc(rel, func(r string) bool { return r == "alternate" || strings.Contains(r, "icon") }) &&
			feedWord.MatchString(href+" "+s.AttrOr("title", "")):
			out = append(out, href)
		}
	})
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href := s.AttrOr("href", "")
		if feedWord.MatchString(href) || feedWord.MatchString(anchorLabel(s)) {
			out = append(out, href)
		}
	})
	return out
}

// anchorLabel joins an anchor's text with the attributes icon-only links use to name themselves.
func anchorLabel(a *goquery.Selection) string {
	parts := []string{a.Text(), a.AttrOr("title", ""), a.AttrOr("aria-label", ""), a.AttrOr("class", "")}
	a.Find("[alt], [title], [aria-label], [class], img[src]").Each(func(_ int, s *goquery.Selection) {
		for _, attr := range []string{"alt", "title", "aria-label", "class", "src"} {
			parts = append(parts, s.AttrOr(attr, ""))
		}
	})
	return strings.Join(parts, " ")
}

// probeAll returns the probes that parsed as feeds, one per final URL after redirects.
func (f *Finder) probeAll(ctx context.Context, probes []probe, lang string) []*probedFeed {
	results := make([]*probedFeed, len(probes))
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = f.probeFeed(ctx, p, lang)
		})
	}
	wg.Wait()

	var out []*probedFeed
	seen := map[string]struct{}{}
	for _, r := range results {
		if r == nil {
			continue
		}
		if _, dup := seen[r.finalURL]; dup {
			continue
		}
		seen[r.finalURL] = struct{}{}
		out = append(out, r)
	}
	return out
}

// probeFeed trusts only a 2xx body that parses as a feed, because sites serve HTML shells under feed paths and content types.
func (f *Finder) probeFeed(ctx context.Context, p probe, lang string) *probedFeed {
	req, err := newGET(ctx, p.url)
	if err != nil {
		return nil
	}
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedSniffBytes))
	if err != nil {
		return nil
	}
	parsed, err := gofeed.NewParser().Parse(bytes.NewReader(buf))
	if err != nil || parsed == nil {
		return nil
	}

	final := req.URL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL
	}
	keys := make(map[string]struct{}, len(parsed.Items))
	for _, item := range parsed.Items {
		if item == nil {
			continue
		}
		if abs, err := final.Parse(strings.TrimSpace(item.Link)); err == nil && abs.Host != "" {
			keys[linkKey(abs)] = struct{}{}
		}
	}
	return &probedFeed{
		probe:       p,
		finalURL:    final.String(),
		contentType: strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]),
		title:       strings.TrimSpace(parsed.Title),
		itemKeys:    keys,
	}
}

func listingLinkKeys(doc *goquery.Document, base *url.URL) map[string]struct{} {
	keys := map[string]struct{}{}
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		if abs, err := base.Parse(strings.TrimSpace(s.AttrOr("href", ""))); err == nil && abs.Host != "" {
			keys[linkKey(abs)] = struct{}{}
		}
	})
	return keys
}

// linkKey ignores scheme, www., trailing slash and fragment, which feeds and listings disagree on for the same post.
func linkKey(u *url.URL) string {
	key := strings.TrimPrefix(strings.ToLower(u.Host), "www.") + strings.TrimSuffix(u.Path, "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

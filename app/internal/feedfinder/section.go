package feedfinder

import (
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"morgenblau/internal/postkey"
	"morgenblau/internal/sitesection"
)

// maxSectionPages bounds the extra page fetches a feedless page can cost.
const maxSectionPages = 2

// sectionFeeds runs page discovery on a homepage's writing sections, since a homepage often advertises no feed while its blog does.
// A pasted subpage is already the listing the reader wants, so its sibling sections would be other streams.
func (f *Finder) sectionFeeds(ctx context.Context, doc *goquery.Document, base *url.URL, probed map[string]*probedFeed) []Candidate {
	if strings.Trim(base.Path, "/") != "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, probeBudget)
	defer cancel()
	for _, section := range sectionLinks(doc, base) {
		sdoc, sbase := f.fetchSection(ctx, section)
		if sdoc == nil {
			continue
		}
		out := f.pageFeeds(ctx, sdoc, sbase, true, probed)
		if len(out) == 0 {
			continue
		}
		for i := range out {
			out[i].SiteURL = base.String()
		}
		return out
	}
	return nil
}

// sectionLinks returns up to maxSectionPages same-host links to a one-segment section path, in page order.
func sectionLinks(doc *goquery.Document, base *url.URL) []*url.URL {
	var out []*url.URL
	seen := map[string]struct{}{}
	doc.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		abs, err := base.Parse(strings.TrimSpace(s.AttrOr("href", "")))
		if err != nil || (abs.Scheme != "http" && abs.Scheme != "https") || !sitesection.SameSite(abs, base) {
			return true
		}
		if !sitesection.Name(strings.Trim(abs.Path, "/")) {
			return true
		}
		abs.RawQuery, abs.ForceQuery, abs.Fragment = "", false, ""
		key := postkey.FromURL(abs)
		if _, dup := seen[key]; dup {
			return true
		}
		seen[key] = struct{}{}
		out = append(out, abs)
		return len(out) < maxSectionPages
	})
	return out
}

// fetchSection returns the parsed section page and its final URL, or nil when it is no readable page.
func (f *Finder) fetchSection(ctx context.Context, section *url.URL) (*goquery.Document, *url.URL) {
	req, err := newGET(ctx, section.String())
	if err != nil {
		return nil, nil
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if _, isFeed := feedContentTypes[strings.ToLower(mediaType(resp))]; isFeed || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, maxFeedSniffBytes))
	if err != nil {
		return nil, nil
	}
	return doc, finalURL(resp, req.URL)
}

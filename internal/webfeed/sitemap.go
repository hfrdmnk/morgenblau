package webfeed

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"morgenblau/internal/postkey"
)

const maxSitemapFetches = 8

// sitemapSetup serves a listing that refuses our fetch from its sitemap, the only route that respects the block.
func (s *session) sitemapSetup(ctx context.Context, res *Result, listing *url.URL, rb robots) (*Result, error) {
	rec := &res.Recipe
	rec.Setup.Stages = append(rec.Setup.Stages, "blocked", "sitemap")
	queue := slices.Clone(rb.sitemaps)
	if len(queue) == 0 {
		queue = []string{listing.Scheme + "://" + listing.Host + "/sitemap.xml"}
	}
	listingPath := strings.TrimSuffix(listing.Path, "/")
	listingSeg := pathFirstSegment(listing.String())
	var found []sitemapURL
	var source string
	fetches := 0
	seen := map[string]bool{}
	for len(queue) > 0 && fetches < maxSitemapFetches {
		next := queue[0]
		queue = queue[1:]
		if seen[next] {
			continue
		}
		seen[next] = true
		fetches++
		r := s.fetchOnce(ctx, next, maxBody)
		if r.err != nil || r.status != http.StatusOK {
			continue
		}
		sm, err := parseSitemap(r.body)
		if err != nil {
			continue
		}
		// Children that name the listing's section or a post type go first; the rest wait.
		slices.SortStableFunc(sm.children, func(a, b string) int { return sitemapRank(b, listingSeg) - sitemapRank(a, listingSeg) })
		queue = append(queue, sm.children...)
		before := len(found)
		for _, su := range sm.urls {
			u, err := url.Parse(su.loc)
			if err != nil || !sameSite(u.Host, listing.Host) || postkey.URL(su.loc) == postkey.URL(listing.String()) {
				continue
			}
			if listingPath != "" {
				if !strings.HasPrefix(u.Path, listingPath+"/") {
					continue
				}
			} else if !postLikePath(su.loc) {
				continue
			}
			found = append(found, su)
		}
		if len(found) > before && source == "" {
			source = next
		}
	}
	tmpl := ""
	counts := map[string]int{}
	for _, su := range found {
		t := URLTemplate(su.loc)
		counts[t]++
		if counts[t] > counts[tmpl] {
			tmpl = t
		}
	}
	var items []Item
	withLastmod := 0
	for _, su := range found {
		if URLTemplate(su.loc) == tmpl {
			items = append(items, Item{URL: su.loc, Title: slugTitle(su.loc)})
			if !su.lastmod.IsZero() {
				withLastmod++
			}
		}
	}
	if len(items) == 0 {
		res.Blocked, res.Reasons = true, []Reason{ReasonBlocked}
		return res, nil
	}
	// lastmod records edits, not publishing, so it orders the list but never dates an item.
	if withLastmod*10 >= len(items)*7 {
		lastmod := map[string]int64{}
		for _, su := range found {
			lastmod[su.loc] = su.lastmod.Unix()
		}
		slices.SortStableFunc(items, func(a, b Item) int {
			switch {
			case lastmod[a.URL] > lastmod[b.URL]:
				return -1
			case lastmod[a.URL] < lastmod[b.URL]:
				return 1
			}
			return 0
		})
	}
	items = capItems(items)
	rec.Mode = ModeSitemap
	rec.FeedURL = source
	rec.Streams = []Stream{{URLTemplate: tmpl, TitleHints: TitleHints{Source: TitleSlug}}}
	rec.Order, rec.DateSource = OrderPage, DateSourceNone
	sig := ComputeSignals(items, []string{tmpl})
	rec.Fingerprint = fingerprintOf(sig, []string{tmpl})
	res.Items = items
	res.Reasons = sig.doubts()
	return res, nil
}

func sitemapRank(loc, listingSeg string) int {
	l := strings.ToLower(loc)
	switch {
	case listingSeg != "" && strings.Contains(l, strings.ToLower(listingSeg)):
		return 3
	case strings.Contains(l, "post") || strings.Contains(l, "blog") || strings.Contains(l, "news") || strings.Contains(l, "article"):
		return 2
	case strings.Contains(l, "image") || strings.Contains(l, "video") || strings.Contains(l, "product") || strings.Contains(l, "categor") || strings.Contains(l, "tag"):
		return 0
	}
	return 1
}

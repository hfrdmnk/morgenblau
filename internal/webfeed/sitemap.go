package webfeed

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"morgenblau/internal/postkey"
)

const maxSitemapFetches = 8

// sitemapSetup serves a listing that refuses our fetch from its sitemap, the only route that respects the block.
func (s *session) sitemapSetup(ctx context.Context, res *Result, listing *url.URL, rb robots) (*Result, error) {
	rec := &res.Recipe
	rec.Setup.Stages = append(rec.Setup.Stages, "blocked", "sitemap")
	roots := slices.Clone(rb.sitemaps)
	if len(roots) == 0 {
		roots = []string{listing.Scheme + "://" + listing.Host + "/sitemap.xml"}
	}
	listingPath := strings.TrimSuffix(listing.Path, "/")
	underListing := func(su sitemapURL) bool {
		u, err := url.Parse(su.loc)
		if err != nil || !sameSite(u.Host, listing.Host) || postkey.URL(su.loc) == postkey.URL(listing.String()) {
			return false
		}
		if listingPath != "" {
			return strings.HasPrefix(u.Path, listingPath+"/")
		}
		return postLikePath(su.loc)
	}
	walk := walkSitemaps(roots, listing.String(), func(rawURL string) *response { return s.fetchOnce(ctx, rawURL, maxBody) }, underListing)
	tmpl := ""
	counts := map[string]int{}
	for _, su := range walk.found {
		t := URLTemplate(su.loc)
		counts[t]++
		if counts[t] > counts[tmpl] {
			tmpl = t
		}
	}
	items := sitemapItems(walk.found, func(loc string) bool { return URLTemplate(loc) == tmpl })
	if len(items) == 0 {
		res.Blocked, res.Reasons = true, []Reason{ReasonBlocked}
		return res, nil
	}
	rec.Mode = ModeSitemap
	// Polls walk again from the root, since posts published later land in a child setup may not have read first.
	rec.FeedURL = walk.root
	rec.Streams = []Stream{{URLTemplate: tmpl, TitleHints: TitleHints{Source: TitleSlug}}}
	rec.Order, rec.DateSource = OrderPage, DateSourceNone
	sig := ComputeSignals(items, []string{tmpl})
	rec.Fingerprint = fingerprintOf(sig, []string{tmpl})
	res.Items = items
	res.Reasons = sig.doubts()
	return res, nil
}

// sitemapTree is what a walk found: the kept entries, the root whose files first yielded one, and that first file's body.
type sitemapTree struct {
	found  []sitemapURL
	root   string
	source []byte
	// rootErr is why the first root did not parse; rootIndex says it was an index, whose validators cannot vouch for its children.
	rootErr   error
	rootIndex bool
}

// walkSitemaps reads sitemaps breadth-first from roots, children that name the listing's section or a post type first,
// within maxSitemapFetches.
func walkSitemaps(roots []string, listing string, get func(string) *response, keep func(sitemapURL) bool) sitemapTree {
	type queued struct{ loc, root string }
	listingSeg := pathFirstSegment(listing)
	queue := make([]queued, len(roots))
	for i, r := range roots {
		queue[i] = queued{r, r}
	}
	var tree sitemapTree
	fetches := 0
	seen := map[string]bool{}
	for len(queue) > 0 && fetches < maxSitemapFetches {
		next := queue[0]
		queue = queue[1:]
		if seen[next.loc] {
			continue
		}
		seen[next.loc] = true
		r := get(next.loc)
		fetches++
		if r.err != nil || r.status != http.StatusOK {
			continue
		}
		sm, err := parseSitemap(r.body)
		if fetches == 1 {
			tree.rootErr, tree.rootIndex = err, len(sm.children) > 0
		}
		if err != nil {
			continue
		}
		slices.SortStableFunc(sm.children, func(a, b string) int { return sitemapRank(b, listingSeg) - sitemapRank(a, listingSeg) })
		for _, c := range sm.children {
			queue = append(queue, queued{c, next.root})
		}
		before := len(tree.found)
		for _, su := range sm.urls {
			if keep(su) {
				tree.found = append(tree.found, su)
			}
		}
		if len(tree.found) > before && tree.root == "" {
			tree.root, tree.source = next.root, r.body
		}
	}
	return tree
}

// sitemapItems is the one way setup and polls turn sitemap entries into items, so both serve the same capped list.
func sitemapItems(found []sitemapURL, match func(loc string) bool) []Item {
	type dated struct {
		item    Item
		lastmod time.Time
	}
	var list []dated
	withLastmod := 0
	seen := map[string]bool{}
	for _, su := range found {
		k := postkey.URL(su.loc)
		if seen[k] || !match(su.loc) {
			continue
		}
		seen[k] = true
		list = append(list, dated{Item{URL: su.loc, Title: slugTitle(su.loc)}, su.lastmod})
		if !su.lastmod.IsZero() {
			withLastmod++
		}
	}
	// lastmod records edits, not publishing, so it orders the list but never dates an item.
	if withLastmod*10 >= len(list)*7 {
		slices.SortStableFunc(list, func(a, b dated) int { return b.lastmod.Compare(a.lastmod) })
	}
	items := make([]Item, len(list))
	for i, d := range list {
		items[i] = d.item
	}
	return capItems(items)
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

package webfeed

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"

	"morgenblau/internal/postkey"
)

// errUnsupportedMode is a recipe mode apply cannot run on a fetched page.
var errUnsupportedMode = errors.New("webfeed: recipe mode not supported by Apply")

// apply runs a stored recipe on a freshly fetched page (the listing, or the sitemap in mode sitemap) without any model or extra fetch.
func apply(r Recipe, body []byte, pageURL string) ([]Item, error) {
	if r.Mode == ModeSitemap {
		return applySitemap(r, body)
	}
	p, err := parsePage(body, pageURL)
	if err != nil {
		return nil, err
	}
	return applyPage(r, p)
}

// applyPage runs a listing recipe on a page already parsed.
func applyPage(r Recipe, p *pageDoc) ([]Item, error) {
	switch r.Mode {
	case ModeStreams:
		return applyStreams(r, p), nil
	case ModeCSS:
		return applyCSSRecipe(r, p)
	}
	return nil, fmt.Errorf("%w: %s", errUnsupportedMode, r.Mode)
}

func applyStreams(r Recipe, p *pageDoc) []Item {
	clusters := p.clusters()
	var entries []*entry
	seen := map[string]bool{}
	for _, st := range r.Streams {
		for _, c := range refind(clusters, st) {
			for _, e := range c.entries {
				k := postkey.URL(e.URL)
				if seen[k] || !matchTemplate(st.URLTemplate, URLTemplate(e.URL)) || slices.Contains(r.ExcludeURLs, e.URL) {
					continue
				}
				seen[k] = true
				if st.TitleHints.Source == TitleCalibrated {
					applyTitleHint(e, st.TitleHints.Tag)
				}
				entries = append(entries, e)
			}
		}
	}
	slices.SortStableFunc(entries, func(a, b *entry) int { return p.order[a.node] - p.order[b.node] })
	items := make([]Item, len(entries))
	for i, e := range entries {
		items[i] = e.item()
	}
	sortItems(items, r.Order)
	return capItems(items)
}

// capItems keeps the first MaxItems, the newest in date order.
func capItems(items []Item) []Item { return items[:min(len(items), MaxItems)] }

// refind locates a stream: the exact container hint, then the same structure under rehashed classes, then any list of its URL shape.
func refind(clusters []*cluster, st Stream) []*cluster {
	shapeShare := func(c *cluster) float64 {
		n := 0
		for _, e := range c.entries {
			if matchTemplate(st.URLTemplate, URLTemplate(e.URL)) {
				n++
			}
		}
		return float64(n) / float64(max(1, len(c.entries)))
	}
	var exact, skeleton, shape []*cluster
	skeletons := make([]string, len(st.ContainerHints))
	for i, h := range st.ContainerHints {
		skeletons[i] = hintSkeleton(h)
	}
	for _, c := range clusters {
		share := shapeShare(c)
		if share == 0 {
			continue
		}
		switch {
		case slices.Contains(st.ContainerHints, c.hint):
			exact = append(exact, c)
		case slices.Contains(skeletons, hintSkeleton(c.hint)):
			skeleton = append(skeleton, c)
		}
		if share >= 0.5 && !c.sideList() {
			shape = append(shape, c)
		}
	}
	switch {
	case len(exact) > 0:
		return exact
	case len(skeleton) > 0:
		return skeleton
	}
	return shape
}

// sideList reports navigation, sidebar and "popular"-style lists, which share the stream's URL shape without being it.
func (c *cluster) sideList() bool {
	nav, aside := 0, 0
	for _, e := range c.entries {
		if e.inNav {
			nav++
		}
		if e.inAside {
			aside++
		}
	}
	n := max(1, len(c.entries))
	return nav*2 >= n || aside*2 >= n || isDecoyLabel(sectionLabel(c.container))
}

func applyTitleHint(e *entry, sig string) {
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && nodeSig(n, true) == sig {
			if t := cleanTitle(nodeText(n)); t != "" && !isGeneric(t) {
				e.Title, e.TitleSource = t, TitleCalibrated
				return true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(e.node)
}

func applySitemap(r Recipe, body []byte) ([]Item, error) {
	sm, err := parseSitemap(body)
	if err != nil {
		return nil, err
	}
	var items []Item
	seen := map[string]bool{}
	for _, su := range sm.urls {
		k := postkey.URL(su.loc)
		if seen[k] || slices.Contains(r.ExcludeURLs, su.loc) {
			continue
		}
		for _, st := range r.Streams {
			if matchTemplate(st.URLTemplate, URLTemplate(su.loc)) && !strings.EqualFold(k, postkey.URL(r.ListingURL)) {
				seen[k] = true
				items = append(items, Item{URL: su.loc, Title: slugTitle(su.loc)})
				break
			}
		}
	}
	return capItems(items), nil
}

func applyCSSRecipe(r Recipe, p *pageDoc) ([]Item, error) {
	if r.CSS == nil {
		return nil, errors.New("webfeed: css recipe without selectors")
	}
	items, _, err := applyCSS(p, *r.CSS)
	if err != nil {
		return nil, err
	}
	items = slices.DeleteFunc(items, func(it Item) bool { return slices.Contains(r.ExcludeURLs, it.URL) })
	sortItems(items, r.Order)
	return capItems(items), nil
}

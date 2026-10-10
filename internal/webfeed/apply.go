package webfeed

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// ErrUnsupportedMode is returned by Apply for modes it cannot run on a fetched page.
var ErrUnsupportedMode = errors.New("webfeed: recipe mode not supported by Apply")

// Apply runs a stored recipe on a freshly fetched page (the listing, or the sitemap in mode sitemap) without any model or extra fetch.
func Apply(r Recipe, body []byte, pageURL string) ([]Item, error) {
	switch r.Mode {
	case ModeStreams:
		return applyStreams(r, body, pageURL)
	case ModeSitemap:
		return applySitemap(r, body)
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupportedMode, r.Mode)
}

func applyStreams(r Recipe, body []byte, pageURL string) ([]Item, error) {
	p, err := parsePage(body, pageURL)
	if err != nil {
		return nil, err
	}
	clusters := p.clusters()
	var entries []*entry
	seen := map[string]bool{}
	for _, st := range r.Streams {
		for _, c := range refind(clusters, st) {
			for _, e := range c.entries {
				k := urlKey(e.URL)
				if seen[k] || !matchTemplate(st.URLTemplate, URLTemplate(e.URL)) || slices.Contains(r.ExcludeURLs, e.URL) {
					continue
				}
				seen[k] = true
				if st.TitleHints.Source == "calibrated" {
					applyTitleHint(e, st.TitleHints.Tag)
				}
				entries = append(entries, e)
			}
		}
	}
	slices.SortStableFunc(entries, func(a, b *entry) int { return p.order[a.node] - p.order[b.node] })
	items := make([]Item, len(entries))
	for i, e := range entries {
		items[i] = Item{URL: e.URL, Title: e.Title, Date: e.Date}
	}
	sortItems(items, r.Order)
	return items, nil
}

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
				e.Title, e.TitleSource = t, "calibrated"
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
		k := urlKey(su.loc)
		if seen[k] || slices.Contains(r.ExcludeURLs, su.loc) {
			continue
		}
		for _, st := range r.Streams {
			if matchTemplate(st.URLTemplate, URLTemplate(su.loc)) && !strings.EqualFold(k, urlKey(r.ListingURL)) {
				seen[k] = true
				items = append(items, Item{URL: su.loc, Title: slugTitle(su.loc)})
				break
			}
		}
	}
	return items, nil
}

package webfeed

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"morgenblau/internal/postkey"
	"morgenblau/internal/sitesection"
)

const sectionMinItems = 5

func (s *session) preferSection(ctx context.Context, home *Result) (*Result, error) {
	if home.listing == nil {
		return home, nil
	}
	target := sectionCandidate(home.listing, home.Items, !home.Healthy() || len(home.Items) < sectionMinItems)
	if target == "" {
		return home, nil
	}
	sec, err := s.setup(ctx, target, "", nil)
	if err != nil {
		return home, ctx.Err()
	}
	if !sectionBetter(home, sec) {
		return home, nil
	}
	rec := &sec.Recipe.Setup
	rec.Stages = append(append(slices.Clone(home.Recipe.Setup.Stages), "section"), rec.Stages...)
	rec.Section = &SectionRecord{From: home.Recipe.ListingURL, Judge: home.Recipe.Setup.Judge}
	if home.Title != "" {
		sec.Title = home.Title
	}
	return sec, nil
}

func sectionBetter(home, sec *Result) bool {
	if sec.listing == nil || !sec.Healthy() {
		return false
	}
	if !home.Healthy() {
		return len(sec.Items) > len(home.Items)
	}
	have := map[string]bool{}
	for _, it := range sec.Items {
		have[postkey.URL(it.URL)] = true
	}
	return !slices.ContainsFunc(home.Items, func(it Item) bool { return !have[postkey.URL(it.URL)] })
}

func sectionCandidate(p *pageDoc, items []Item, weak bool) string {
	listing := strings.Trim(p.url.Path, "/")
	if readerChoseADeeperPage := strings.Contains(listing, "/"); readerChoseADeeperPage {
		return ""
	}
	var order []string
	links := map[string]string{}
	for _, a := range p.anchors {
		u, err := url.Parse(a.url)
		if err != nil || !sitesection.SameSite(u, p.url) {
			continue
		}
		seg := strings.Trim(u.Path, "/")
		if seg == "" || strings.Contains(seg, "/") || seg == listing {
			continue
		}
		if _, ok := links[seg]; !ok {
			u.RawQuery = ""
			links[seg] = u.String()
			order = append(order, seg)
		}
	}
	if seg := teaserSection(p.url, items); seg != "" && links[seg] != "" {
		return links[seg]
	}
	if isFrontPage := listing == "" || defaultDocuments[listing]; weak && isFrontPage {
		for _, seg := range order {
			if sitesection.Name(seg) {
				return links[seg]
			}
		}
	}
	return ""
}

func teaserSection(listing *url.URL, items []Item) string {
	section := ""
	for _, it := range items {
		u, err := url.Parse(it.URL)
		if err != nil || !sitesection.SameSite(u, listing) {
			continue
		}
		first, rest, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
		isYearArchive := strings.Trim(first, "0123456789") == ""
		if rest == "" || isYearArchive || (section != "" && first != section) {
			return ""
		}
		section = first
	}
	return section
}

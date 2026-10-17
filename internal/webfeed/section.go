package webfeed

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"morgenblau/internal/postkey"
)

// sectionWords name the one-segment pages where sites keep their full list of posts; discovery reads them too.
var sectionWords = map[string]bool{
	"blog": true, "blogs": true, "writing": true, "writings": true, "posts": true, "post": true, "articles": true,
	"notes": true, "essays": true, "journal": true, "thoughts": true, "updates": true, "news": true, "changelog": true, "log": true,
}

// SectionName reports a one-segment path such as "blog" or "Writing" that sites use for their list of posts.
func SectionName(segment string) bool { return sectionWords[strings.ToLower(segment)] }

// sectionMinItems is how many posts a healthy listing shows before setup stops looking for a fuller section page.
const sectionMinItems = 5

// preferSection moves a teasing or weak listing's setup to the section page it links when that page serves its posts and
// more. The section is read without a model, so a move needs the heuristic's own confidence and no spend is discarded.
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

// sectionBetter accepts a section page that serves a healthy list: every post of a healthy listing, or more posts than a
// weak one. A refused section yields no page of its own, only robots or the sitemap, and is never a move.
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

// sectionCandidate is the one-segment section page on the listing's host to try instead: the section all its posts live
// under when the page links it, else for a weak front page the first linked section named like a blog; "" for none.
func sectionCandidate(p *pageDoc, items []Item, weak bool) string {
	listing := strings.Trim(p.url.Path, "/")
	// A page deeper in the site, such as a tag page, is a choice the reader made, not a teaser.
	if strings.Contains(listing, "/") {
		return ""
	}
	var order []string
	links := map[string]string{}
	for _, a := range p.anchors {
		u, err := url.Parse(a.url)
		if err != nil || !strings.EqualFold(u.Host, p.url.Host) {
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
	if seg := teaserSection(p.url.Host, items); seg != "" && links[seg] != "" {
		return links[seg]
	}
	// A named page the reader pasted, such as /updates, is a stream of its own; only the front page gives way to a sibling.
	if weak && (listing == "" || defaultDocuments[listing]) {
		for _, seg := range order {
			if SectionName(seg) {
				return links[seg]
			}
		}
	}
	return ""
}

// teaserSection is the first path segment every item on host lives under, with more path after it; "" when they differ.
func teaserSection(host string, items []Item) string {
	section := ""
	for _, it := range items {
		u, err := url.Parse(it.URL)
		if err != nil || !strings.EqualFold(u.Host, host) {
			continue
		}
		first, rest, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
		if rest == "" || (section != "" && first != section) {
			return ""
		}
		section = first
	}
	return section
}

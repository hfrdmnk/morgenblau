package webfeed

import (
	"strings"

	"morgenblau/internal/postkey"
)

const excerptSegments = 2

// listingExcerpts finds each item's teaser on the listing page it came from; items without one are absent.
func listingExcerpts(p *pageDoc, items []Item) map[string]string {
	want := make(map[string]Item, len(items))
	for _, it := range items {
		want[postkey.URL(it.URL)] = it
	}
	out := map[string]string{}
	for _, c := range p.clusters() {
		for _, e := range c.entries {
			it, ok := want[postkey.URL(e.URL)]
			if !ok || out[it.URL] != "" {
				continue
			}
			if ex := entryExcerpt(p, e, it.Title); ex != "" {
				out[it.URL] = ex
			}
		}
	}
	return out
}

// entryExcerpt is the entry's sentence-like text other than its title, date, labels and link furniture.
func entryExcerpt(p *pageDoc, e *entry, title string) string {
	var parts []string
	for _, s := range p.segments(e) {
		t := cleanTitle(s.text)
		if s.inOther || wordCount(t) < 5 || isDateLike(t) || isGeneric(t) || reReadingTime.MatchString(t) {
			continue
		}
		if strings.EqualFold(t, title) || strings.EqualFold(t, e.Title) ||
			(titleSimilarity(t, title) >= 0.8 && wordCount(t) <= wordCount(title)+2) {
			continue
		}
		parts = append(parts, t)
		if len(parts) == excerptSegments {
			break
		}
	}
	return strings.Join(parts, " ")
}

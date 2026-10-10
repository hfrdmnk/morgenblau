package webfeed

import "strings"

// Health is a generated feed's verdict, shown on the Sources page.
type Health string

const (
	HealthOK             Health = "ok"
	HealthNeedsAttention Health = "needs_attention"
	HealthBlockedBySite  Health = "blocked_by_site"
)

// Signals are the detector's measurements of one extraction.
type Signals struct {
	Count         int
	TitleShape    float64 // share of titles with 2–30 words, not all caps, not a bare number or date
	DupTitles     float64 // 1 − distinct titles / titles
	TemplateShare float64 // share of URLs matching one of the expected templates
	DateFrac      float64
}

// ComputeSignals measures items against the URL templates they are expected to match.
func ComputeSignals(items []Item, templates []string) Signals {
	s := Signals{Count: len(items)}
	if len(items) == 0 {
		return s
	}
	distinct := map[string]bool{}
	var shape, tmpl, dated float64
	for _, it := range items {
		if goodTitleShape(it.Title) {
			shape++
		}
		if matchAny(templates, URLTemplate(it.URL)) {
			tmpl++
		}
		if !it.Date.IsZero() {
			dated++
		}
		distinct[strings.ToLower(it.Title)] = true
	}
	n := float64(len(items))
	s.TitleShape, s.TemplateShare, s.DateFrac = shape/n, tmpl/n, dated/n
	s.DupTitles = 1 - float64(len(distinct))/n
	return s
}

// setupVerdict is the setup self-check; any reason means the reader should point at one post.
func (s Signals) setupVerdict() (Health, []string) {
	var reasons []string
	if s.Count < 3 {
		reasons = append(reasons, "count")
	}
	if s.Count > 0 && s.TitleShape < 0.7 {
		reasons = append(reasons, "title_shape")
	}
	if s.DupTitles > 0.5 {
		reasons = append(reasons, "dup_titles")
	}
	if s.Count > 0 && s.TemplateShare < 0.5 {
		reasons = append(reasons, "template_share")
	}
	if len(reasons) > 0 {
		return HealthNeedsAttention, reasons
	}
	return HealthOK, nil
}

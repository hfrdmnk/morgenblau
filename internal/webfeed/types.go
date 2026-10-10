package webfeed

import "time"

// MaxItems caps the posts one source yields, whatever the page or sitemap lists.
const MaxItems = 200

// Item is one post a recipe yields.
type Item struct {
	URL   string    `json:"url"`
	Title string    `json:"title"`
	Date  time.Time `json:"date"`
}

// Candidate is one scored list on the listing page, as the judge and the point-at-a-post flow see it.
type Candidate struct {
	Template      string  `json:"template"`
	ContainerHint string  `json:"container_hint"`
	Score         float64 `json:"score"`
	DateFrac      float64 `json:"date_frac"`
	InNav         bool    `json:"in_nav"`
	Entries       []Item  `json:"entries"`
}

func (c *cluster) public() Candidate {
	out := Candidate{Template: c.template, ContainerHint: c.hint, Score: c.score, DateFrac: c.dateFrac, InNav: c.navShare >= 0.5}
	for _, e := range c.entries {
		out.Entries = append(out.Entries, e.item())
	}
	return out
}

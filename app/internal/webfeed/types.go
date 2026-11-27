package webfeed

import "time"

const MaxItems = 200

type Item struct {
	URL   string    `json:"url"`
	Title string    `json:"title"`
	Date  time.Time `json:"date"`
}

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

package webfeed

import "encoding/json"

type Mode string

const (
	ModeStreams Mode = "streams"
	ModeCSS     Mode = "css"
	ModeSitemap Mode = "sitemap"
)

type Order string

const (
	OrderDate Order = "date"
	OrderPage Order = "page"
)

type DateSource string

const (
	DateSourceListing DateSource = "listing"
	DateSourceArticle DateSource = "article"
	DateSourceNone    DateSource = "none"
)

type Recipe struct {
	Version     int         `json:"version"`
	ListingURL  string      `json:"listing_url"`
	Mode        Mode        `json:"mode"`
	FeedURL     string      `json:"feed_url"`
	Streams     []Stream    `json:"streams"`
	CSS         *CSSRecipe  `json:"css"`
	ExcludeURLs []string    `json:"exclude_urls"`
	Lang        string      `json:"lang"`
	Order       Order       `json:"order"`
	DateSource  DateSource  `json:"date_source"`
	Fingerprint Fingerprint `json:"fingerprint"`
	Setup       SetupRecord `json:"setup"`
}

type Stream struct {
	URLTemplate    string     `json:"url_template"`
	ContainerHints []string   `json:"container_hints"`
	TitleHints     TitleHints `json:"title_hints"`
}

func (s Stream) MarshalJSON() ([]byte, error) {
	type plain Stream
	if s.ContainerHints == nil {
		s.ContainerHints = []string{}
	}
	return json.Marshal(plain(s))
}

type TitleSource string

const (
	TitleHeading    TitleSource = "heading"
	TitleClass      TitleSource = "class"
	TitleAnchor     TitleSource = "anchor"
	TitleAttr       TitleSource = "attr"
	TitleSlug       TitleSource = "slug"
	TitleArticle    TitleSource = "article"
	TitleCalibrated TitleSource = "calibrated"
)

type TitleHints struct {
	Source TitleSource `json:"source,omitempty"`
	Tag    string      `json:"tag,omitempty"`
}

type CSSRecipe struct {
	Item  string `json:"item"`
	Link  string `json:"link"`
	Title string `json:"title,omitempty"`
	Date  string `json:"date,omitempty"`
}

type Fingerprint struct {
	Count        int      `json:"count"`
	URLTemplates []string `json:"url_templates"`
	DateFrac     float64  `json:"date_frac"`
	TitleShape   float64  `json:"title_shape"`
}

type SetupRecord struct {
	Stages         []string       `json:"stages"`
	Confidence     float64        `json:"confidence"`
	Margin         float64        `json:"margin"`
	JudgeWanted    bool           `json:"judge_wanted,omitempty"`
	JudgeReasons   []string       `json:"judge_reasons,omitempty"`
	Judge          *JudgeRecord   `json:"judge,omitempty"`
	Induce         *InduceRecord  `json:"induce,omitempty"`
	Anchor         string         `json:"anchor,omitempty"`
	Section        *SectionRecord `json:"section,omitempty"`
	SectionChecked bool           `json:"section_checked,omitempty"`
}

type SectionRecord struct {
	From  string       `json:"from"`
	Judge *JudgeRecord `json:"judge,omitempty"`
}

type JudgeRecord struct {
	Pick         []int   `json:"pick"`
	Order        Order   `json:"order"`
	None         bool    `json:"none,omitempty"`
	Calls        int     `json:"calls,omitempty"`
	Failed       int     `json:"failed,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	USD          float64 `json:"usd,omitempty"`
}

func newRecipe(listingURL string, mode Mode) Recipe {
	return Recipe{
		Version:     1,
		ListingURL:  listingURL,
		Mode:        mode,
		Streams:     []Stream{},
		ExcludeURLs: []string{},
		Order:       OrderPage,
		DateSource:  DateSourceNone,
		Fingerprint: Fingerprint{URLTemplates: []string{}},
		Setup:       SetupRecord{Stages: []string{}},
	}
}

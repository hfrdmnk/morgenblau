package webfeed

import "encoding/json"

// Mode says how a recipe finds posts.
type Mode string

const (
	ModeStreams Mode = "streams"
	ModeCSS     Mode = "css"
	ModeSitemap Mode = "sitemap"
)

// Order is how a recipe's items are sorted.
type Order string

const (
	OrderDate Order = "date"
	OrderPage Order = "page"
)

// DateSource records where item dates come from.
type DateSource string

const (
	DateSourceListing DateSource = "listing"
	DateSourceArticle DateSource = "article"
	DateSourceNone    DateSource = "none"
)

// Recipe is the stored, versioned instruction for turning a listing page into items.
type Recipe struct {
	Version     int         `json:"version"`
	ListingURL  string      `json:"listing_url"`
	Mode        Mode        `json:"mode"`
	FeedURL     string      `json:"feed_url"` // the feed in mode feed; the sitemap to poll in mode sitemap
	Streams     []Stream    `json:"streams"`
	CSS         *CSSRecipe  `json:"css"`
	ExcludeURLs []string    `json:"exclude_urls"`
	Lang        string      `json:"lang"`
	Order       Order       `json:"order"`
	DateSource  DateSource  `json:"date_source"`
	Fingerprint Fingerprint `json:"fingerprint"`
	Setup       SetupRecord `json:"setup"`
}

// Stream is one block of posts, keyed on its URL shape; container hints are soft.
type Stream struct {
	URLTemplate    string     `json:"url_template"`
	ContainerHints []string   `json:"container_hints"`
	TitleHints     TitleHints `json:"title_hints"`
}

// MarshalJSON keeps container_hints an array, never null, so stored recipes have one shape.
func (s Stream) MarshalJSON() ([]byte, error) {
	type plain Stream
	if s.ContainerHints == nil {
		s.ContainerHints = []string{}
	}
	return json.Marshal(plain(s))
}

// TitleSource says where an entry's title came from.
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

// TitleHints records where setup found each entry's title.
type TitleHints struct {
	Source TitleSource `json:"source,omitempty"`
	Tag    string      `json:"tag,omitempty"`
}

// CSSRecipe holds induced selectors for mode css.
type CSSRecipe struct {
	Item  string `json:"item"`
	Link  string `json:"link"`
	Title string `json:"title,omitempty"`
	Date  string `json:"date,omitempty"`
}

// Fingerprint summarizes the setup result so polls can tell drift from news.
type Fingerprint struct {
	Count        int      `json:"count"`
	URLTemplates []string `json:"url_templates"`
	DateFrac     float64  `json:"date_frac"`
	TitleShape   float64  `json:"title_shape"`
}

// SetupRecord is the audit trail of how the recipe was chosen.
type SetupRecord struct {
	Stages       []string      `json:"stages"`
	Confidence   float64       `json:"confidence"`
	Margin       float64       `json:"margin"`
	JudgeWanted  bool          `json:"judge_wanted,omitempty"`
	JudgeReasons []string      `json:"judge_reasons,omitempty"`
	Judge        *JudgeRecord  `json:"judge,omitempty"`
	Induce       *InduceRecord `json:"induce,omitempty"`
	Anchor       string        `json:"anchor,omitempty"`
	// Section is set when setup moved from the pasted page to the section page that lists all its posts.
	Section *SectionRecord `json:"section,omitempty"`
	// SectionChecked marks a plain setup that looked for a section page; recipes from before that check lack it.
	SectionChecked bool `json:"section_checked,omitempty"`
}

// SectionRecord is the page setup moved away from, and the judge it paid for there.
type SectionRecord struct {
	From  string       `json:"from"`
	Judge *JudgeRecord `json:"judge,omitempty"`
}

// JudgeRecord is the aggregated judge verdict and its metered cost.
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

// newRecipe returns a version-1 recipe with every list initialized.
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

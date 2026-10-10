// Package websource owns server-generated feeds for sites without one: their storage, the RSS served at /web/<id>, and the poll schedule.
package websource

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Generator derives a feed from a listing page; internal/webfeed implements it behind an adapter wired in internal/server.
type Generator interface {
	// Setup reads the listing (anchored on one post when anchorURL is set) and returns the recipe and the posts found now.
	Setup(ctx context.Context, listingURL, anchorURL string) (Generated, error)
	// Poll re-reads the listing with the stored recipe and judges the result against prev.
	Poll(ctx context.Context, recipe json.RawMessage, prev PollState) (PollOutcome, error)
}

type Health string

const (
	HealthOK             Health = "ok"
	HealthNeedsAttention Health = "needs_attention"
	HealthBlockedBySite  Health = "blocked_by_site"
	HealthBroken         Health = "broken"
)

type Verdict string

const (
	VerdictOK      Verdict = "ok"
	VerdictSuspect Verdict = "suspect"
	VerdictBroken  Verdict = "broken"
)

// Item is one post on a listing page; a zero Date means the page shows none.
type Item struct {
	URL     string
	Title   string
	Date    time.Time
	Excerpt string
}

// Fingerprint is the part of a poll's shape only the generator can compute; the service derives count, URLs and titles from the items.
type Fingerprint struct {
	URLTemplate string
	DateFrac    float64
	TitleShape  float64
}

// Cost is the setup's spend, kept for the budget record.
type Cost struct {
	Stages       []string `json:"stages"`
	JudgeCalls   int      `json:"judgeCalls"`
	InputTokens  int      `json:"inputTokens"`
	OutputTokens int      `json:"outputTokens"`
	USD          float64  `json:"usd"`
}

type Generated struct {
	Recipe       json.RawMessage
	Title        string
	SiteURL      string
	Health       Health
	HealthReason string
	NeedsAnchor  bool
	Items        []Item
	Fingerprint  Fingerprint
	Cost         Cost
}

// PollRecord is one good poll in the detector's history.
type PollRecord struct {
	PolledAt time.Time
	Count    int
	URLs     []string
	Titles   []string
	Fingerprint
}

// PollState is what the generator needs from earlier polls: conditional-GET validators, the link-set hash and the good-poll history, newest first.
type PollState struct {
	ETag         string
	LastModified string
	LinkSetHash  string
	History      []PollRecord
}

// PollOutcome carries the full current extraction in Items; the service publishes only the ones it has not seen.
type PollOutcome struct {
	Verdict      Verdict
	Reason       string
	NotModified  bool
	Items        []Item
	Recipe       json.RawMessage
	ETag         string
	LastModified string
	LinkSetHash  string
	Fingerprint  Fingerprint
}

type Source struct {
	ID           string
	FeedURL      string
	ListingURL   string
	SiteURL      string
	Title        string
	Health       Health
	HealthReason string
	NeedsAnchor  bool
}

// FeedItem is an item as the feed serves it; Date is zero when the feed sends no pubDate.
type FeedItem struct {
	URL     string
	Title   string
	Date    time.Time
	Excerpt string
}

// Preview is what a reader sees before subscribing: the source and the first items of its feed.
type Preview struct {
	Source    Source
	Items     []FeedItem
	ItemCount int
}

// Limiter caps generations per reader; *ratelimit.Limiter satisfies it.
type Limiter interface {
	Allow(key string) (wait time.Duration, ok bool)
}

// RateLimitError matches ErrRateLimited and says when the reader may generate again.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string        { return "websource: too many generations" }
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

var (
	ErrRateLimited = errors.New("websource: too many generations")
	ErrInvalidURL  = errors.New("websource: invalid url")
	ErrGeneration  = errors.New("websource: generation failed")
	ErrUnavailable = errors.New("websource: feed generation is not available")
)

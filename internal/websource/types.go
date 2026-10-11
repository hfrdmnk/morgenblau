// Package websource owns server-generated feeds for sites without one: their storage, the RSS served at /web/<id>, and the poll schedule.
package websource

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Generator derives a feed from a listing page; WebfeedGenerator implements it over internal/webfeed.
type Generator interface {
	// Setup reads the listing (anchored on one post when anchorURL is set) and returns the recipe and the posts found now;
	// allowModel false keeps it to the free heuristic. A failed setup still returns the Cost of the model calls it made.
	Setup(ctx context.Context, listingURL, anchorURL string, allowModel bool) (Generated, error)
	// Poll re-reads the listing with the stored recipe, judges the result against prev and repairs a suspect one.
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
	Items        []Item
	Fingerprint  Fingerprint
	Cost         Cost
	// AnchorOnPage says an anchored setup found the reader's post linked on the listing it read.
	AnchorOnPage bool
	// Confirm asks the model once whether an anchored setup's items are the listing's posts; nil without a model.
	Confirm func(ctx context.Context) (Confirmation, error)
}

// Confirmation is the model's one look at an anchored list; USD is spent even when the call fails.
type Confirmation struct {
	Agreed bool
	Reason string
	USD    float64
}

// AnchorRejection says why a reader's post did not replace a source other readers follow.
type AnchorRejection string

const (
	AnchorNotOnPage    AnchorRejection = "not_on_page"
	AnchorNotConfirmed AnchorRejection = "not_confirmed"
	AnchorNoBetter     AnchorRejection = "no_better"
)

// PollRecord is one good poll in the detector's history.
type PollRecord struct {
	URLs   []string
	Titles []string
	Fingerprint
}

// PollState is what the generator needs from earlier polls: conditional-GET validators, the link-set hash and the
// good-poll history, newest first, plus the service's budget decisions for this poll.
type PollState struct {
	ETag         string
	LastModified string
	LinkSetHash  string
	History      []PollRecord
	// Known are the posts already stored, with their dates; a poll dates its posts from these before fetching any post page.
	Known []Item
	// AllowModelRepair is false while the site's one model repair a day is spent.
	AllowModelRepair bool
	// CheckDue asks for the weekly second opinion on this poll.
	CheckDue bool
	// WeeklyAlarm says a weekly check doubts the current list and nothing has answered it yet.
	WeeklyAlarm bool
}

// PollOutcome carries the full current extraction in Items; the service publishes only the ones it has not seen.
// Reason is shown to readers on the Sources page, so it is plain language.
type PollOutcome struct {
	Verdict Verdict
	Reason  string
	// Blocked marks a broken poll the site refused, which readers see as "This site blocks feed readers".
	Blocked      bool
	NotModified  bool
	Items        []Item
	Recipe       json.RawMessage
	ETag         string
	LastModified string
	LinkSetHash  string
	Fingerprint  Fingerprint
	// Repair is set when this poll ran the repair ladder; an accepted repair arrives as VerdictOK with its recipe and items.
	Repair *RepairReport
	// Checked says the weekly check ran on this poll; CheckUSD is what it cost.
	Checked  bool
	CheckUSD float64
}

type RepairCause string

const (
	RepairCauseDetector    RepairCause = "detector"
	RepairCauseWeeklyCheck RepairCause = "weekly_check"
)

type RepairOutcome string

const (
	RepairAccepted   RepairOutcome = "accepted"
	RepairFalseAlarm RepairOutcome = "false_alarm"
	RepairUnresolved RepairOutcome = "unresolved"
)

// RepairReport is one run of the repair ladder, kept for the daily model budget and the false-alarm record.
type RepairReport struct {
	Cause   RepairCause
	Outcome RepairOutcome
	// Step is the accepted rung, or the last one tried; Rule is the acceptance rule that held.
	Step      string
	Rule      string
	Reason    string
	UsedModel bool
	USD       float64
}

type Source struct {
	ID           string
	FeedURL      string
	ListingURL   string
	SiteURL      string
	Title        string
	Health       Health
	HealthReason string
	// NeedsAnchor says the setup in force doubted its list, so pointing at one post can settle it.
	NeedsAnchor bool
}

// Preview is what a reader sees before subscribing: the source and the first items of its feed.
type Preview struct {
	Source    Source
	Items     []Item
	ItemCount int
	// AnchorRejected is set when the reader's post left the source as it was.
	AnchorRejected AnchorRejection
}

// Limiter caps generations per reader; *ratelimit.Limiter satisfies it.
type Limiter interface {
	Allow(key string) (wait time.Duration, ok bool)
}

// RateLimitError says when the reader may generate again.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string { return "websource: too many generations" }

var (
	ErrInvalidURL = errors.New("websource: invalid url")
	ErrGeneration = errors.New("websource: generation failed")
	// ErrUnreachable is a generation that failed because the site did not answer, not because it refused us.
	ErrUnreachable = errors.New("websource: site unreachable")
	ErrUnavailable = errors.New("websource: feed generation is not available")
	// ErrBusy means every setup slot stayed taken until the setup deadline.
	ErrBusy = errors.New("websource: feed generation is busy")
	// ErrSetupInFlight means this reader's previous setup is still running.
	ErrSetupInFlight = errors.New("websource: a setup for this reader is still running")
)

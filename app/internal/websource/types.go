package websource

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Generator interface {
	Setup(ctx context.Context, listingURL, anchorURL string, allowModel bool) (Generated, error)
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

type Item struct {
	URL     string
	Title   string
	Date    time.Time
	Excerpt string
}

type Fingerprint struct {
	URLTemplate string
	DateFrac    float64
	TitleShape  float64
}

type Cost struct {
	Stages       []string `json:"stages"`
	JudgeCalls   int      `json:"judgeCalls"`
	InputTokens  int      `json:"inputTokens"`
	OutputTokens int      `json:"outputTokens"`
	USD          float64  `json:"usd"`
}

type Generated struct {
	ListingURL   string
	Recipe       json.RawMessage
	Title        string
	SiteURL      string
	Health       Health
	HealthReason string
	Items        []Item
	Fingerprint  Fingerprint
	Cost         Cost
	AnchorOnPage bool
	Confirm      func(ctx context.Context) (Confirmation, error)
}

type Confirmation struct {
	Agreed bool
	Reason string
	USD    float64
}

type AnchorRejection string

const (
	AnchorNotOnPage    AnchorRejection = "not_on_page"
	AnchorNotConfirmed AnchorRejection = "not_confirmed"
	AnchorNoBetter     AnchorRejection = "no_better"
)

type PollRecord struct {
	URLs   []string
	Titles []string
	Fingerprint
}

type PollState struct {
	ETag             string
	LastModified     string
	LinkSetHash      string
	History          []PollRecord
	Known            []Item
	AllowModelRepair bool
	CheckDue         bool
	WeeklyAlarm      bool
}

type PollOutcome struct {
	Verdict      Verdict
	Reason       string
	Blocked      bool
	NotModified  bool
	Items        []Item
	Recipe       json.RawMessage
	ETag         string
	LastModified string
	LinkSetHash  string
	Fingerprint  Fingerprint
	Repair       *RepairReport
	Checked      bool
	CheckUSD     float64
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

type RepairReport struct {
	Cause     RepairCause
	Outcome   RepairOutcome
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
	NeedsAnchor  bool
}

type Preview struct {
	Source         Source
	Items          []Item
	ItemCount      int
	AnchorRejected AnchorRejection
}

type Limiter interface {
	Allow(key string) (wait time.Duration, ok bool)
}

type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string { return "websource: too many generations" }

var (
	ErrInvalidURL    = errors.New("websource: invalid url")
	ErrGeneration    = errors.New("websource: generation failed")
	ErrBrowserBuilt  = errors.New("websource: listing is built in the browser")
	ErrUnreachable   = errors.New("websource: site unreachable")
	ErrUnavailable   = errors.New("websource: feed generation is not available")
	ErrBusy          = errors.New("websource: feed generation is busy")
	ErrSetupInFlight = errors.New("websource: a setup for this reader is still running")
)

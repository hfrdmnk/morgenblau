package webfeed

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
)

// Effort is how hard the model thinks on one call.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

// Prompt is one model call.
type Prompt struct {
	System string
	User   string
	Effort Effort
}

// Model answers a prompt with text; each step parses its own reply. Options.Model nil keeps every step heuristic.
type Model interface {
	Complete(ctx context.Context, p Prompt) (string, Usage, error)
}

// Usage is the metered cost of one call, reported even when its answer does not parse.
type Usage struct {
	InputTokens  int
	OutputTokens int
	USD          float64
}

func roundUSD(f float64) float64 { return math.Round(f*1e8) / 1e8 }

// decodeReply reads the JSON object in a model's reply, tolerating code fences and prose around it.
func decodeReply(text string, into any) error {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return errors.New("no JSON object in reply")
	}
	return json.Unmarshal([]byte(text[start:end+1]), into)
}

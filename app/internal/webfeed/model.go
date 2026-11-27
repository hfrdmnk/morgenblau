package webfeed

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
)

type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
)

type Prompt struct {
	System string
	User   string
	Effort Effort
}

type Model interface {
	Complete(ctx context.Context, p Prompt) (string, Usage, error)
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	USD          float64
}

func roundUSD(f float64) float64 { return math.Round(f*1e8) / 1e8 }

func decodeReply(text string, into any) error {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return errors.New("no JSON object in reply")
	}
	return json.Unmarshal([]byte(text[start:end+1]), into)
}

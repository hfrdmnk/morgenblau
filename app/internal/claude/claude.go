// Package claude answers prompts on Claude Haiku 5.5 through the Anthropic SDK, metering each call.
package claude

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// model is the one model every call runs on.
const model = "claude-haiku-5-5"

const (
	defaultCallTimeout = 60 * time.Second
	// maxTokens covers adaptive thinking plus a short JSON answer; the research saw about 600 output tokens at effort high.
	maxTokens  = 16_000
	maxRetries = 2
)

// Config builds a Client.
type Config struct {
	APIKey string
	// BaseURL replaces api.anthropic.com in tests.
	BaseURL string
	// CallTimeout bounds one call including the SDK's retries; zero means 60 s.
	CallTimeout time.Duration
}

// Client answers prompts on Haiku 5.5.
type Client struct {
	msgs    anthropic.MessageService
	timeout time.Duration
}

// Usage is one call's tokens and their price.
type Usage struct {
	InputTokens  int
	OutputTokens int
	USD          float64
}

// New builds a Client that ignores ANTHROPIC_* environment defaults, so neither an env base URL nor a stored profile can
// redirect the key. api.anthropic.com is a fixed, trusted host, so a timeout-bounded client needs no SSRF guard.
func New(cfg Config) (*Client, error) { return newClient(cfg, nil) }

// newClient takes the transport tests answer from; nil is the default network transport.
func newClient(cfg Config, transport http.RoundTripper) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("claude: API key is required")
	}
	timeout := cfg.CallTimeout
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(cfg.APIKey),
		option.WithHTTPClient(&http.Client{Timeout: timeout, Transport: transport}),
		option.WithMaxRetries(maxRetries),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	c := anthropic.NewClient(opts...)
	return &Client{msgs: c.Messages, timeout: timeout}, nil
}

var efforts = map[string]anthropic.OutputConfigEffort{
	"low":    anthropic.OutputConfigEffortLow,
	"medium": anthropic.OutputConfigEffortMedium,
	"high":   anthropic.OutputConfigEffortHigh,
}

// Complete makes one request at effort low, medium or high and returns its text. Haiku 5.5 rejects temperature and
// assistant prefill with a 400, so neither is sent.
func (c *Client) Complete(ctx context.Context, system, user, effort string) (string, Usage, error) {
	level, ok := efforts[effort]
	if !ok {
		return "", Usage{}, fmt.Errorf("claude: effort %q", effort)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	msg, err := c.msgs.New(ctx, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    maxTokens,
		System:       []anthropic.TextBlockParam{{Text: system}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
		Thinking:     anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.OutputConfigParam{Effort: level},
	})
	if err != nil {
		return "", Usage{}, fmt.Errorf("claude: %w", err)
	}
	in, out := int(msg.Usage.InputTokens), int(msg.Usage.OutputTokens)
	u := Usage{InputTokens: in, OutputTokens: out, USD: cost(in, out)}
	switch msg.StopReason {
	case anthropic.StopReasonRefusal:
		return "", u, fmt.Errorf("claude: refused (%s)", msg.StopDetails.Category)
	case anthropic.StopReasonMaxTokens:
		return "", u, errors.New("claude: reply cut at max_tokens")
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	if text.Len() == 0 {
		return "", u, errors.New("claude: reply has no text")
	}
	return text.String(), u, nil
}

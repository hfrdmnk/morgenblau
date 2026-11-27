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

const model = "claude-haiku-5-5"

const (
	defaultCallTimeout = 60 * time.Second
	maxTokens          = 16_000
	maxRetries         = 2
)

type Config struct {
	APIKey      string
	BaseURL     string
	CallTimeout time.Duration
}

type Client struct {
	msgs    anthropic.MessageService
	timeout time.Duration
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	USD          float64
}

func New(cfg Config) (*Client, error) { return newClient(cfg, nil) }

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

// Package claude runs webfeed's model steps (judge, recipe induction, weekly check) on Claude Haiku 5.5 through the Anthropic SDK.
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

	"morgenblau/internal/webfeed"
)

// Model is the one model every webfeed step runs on.
const Model = "claude-haiku-5-5"

const (
	defaultCallTimeout = 60 * time.Second
	// maxTokens covers adaptive thinking plus a short JSON answer; the research saw about 600 output tokens at effort high.
	maxTokens  = 16_000
	maxRetries = 2
)

// Config builds a Client.
type Config struct {
	APIKey string
	// HTTPClient carries every call. api.anthropic.com is a fixed, trusted host, so a plain timeout-bounded client is enough; no SSRF guard is needed.
	HTTPClient *http.Client
	// BaseURL replaces api.anthropic.com in tests.
	BaseURL string
	// CallTimeout bounds one call including the SDK's retries; zero means 60 s.
	CallTimeout time.Duration
}

// Client implements webfeed.Judge, webfeed.Inducer and webfeed.Watcher.
type Client struct {
	msgs    anthropic.MessageService
	timeout time.Duration
}

var (
	_ webfeed.Judge   = (*Client)(nil)
	_ webfeed.Inducer = (*Client)(nil)
	_ webfeed.Watcher = (*Client)(nil)
)

// New builds a Client that ignores ANTHROPIC_* environment defaults, so neither an env base URL nor a stored profile can redirect the key.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("claude: API key is required")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("claude: HTTP client is required")
	}
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(cfg.APIKey),
		option.WithHTTPClient(cfg.HTTPClient),
		option.WithMaxRetries(maxRetries),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	timeout := cfg.CallTimeout
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	c := anthropic.NewClient(opts...)
	return &Client{msgs: c.Messages, timeout: timeout}, nil
}

// Judge picks candidate lists at effort high, per the benchmarked judge contract.
func (c *Client) Judge(ctx context.Context, in webfeed.JudgeInput) (webfeed.Verdict, webfeed.Usage, error) {
	text, u, err := c.call(ctx, anthropic.OutputConfigEffortHigh, in.System, in.User)
	if err != nil {
		return webfeed.Verdict{}, u, err
	}
	v, err := webfeed.ParseVerdict(text, in.Candidates)
	return v, u, err
}

// Induce writes a CSS recipe at effort medium.
func (c *Client) Induce(ctx context.Context, in webfeed.InduceInput) (webfeed.CSSRecipe, webfeed.Usage, error) {
	text, u, err := c.call(ctx, anthropic.OutputConfigEffortMedium, in.System, in.User)
	if err != nil {
		return webfeed.CSSRecipe{}, u, err
	}
	r, err := webfeed.ParseCSSRecipe(text)
	return r, u, err
}

// Watch runs the weekly check at effort low.
func (c *Client) Watch(ctx context.Context, in webfeed.WatchInput) (webfeed.WatchVerdict, webfeed.Usage, error) {
	text, u, err := c.call(ctx, anthropic.OutputConfigEffortLow, in.System, in.User)
	if err != nil {
		return webfeed.WatchVerdict{}, u, err
	}
	v, err := webfeed.ParseWatchVerdict(text)
	return v, u, err
}

// call makes one request and returns its text; Haiku 5.5 rejects temperature and assistant prefill with a 400, so neither is sent.
func (c *Client) call(ctx context.Context, effort anthropic.OutputConfigEffort, system, user string) (string, webfeed.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	msg, err := c.msgs.New(ctx, anthropic.MessageNewParams{
		Model:        Model,
		MaxTokens:    maxTokens,
		System:       []anthropic.TextBlockParam{{Text: system}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
		Thinking:     anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.OutputConfigParam{Effort: effort},
	})
	if err != nil {
		return "", webfeed.Usage{}, fmt.Errorf("claude: %w", err)
	}
	in, out := int(msg.Usage.InputTokens), int(msg.Usage.OutputTokens)
	u := webfeed.Usage{InputTokens: in, OutputTokens: out, USD: Cost(in, out)}
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

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"morgenblau/internal/claude"
	"morgenblau/internal/webfeed"
	"morgenblau/internal/websource"
)

func publicBaseURL(getenv func(string) string, port int) (string, error) {
	if raw := strings.TrimSpace(getenv("PUBLIC_BASE_URL")); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
			strings.Trim(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf("PUBLIC_BASE_URL must be a bare http(s) origin, got %q", raw)
		}
		return u.Scheme + "://" + u.Host, nil
	}
	if getenv("APP_ENV") == "local" {
		return "http://127.0.0.1:" + strconv.Itoa(port), nil
	}
	u, err := url.Parse(strings.TrimSpace(getenv("BLUESKY_CLIENT_ID")))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errors.New("set PUBLIC_BASE_URL, or BLUESKY_CLIENT_ID to an HTTPS metadata URL, outside APP_ENV=local")
	}
	return "https://" + u.Host, nil
}

const modelCallTimeout = 25 * time.Second

func webModelDailyUSD(getenv func(string) string) (float64, error) {
	raw := strings.TrimSpace(getenv("WEB_MODEL_DAILY_USD"))
	if raw == "" {
		return 1, nil
	}
	usd, err := strconv.ParseFloat(raw, 64)
	if err != nil || usd < 0 || math.IsNaN(usd) || math.IsInf(usd, 0) {
		return 0, fmt.Errorf("WEB_MODEL_DAILY_USD must be a dollar amount of 0 or more, got %q", raw)
	}
	return usd, nil
}

func newWebGenerator(client webfeed.Doer, cfg claude.Config) (websource.Generator, bool, error) {
	opts := webfeed.Options{Client: client}
	if cfg.APIKey == "" {
		return websource.NewWebfeedGenerator(opts), false, nil
	}
	cfg.CallTimeout = modelCallTimeout
	c, err := claude.New(cfg)
	if err != nil {
		return nil, false, err
	}
	opts.Model = claudeModel{c}
	return websource.NewWebfeedGenerator(opts), true, nil
}

type claudeModel struct{ c *claude.Client }

func (m claudeModel) Complete(ctx context.Context, p webfeed.Prompt) (string, webfeed.Usage, error) {
	text, u, err := m.c.Complete(ctx, p.System, p.User, string(p.Effort))
	return text, webfeed.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, USD: u.USD}, err
}

type webPoller interface {
	PollDue(ctx context.Context) (int, error)
}

func runWebPoll(ctx context.Context, p webPoller, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := p.PollDue(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("web source poll", "err", err)
				continue
			}
			if n > 0 {
				slog.Info("web source poll", "sources", n)
			}
		}
	}
}

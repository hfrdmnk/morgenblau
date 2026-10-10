package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"morgenblau/internal/claude"
	"morgenblau/internal/webfeed"
	"morgenblau/internal/websource"
)

// publicBaseURL is the origin readers and feed readers reach this server on; generated feed URLs hang off it.
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

// isOwnWebFeed reports whether a feed URL is one of this server's generated /web/ feeds.
func isOwnWebFeed(base string) func(rawURL string) bool {
	origin, err := url.Parse(base)
	return func(rawURL string) bool {
		u, uerr := url.Parse(rawURL)
		return err == nil && uerr == nil && origin.Host != "" &&
			strings.EqualFold(u.Scheme, origin.Scheme) && strings.EqualFold(u.Host, origin.Host) && strings.HasPrefix(u.Path, "/web/")
	}
}

// ownWebFeeds answers this server's own /web/ feeds in process: the safe transport refuses loopback, and in production the request would hairpin out and back.
func ownWebFeeds(base string, feeds http.Handler, next http.RoundTripper) http.RoundTripper {
	own := isOwnWebFeed(base)
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !own(req.URL.String()) {
			return next.RoundTrip(req)
		}
		rec := httptest.NewRecorder()
		feeds.ServeHTTP(rec, req)
		resp := rec.Result()
		resp.Request = req
		return resp, nil
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// modelTimeout bounds one model call; api.anthropic.com is a fixed host, so the client needs no SSRF guard.
const modelTimeout = 90 * time.Second

// newWebGenerator builds feed generation over client, which must be SSRF-safe; with an API key, Haiku judges unsure
// setups, induces recipes, runs the daily model repair and the weekly check.
func newWebGenerator(client webfeed.Doer, apiKey string) (websource.Generator, bool, error) {
	opts := webfeed.Options{Client: client}
	if apiKey == "" {
		return websource.NewWebfeedGenerator(opts, nil), false, nil
	}
	model, err := claude.New(claude.Config{APIKey: apiKey, HTTPClient: &http.Client{Timeout: modelTimeout}})
	if err != nil {
		return nil, false, err
	}
	opts.Judge, opts.Inducer = model, model
	return websource.NewWebfeedGenerator(opts, model), true, nil
}

// webPoller is the slice of *websource.Service the poll loop drives.
type webPoller interface {
	PollDue(ctx context.Context) (int, error)
}

// runWebPoll scans for due web sources every tick; each source keeps its own schedule in next_poll_at.
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

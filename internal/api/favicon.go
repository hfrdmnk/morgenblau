package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"morgenblau/internal/safehttp"
)

const (
	faviconProxyTimeout = 5 * time.Second
	// site.standard.publication's icon lexicon says 1MB, but writers don't enforce it (2.4MB observed live);
	// the cap bounds proxy buffering, not lexicon compliance, so it carries headroom over real-world blobs.
	faviconProxyMaxBytes = 4 * 1024 * 1024
)

// FaviconReader gates the favicon proxy: only URLs the sync pipeline already stored on a known feed are eligible for streaming.
type FaviconReader interface {
	GetFeedIconURL(ctx context.Context, feedURL string) (*string, error)
}

// FaviconProxyHandler streams a known feed's stored favicon through the Go server so canvas can sample it without accepting an arbitrary upstream URL.
func FaviconProxyHandler(reader FaviconReader, client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := requireSession(w, r)
		if !ok {
			return
		}
		feedURL := r.URL.Query().Get("feed")
		if feedURL == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "feed is required")
			return
		}

		icon, err := reader.GetFeedIconURL(r.Context(), feedURL)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			slog.Warn("/api/favicon: lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if icon == nil || *icon == "" {
			http.NotFound(w, r)
			return
		}
		iconURL := *icon

		ctx, cancel := context.WithTimeout(r.Context(), faviconProxyTimeout)
		defer cancel()
		upReq, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
		if err != nil {
			slog.Warn("/api/favicon: bad upstream URL", "url", iconURL, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		upReq.Header.Set("User-Agent", safehttp.UserAgent)

		resp, err := client.Do(upReq)
		if err != nil {
			slog.Warn("/api/favicon: upstream failed", "url", iconURL, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(strings.ToLower(ct), "image/") {
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}

		// A truncated icon renders as a corrupt image and gets cached for 24h, so the cap rejects instead of streaming a partial body.
		if resp.ContentLength > faviconProxyMaxBytes {
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, faviconProxyMaxBytes+1))
		if err != nil {
			slog.Warn("/api/favicon: read failed", "url", iconURL, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}
		if len(body) > faviconProxyMaxBytes {
			writeError(w, http.StatusBadGateway, codeUpstreamError, "bad gateway")
			return
		}

		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		if _, err := w.Write(body); err != nil {
			slog.Warn("/api/favicon: write failed", "err", err)
		}
	})
}

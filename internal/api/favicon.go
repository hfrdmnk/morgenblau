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

	"morgenblau/internal/middleware/auth"
)

const (
	faviconProxyTimeout  = 5 * time.Second
	faviconProxyMaxBytes = 256 * 1024
)

// FaviconReader gates the favicon proxy: only URLs the sync pipeline has
// already stored on a known feed are eligible for streaming.
type FaviconReader interface {
	GetFeedIconURL(ctx context.Context, feedURL string) (*string, error)
}

// FaviconProxyHandler streams a feed's stored favicon through the Go server
// so the browser sees it same-origin and can sample dominant color via
// canvas. SSRF is bounded by the feed_url → icon_url table lookup — we never
// follow an arbitrary URL handed to us by the caller.
func FaviconProxyHandler(reader FaviconReader, client *http.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		feedURL := r.URL.Query().Get("feed")
		if feedURL == "" {
			http.Error(w, "feed is required", http.StatusBadRequest)
			return
		}

		ptr, err := reader.GetFeedIconURL(r.Context(), feedURL)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			slog.Warn("/api/favicon: lookup failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if ptr == nil || *ptr == "" {
			http.NotFound(w, r)
			return
		}
		iconURL := *ptr

		ctx, cancel := context.WithTimeout(r.Context(), faviconProxyTimeout)
		defer cancel()
		upReq, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
		if err != nil {
			slog.Warn("/api/favicon: bad upstream URL", "url", iconURL, "err", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		upReq.Header.Set("User-Agent", "Morgenblau/0.1 (+https://morgen.blue/about; bot@morgen.blue)")

		resp, err := client.Do(upReq)
		if err != nil {
			slog.Warn("/api/favicon: upstream failed", "url", iconURL, "err", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.HasPrefix(strings.ToLower(ct), "image/") {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		if _, err := io.Copy(w, io.LimitReader(resp.Body, faviconProxyMaxBytes)); err != nil {
			slog.Warn("/api/favicon: copy failed", "err", err)
		}
	})
}

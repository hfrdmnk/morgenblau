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

	"morgenblau/internal/database/db"
	"morgenblau/internal/feedkey"
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

// DiscoverCandidateFaviconReader is the fallback gate for a discovered-but-unsubscribed candidate, not yet in the feeds table.
type DiscoverCandidateFaviconReader interface {
	GetDiscoverSourceFaviconURL(ctx context.Context, sourceKey string) (*string, error)
}

// PublicationResolutionFaviconReader is the last fallback: the discover-crawl resolution cache already carries a
// real icon for most standardfeed publications once resolved, keyed by canonical_key (the publication's at:// URI, stored verbatim).
type PublicationResolutionFaviconReader interface {
	GetDiscoverPublicationResolutionByCanonicalKey(ctx context.Context, canonicalKey *string) (db.DiscoverPublicationResolution, error)
}

// OnDemandFaviconResolver is the last resort when every cache misses: it only ever fetches a site URL a
// background crawl already wrote (never one derived from the caller's feed param), so the never-caller-supplied-URL invariant holds.
type OnDemandFaviconResolver interface {
	Resolve(ctx context.Context, candidateKey string) (string, error)
}

// FaviconProxyHandler streams a feed's favicon through the Go server (same-origin, so canvas can sample color); SSRF is bounded
// by the feed_url→icon_url (or discover-candidate) table lookup, never an arbitrary caller-supplied URL.
func FaviconProxyHandler(reader FaviconReader, candidates DiscoverCandidateFaviconReader, resolutions PublicationResolutionFaviconReader, onDemand OnDemandFaviconResolver, client *http.Client) http.Handler {
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

		iconURL, err := resolveFaviconURL(r.Context(), reader, candidates, resolutions, onDemand, feedURL)
		if err != nil {
			slog.Warn("/api/favicon: lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if iconURL == "" {
			http.NotFound(w, r)
			return
		}

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

// resolveFaviconURL checks the subscribed-feed catalog, then the discover-candidate posts cache, then the discover-crawl
// resolution cache, then falls through to on-demand discovery, so unsubscribed cards still get a real favicon; "" with
// a nil error means no source has one (404, not a failure).
func resolveFaviconURL(ctx context.Context, reader FaviconReader, candidates DiscoverCandidateFaviconReader, resolutions PublicationResolutionFaviconReader, onDemand OnDemandFaviconResolver, feedURL string) (string, error) {
	ptr, err := reader.GetFeedIconURL(ctx, feedURL)
	switch {
	case err == nil:
		if ptr != nil && *ptr != "" {
			return *ptr, nil
		}
	case !errors.Is(err, sql.ErrNoRows):
		return "", err
	}

	// Candidate keys are feedkey.Normalize'd at construction (see discover_sources.go); normalizing here keeps this lookup aligned even if the caller passes the raw form.
	ptr, err = candidates.GetDiscoverSourceFaviconURL(ctx, feedkey.Normalize(feedURL))
	switch {
	case err == nil:
		if ptr != nil && *ptr != "" {
			return *ptr, nil
		}
	case !errors.Is(err, sql.ErrNoRows):
		return "", err
	}

	// canonical_key is either an at:// URI (feedkey.Normalize no-ops on it: url.Parse rejects the DID authority as an
	// invalid port) or, for a leaflet-resolved rss fallback, an already-Normalize'd feed URL; normalizing here matches both.
	resolutionKey := feedkey.Normalize(feedURL)
	row, err := resolutions.GetDiscoverPublicationResolutionByCanonicalKey(ctx, &resolutionKey)
	switch {
	case err == nil:
		if row.IconUrl != nil && *row.IconUrl != "" {
			return *row.IconUrl, nil
		}
	case !errors.Is(err, sql.ErrNoRows):
		return "", err
	}

	return onDemand.Resolve(ctx, resolutionKey)
}

package api

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"morgenblau/internal/websource"
)

// --- POST /api/web-sources ---

// WebSourceCreator is the slice of *websource.Service the create route needs.
type WebSourceCreator interface {
	Create(ctx context.Context, did, listingURL, anchorURL string) (websource.Preview, error)
}

type webSourceRequest struct {
	URL       string `json:"url"`
	AnchorURL string `json:"anchorUrl"`
}

type webSourceItemWire struct {
	URL   string     `json:"url"`
	Title string     `json:"title"`
	Date  *time.Time `json:"date"`
}

type webSourceWire struct {
	ID           string              `json:"id"`
	FeedURL      string              `json:"feedUrl"`
	Title        string              `json:"title"`
	SiteURL      string              `json:"siteUrl"`
	Health       websource.Health    `json:"health"`
	HealthReason string              `json:"healthReason"`
	NeedsAnchor  bool                `json:"needsAnchor"`
	Items        []webSourceItemWire `json:"items"`
	ItemCount    int                 `json:"itemCount"`
}

// WebSourcesCreateHandler generates (or reuses) the site's web source and returns a preview; it writes only server-owned storage, never the PDS.
func WebSourcesCreateHandler(svc WebSourceCreator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body webSourceRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.URL = strings.TrimSpace(body.URL)
		if body.URL == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "url is required")
			return
		}
		preview, err := svc.Create(r.Context(), sess.Data.AccountDID.String(), body.URL, strings.TrimSpace(body.AnchorURL))
		var limited *websource.RateLimitError
		switch {
		case errors.As(err, &limited):
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
			writeError(w, http.StatusTooManyRequests, codeRateLimited, "You've generated a lot of feeds in the last hour. Try again later.")
			return
		case errors.Is(err, websource.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Enter a web address that starts with http:// or https://")
			return
		case errors.Is(err, websource.ErrGeneration):
			slog.Warn("/api/web-sources: generation failed", "url", body.URL, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "Couldn't read that page")
			return
		case errors.Is(err, websource.ErrUnavailable):
			writeError(w, http.StatusServiceUnavailable, codeUnavailable, "Feed generation isn't available yet")
			return
		case err != nil:
			slog.Error("/api/web-sources: create failed", "url", body.URL, "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		src := preview.Source
		out := webSourceWire{
			ID: src.ID, FeedURL: src.FeedURL, Title: src.Title, SiteURL: src.SiteURL, Health: src.Health,
			HealthReason: src.HealthReason, NeedsAnchor: src.NeedsAnchor, Items: []webSourceItemWire{}, ItemCount: preview.ItemCount,
		}
		for _, it := range preview.Items {
			item := webSourceItemWire{URL: it.URL, Title: it.Title}
			if !it.Date.IsZero() {
				item.Date = &it.Date
			}
			out.Items = append(out.Items, item)
		}
		writeJSON(w, out)
	})
}

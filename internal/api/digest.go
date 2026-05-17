package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
	"morgenblau/internal/middleware/auth"
)

// DigestReader is the slice of *db.Queries the digest handler depends on.
type DigestReader interface {
	ListDigestForUser(ctx context.Context, arg db.ListDigestForUserParams) ([]db.ListDigestForUserRow, error)
	ListAllEntriesForUser(ctx context.Context, did string) ([]db.ListAllEntriesForUserRow, error)
}

// EntryWire is the on-the-wire entry shape consumed by /consume and the entry
// detail page. body is the sanitized HTML; the frontend treats it as trusted.
type EntryWire struct {
	ID          int64      `json:"id"`
	EntrySlug   string     `json:"entrySlug"`
	Title       *string    `json:"title"`
	URL         string     `json:"url"`
	ContentType string     `json:"contentType"`
	PublishedAt string     `json:"publishedAt"`
	Source      SourceMeta `json:"source"`
	Body        *string    `json:"body"`
	Metadata    *string    `json:"metadata,omitempty"`
}

type SourceMeta struct {
	FeedURL    string  `json:"feedUrl"`
	Title      *string `json:"title"`
	SiteURL    *string `json:"siteUrl"`
	FaviconURL *string `json:"faviconUrl"`
}

// DigestResponse adds in-flight metadata so the frontend can swap empty-state
// copy without a second round-trip.
type DigestResponse struct {
	Date         string      `json:"date"`
	Entries      []EntryWire `json:"entries"`
	HasActiveJob bool        `json:"hasActiveJob"`
}

// JobsActiveProbe is the slice of jobs.Tracker the digest handler uses to
// decide between in-flight and steady-state empty copy.
type JobsActiveProbe interface {
	ActiveForUser(did syntax.DID) *jobs.Job
}

// DigestHandler returns entries for ?date=YYYY-MM-DD (default: today UTC),
// joined across the user's Tier-1 subscriptions.
func DigestHandler(reader DigestReader, jobsSrc JobsActiveProbe) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		dateStr := r.URL.Query().Get("date")
		did := sess.Data.AccountDID.String()

		var entries []EntryWire
		var responseDate string

		if dateStr == "" {
			// TODO: when no ?date param is provided, this should default to
			// date=today (UTC). Currently returns all entries unbounded for
			// debugging; remove this branch + the ListAllEntriesForUser sqlc
			// query before v1.
			rows, err := reader.ListAllEntriesForUser(r.Context(), did)
			if err != nil {
				slog.Warn("/api/digest: list-all failed", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			entries = make([]EntryWire, 0, len(rows))
			for _, row := range rows {
				entries = append(entries, allEntriesRowToWire(row))
			}
			responseDate = time.Now().UTC().Format("2006-01-02")
		} else {
			parsed, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				http.Error(w, "invalid date (want YYYY-MM-DD)", http.StatusBadRequest)
				return
			}
			day := parsed.UTC()
			next := day.Add(24 * time.Hour)

			rows, err := reader.ListDigestForUser(r.Context(), db.ListDigestForUserParams{
				Did:           did,
				PublishedAt:   day.Format(time.RFC3339),
				PublishedAt_2: next.Format(time.RFC3339),
			})
			if err != nil {
				slog.Warn("/api/digest: list failed", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			entries = make([]EntryWire, 0, len(rows))
			for _, row := range rows {
				entries = append(entries, digestRowToWire(row))
			}
			responseDate = day.Format("2006-01-02")
		}

		hasActive := false
		if jobsSrc != nil && jobsSrc.ActiveForUser(sess.Data.AccountDID) != nil {
			hasActive = true
		}

		writeJSON(w, DigestResponse{
			Date:         responseDate,
			Entries:      entries,
			HasActiveJob: hasActive,
		})
	})
}

func allEntriesRowToWire(row db.ListAllEntriesForUserRow) EntryWire {
	return EntryWire{
		ID:          row.ID,
		EntrySlug:   row.EntrySlug,
		Title:       row.Title,
		URL:         row.Url,
		ContentType: row.ContentType,
		PublishedAt: row.PublishedAt,
		Source:      buildSourceMeta(row.FeedUrl, row.FeedTitle, row.FeedSiteUrl, row.FeedIconUrl),
		Body:        row.ContentHtml,
		Metadata:    row.Metadata,
	}
}

func digestRowToWire(row db.ListDigestForUserRow) EntryWire {
	return EntryWire{
		ID:          row.ID,
		EntrySlug:   row.EntrySlug,
		Title:       row.Title,
		URL:         row.Url,
		ContentType: row.ContentType,
		PublishedAt: row.PublishedAt,
		Source:      buildSourceMeta(row.FeedUrl, row.FeedTitle, row.FeedSiteUrl, row.FeedIconUrl),
		Body:        row.ContentHtml,
		Metadata:    row.Metadata,
	}
}

// buildSourceMeta returns the canonical favicon resolved by the sync pipeline,
// or nil when discovery hasn't (or couldn't) populate one — the frontend
// renders a placeholder in that case.
func buildSourceMeta(feedURL string, title, siteURL, feedIconURL *string) SourceMeta {
	var favicon *string
	if feedIconURL != nil && *feedIconURL != "" {
		favicon = feedIconURL
	}
	return SourceMeta{
		FeedURL:    feedURL,
		Title:      title,
		SiteURL:    siteURL,
		FaviconURL: favicon,
	}
}

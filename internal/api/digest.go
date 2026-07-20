package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
)

// DigestReader is the slice of *db.Queries the digest handler depends on.
type DigestReader interface {
	ListDigestForUser(ctx context.Context, arg db.ListDigestForUserParams) ([]db.ListDigestForUserRow, error)
	ListAllEntriesForUser(ctx context.Context, did string) ([]db.ListAllEntriesForUserRow, error)
}

// EntryWire is the on-the-wire entry shape; Body is pre-sanitized HTML the frontend trusts as-is.
// SavedState stays nil here (only the entry detail handler sets it) to avoid N+1 lookups on the digest list.
type EntryWire struct {
	ID          int64        `json:"id"`
	EntrySlug   string       `json:"entrySlug"`
	Title       *string      `json:"title"`
	URL         string       `json:"url"`
	ContentType string       `json:"contentType"`
	PublishedAt string       `json:"publishedAt"`
	Source      SourceMeta   `json:"source"`
	Body        *string      `json:"body"`
	Metadata    *string      `json:"metadata,omitempty"`
	SavedState  *SavedState  `json:"savedState"`
	SharedState *SharedState `json:"sharedState"`
}

// SavedState mirrors the frontend's view; Rkey is what the client DELETEs on un-save.
type SavedState struct {
	Rkey string `json:"rkey"`
}

// SharedState mirrors SavedState; Rkey is the recommend rkey for standardfeed shares, the share rkey for rss.
type SharedState struct {
	Rkey string `json:"rkey"`
}

type SourceMeta struct {
	FeedURL    string  `json:"feedUrl"`
	Title      *string `json:"title"`
	SiteURL    *string `json:"siteUrl"`
	FaviconURL *string `json:"faviconUrl"`
	// Rkey is only set on the reader path (entryRowToWire); digest/source-list leave it empty so it drops from JSON.
	Rkey string `json:"rkey,omitempty"`
}

// DigestResponse adds in-flight metadata so the frontend can swap empty-state copy without a second round-trip.
type DigestResponse struct {
	Date         string      `json:"date"`
	Entries      []EntryWire `json:"entries"`
	HasActiveJob bool        `json:"hasActiveJob"`
}

// JobsActiveProbe is the slice of jobs.Tracker used to pick in-flight vs steady-state empty copy.
type JobsActiveProbe interface {
	ActiveForUser(did syntax.DID) *jobs.Job
}

// DigestHandler returns entries for ?date=YYYY-MM-DD (default: today UTC), joined across the user's Tier-1 subscriptions.
func DigestHandler(reader DigestReader, jobsSrc JobsActiveProbe) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}

		dateStr := r.URL.Query().Get("date")
		did := sess.Data.AccountDID.String()

		var entries []EntryWire
		var responseDate string

		if dateStr == "" {
			// TODO: missing ?date returns all entries unbounded for debugging; default to today and drop this branch + ListAllEntriesForUser before v1.
			rows, err := reader.ListAllEntriesForUser(r.Context(), did)
			if err != nil {
				slog.Warn("/api/digest: list-all failed", "err", err)
				writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
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
				writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid date (want YYYY-MM-DD)")
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
				writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
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

// entryListFields is the shared field set behind the entry-list row-to-wire mappers, since the three list queries differ only in filter, not shape.
type entryListFields struct {
	ID           int64
	EntrySlug    string
	Title        *string
	Url          string
	ContentType  string
	PublishedAt  string
	FeedUrl      string
	FeedTitle    *string
	CatalogTitle *string
	FeedSiteUrl  *string
	FeedIconUrl  *string
	ContentHtml  *string
	Metadata     *string
}

func entryListRowToWire(f entryListFields) EntryWire {
	return EntryWire{
		ID:          f.ID,
		EntrySlug:   f.EntrySlug,
		Title:       f.Title,
		URL:         f.Url,
		ContentType: f.ContentType,
		PublishedAt: f.PublishedAt,
		Source:      buildSourceMeta(f.FeedUrl, displayTitle(f.FeedTitle, f.CatalogTitle), f.FeedSiteUrl, f.FeedIconUrl),
		Body:        f.ContentHtml,
		Metadata:    f.Metadata,
	}
}

func allEntriesRowToWire(row db.ListAllEntriesForUserRow) EntryWire {
	return entryListRowToWire(entryListFields{
		ID:           row.ID,
		EntrySlug:    row.EntrySlug,
		Title:        row.Title,
		Url:          row.Url,
		ContentType:  row.ContentType,
		PublishedAt:  row.PublishedAt,
		FeedUrl:      row.FeedUrl,
		FeedTitle:    row.FeedTitle,
		CatalogTitle: row.CatalogTitle,
		FeedSiteUrl:  row.FeedSiteUrl,
		FeedIconUrl:  row.FeedIconUrl,
		ContentHtml:  row.ContentHtml,
		Metadata:     row.Metadata,
	})
}

func digestRowToWire(row db.ListDigestForUserRow) EntryWire {
	return entryListRowToWire(entryListFields{
		ID:           row.ID,
		EntrySlug:    row.EntrySlug,
		Title:        row.Title,
		Url:          row.Url,
		ContentType:  row.ContentType,
		PublishedAt:  row.PublishedAt,
		FeedUrl:      row.FeedUrl,
		FeedTitle:    row.FeedTitle,
		CatalogTitle: row.CatalogTitle,
		FeedSiteUrl:  row.FeedSiteUrl,
		FeedIconUrl:  row.FeedIconUrl,
		ContentHtml:  row.ContentHtml,
		Metadata:     row.Metadata,
	})
}

// displayTitle prefers the user's per-subscription title over the catalog title (publication name for standardfeed, NULL for rss).
func displayTitle(userTitle, catalogTitle *string) *string {
	if userTitle != nil && *userTitle != "" {
		return userTitle
	}
	return catalogTitle
}

// buildSourceMeta returns the sync-resolved favicon, or nil when discovery hasn't populated one (the frontend renders a placeholder).
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

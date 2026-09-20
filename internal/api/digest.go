package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
	"morgenblau/internal/newsletter"
)

// DigestReader is the slice of *db.Queries the digest handler depends on.
type DigestReader interface {
	ListDigestForUser(ctx context.Context, arg db.ListDigestForUserParams) ([]db.ListDigestForUserRow, error)
	ListAllEntriesForUser(ctx context.Context, did string) ([]db.ListAllEntriesForUserRow, error)
}

// EntryWire is the on-the-wire entry shape; Body is pre-sanitized HTML the frontend trusts as-is.
type EntryWire struct {
	ID          any                    `json:"id"`
	EntrySlug   string                 `json:"entrySlug"`
	Title       *string                `json:"title"`
	URL         *string                `json:"url,omitempty"`
	ContentType string                 `json:"contentType"`
	PublishedAt string                 `json:"publishedAt"`
	Source      SourceMeta             `json:"source"`
	Body        *string                `json:"body"`
	Metadata    *string                `json:"metadata,omitempty"`
	Newsletter  *NewsletterMessageMeta `json:"newsletter,omitempty"`
	SavedState  *SavedState            `json:"savedState"`
}

// SavedState mirrors the frontend's view; Rkey is what the client DELETEs on un-save.
type SavedState struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
	Rkey string `json:"rkey,omitempty"`
}

type SourceMeta struct {
	Kind       string  `json:"kind,omitempty"`
	ID         string  `json:"id,omitempty"`
	FeedURL    string  `json:"feedUrl,omitempty"`
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

type newsletterDigestReader interface {
	ListDigestMessages(context.Context, string, time.Time, time.Time) ([]newsletter.Message, error)
}

// DigestHandler returns entries for the requested browser-local calendar day, joined across the user's Tier-1 subscriptions.
func DigestHandler(reader DigestReader, jobsSrc JobsActiveProbe, privateReaders ...newsletterDigestReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}

		dateStr := r.URL.Query().Get("date")
		did := sess.Data.AccountDID.String()

		var entries []EntryWire
		var responseDate string
		var day, next time.Time
		if dateStr == "" {
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
			var err error
			responseDate, day, next, err = digestDayBounds(dateStr, r.URL.Query().Get("timezone"), time.Now())
			if err != nil {
				writeError(w, http.StatusBadRequest, codeInvalidRequest, err.Error())
				return
			}
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
		}

		if len(privateReaders) > 0 && privateReaders[0] != nil {
			privateResponse(w)
			messages, err := privateReaders[0].ListDigestMessages(r.Context(), did, day, next)
			if err != nil {
				writeNewsletterError(w, err)
				return
			}
			for _, message := range messages {
				entries = append(entries, newsletterMessageToWire(message))
			}
			sort.SliceStable(entries, func(i, j int) bool {
				left, leftErr := time.Parse(time.RFC3339, entries[i].PublishedAt)
				right, rightErr := time.Parse(time.RFC3339, entries[j].PublishedAt)
				if leftErr != nil || rightErr != nil {
					return false
				}
				return left.After(right)
			})
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

func digestDayBounds(dateStr, timezone string, now time.Time) (string, time.Time, time.Time, error) {
	loc := time.UTC
	if timezone != "" {
		var err error
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return "", time.Time{}, time.Time{}, fmt.Errorf("invalid timezone")
		}
	}
	if dateStr == "" {
		dateStr = now.In(loc).Format("2006-01-02")
	}
	day, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return "", time.Time{}, time.Time{}, fmt.Errorf("invalid date (want YYYY-MM-DD)")
	}
	return dateStr, day.UTC(), day.AddDate(0, 0, 1).UTC(), nil
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
	entryURL := f.Url
	return EntryWire{
		ID:          f.ID,
		EntrySlug:   f.EntrySlug,
		Title:       f.Title,
		URL:         &entryURL,
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

// buildSourceMeta returns the cached source favicon, or nil when none is known.
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

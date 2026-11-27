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

type DigestReader interface {
	ListDigestForUser(ctx context.Context, arg db.ListDigestForUserParams) ([]db.ListDigestForUserRow, error)
}

type EntryWire struct {
	ID            any                    `json:"id"`
	EntrySlug     string                 `json:"entrySlug"`
	Title         *string                `json:"title"`
	URL           *string                `json:"url,omitempty"`
	ContentType   string                 `json:"contentType"`
	PublishedAt   string                 `json:"publishedAt"`
	Source        SourceMeta             `json:"source"`
	SanitizedBody *string                `json:"body"`
	Metadata      *string                `json:"metadata,omitempty"`
	Image         *string                `json:"image,omitempty"`
	Newsletter    *NewsletterMessageMeta `json:"newsletter,omitempty"`
	SavedState    *SavedState            `json:"savedState"`
}

type SavedState struct {
	Kind string   `json:"kind,omitempty"`
	ID   string   `json:"id,omitempty"`
	Rkey string   `json:"rkey,omitempty"`
	Tags []string `json:"tags,omitempty"`
}

type SourceMeta struct {
	Kind       string  `json:"kind,omitempty"`
	ID         string  `json:"id,omitempty"`
	FeedURL    string  `json:"feedUrl,omitempty"`
	Title      *string `json:"title"`
	SiteURL    *string `json:"siteUrl"`
	FaviconURL *string `json:"faviconUrl"`
	Rkey       string  `json:"rkey,omitempty"`
	Primary    bool    `json:"primary,omitempty"`
}

type DigestResponse struct {
	Date         string      `json:"date"`
	Entries      []EntryWire `json:"entries"`
	HasActiveJob bool        `json:"hasActiveJob"`
}

type JobsActiveProbe interface {
	ActiveForUser(did syntax.DID) *jobs.Job
}

type newsletterDigestReader interface {
	ListDigestMessages(context.Context, string, time.Time, time.Time) ([]newsletter.Message, error)
}

func DigestHandler(reader DigestReader, jobsSrc JobsActiveProbe, privateReaders ...newsletterDigestReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}

		dateStr := r.URL.Query().Get("date")
		did := sess.Data.AccountDID.String()

		responseDate, day, next, err := digestDayBounds(dateStr, r.URL.Query().Get("timezone"), time.Now())
		if err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, err.Error())
			return
		}
		hasActive := jobsSrc != nil && jobsSrc.ActiveForUser(sess.Data.AccountDID) != nil
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
		entries := make([]EntryWire, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, digestRowToWire(row))
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

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Source.Primary && !entries[j].Source.Primary
		})

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
		ID:            f.ID,
		EntrySlug:     f.EntrySlug,
		Title:         f.Title,
		URL:           &entryURL,
		ContentType:   f.ContentType,
		PublishedAt:   f.PublishedAt,
		Source:        buildSourceMeta(f.FeedUrl, displayTitle(f.FeedTitle, f.CatalogTitle), f.FeedSiteUrl, f.FeedIconUrl),
		SanitizedBody: f.ContentHtml,
		Metadata:      f.Metadata,
	}
}

func digestRowToWire(row db.ListDigestForUserRow) EntryWire {
	wire := entryListRowToWire(entryListFields{
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
	wire.Source.Rkey = row.SubscriptionRkey
	wire.Source.Primary = row.IsPrimary != 0
	wire.Image = entryImagePath(row.EntrySlug, row.Metadata, row.Url)
	return wire
}

func displayTitle(userTitle, catalogTitle *string) *string {
	if userTitle != nil && *userTitle != "" {
		return userTitle
	}
	return catalogTitle
}

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

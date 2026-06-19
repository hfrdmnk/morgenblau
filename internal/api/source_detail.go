package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
)

// sourceEntriesLimit caps the per-source entry list. 200 matches the brief —
// pagination beyond that is deferred until a real source bumps the ceiling.
const sourceEntriesLimit int64 = 200

// SubscriptionDetailWire extends the list wire with the per-source stats the
// detail page renders. Keeping it distinct from SubscriptionWire avoids
// growing the list response with fields that would always be zero.
type SubscriptionDetailWire struct {
	SubscriptionWire
	TotalEntries int64 `json:"totalEntries"`
	SavedByYou   int64 `json:"savedByYou"`
}

// SourceDetailReader is the narrow read used by SubscriptionGetHandler.
type SourceDetailReader interface {
	GetUserSourceWithStats(ctx context.Context, arg db.GetUserSourceWithStatsParams) (db.GetUserSourceWithStatsRow, error)
}

// SourceEntriesReader is the narrow read used by SubscriptionEntriesHandler.
// Two queries: ownership (subscription lookup) and the entry list itself.
type SourceEntriesReader interface {
	GetUserSubscription(ctx context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error)
	ListEntriesForSource(ctx context.Context, arg db.ListEntriesForSourceParams) ([]db.ListEntriesForSourceRow, error)
}

// SubscriptionGetHandler returns one user's subscription joined with its feed
// stats: windowed counts, total_entries, saved_by_you. The frequency bucket
// is computed here (same rule as the list endpoint).
func SubscriptionGetHandler(reader SourceDetailReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			http.Error(w, "invalid rkey", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		row, err := reader.GetUserSourceWithStats(r.Context(), db.GetUserSourceWithStatsParams{
			Did:       sess.Data.AccountDID.String(),
			Rkey:      rkey,
			Now:       now.Format(time.RFC3339),
			Cutoff7d:  now.AddDate(0, 0, -7).Format(time.RFC3339),
			Cutoff28d: now.AddDate(0, 0, -28).Format(time.RFC3339),
			Cutoff56d: now.AddDate(0, 0, -56).Format(time.RFC3339),
			Cutoff84d: now.AddDate(0, 0, -84).Format(time.RFC3339),
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			slog.Warn("/api/subscriptions/{rkey}: lookup failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, sourceDetailRowToWire(row, now))
	})
}

// SubscriptionEntriesHandler returns up to sourceEntriesLimit newest entries
// for the given subscription. Ownership is verified explicitly so the
// requester can't probe other users' rkeys via the entry list endpoint.
func SubscriptionEntriesHandler(reader SourceEntriesReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			http.Error(w, "invalid rkey", http.StatusBadRequest)
			return
		}
		didStr := sess.Data.AccountDID.String()

		sub, err := reader.GetUserSubscription(r.Context(), db.GetUserSubscriptionParams{
			Did:  didStr,
			Rkey: rkey,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			slog.Warn("/api/subscriptions/{rkey}/entries: ownership lookup failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		rows, err := reader.ListEntriesForSource(r.Context(), db.ListEntriesForSourceParams{
			Did:     didStr,
			FeedUrl: sub.FeedUrl,
			Limit:   sourceEntriesLimit,
		})
		if err != nil {
			slog.Warn("/api/subscriptions/{rkey}/entries: list failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]EntryWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, sourceEntryRowToWire(row))
		}
		writeJSON(w, out)
	})
}

func sourceDetailRowToWire(row db.GetUserSourceWithStatsRow, now time.Time) SubscriptionDetailWire {
	value := map[string]any{"feedUrl": row.FeedUrl}
	title := ""
	if row.Title != nil {
		value["title"] = *row.Title
		title = *row.Title
	}
	siteURL := ""
	if row.SiteUrl != nil {
		value["siteUrl"] = *row.SiteUrl
		siteURL = *row.SiteUrl
	}
	faviconURL := ""
	if row.IconUrl != nil {
		faviconURL = *row.IconUrl
	}
	primary := row.IsPrimary != 0
	tags := unmarshalTags(row.Tags)
	if primary {
		value["primary"] = true
	}
	if len(tags) > 0 {
		value["tags"] = tags
	}
	lastPublished := asString(row.LastPublishedAt)
	firstPublished := asString(row.FirstPublishedAt)
	return SubscriptionDetailWire{
		SubscriptionWire: SubscriptionWire{
			URI:             row.AtUri,
			Value:           value,
			Rkey:            row.Rkey,
			FeedURL:         row.FeedUrl,
			Title:           title,
			SiteURL:         siteURL,
			FaviconURL:      faviconURL,
			Frequency:       frequencyBucket(firstPublished, row.Count7d, row.Count28d, row.Count56d, row.Count84d, now),
			LastPublishedAt: lastPublished,
			Primary:         primary,
			Tags:            tags,
		},
		TotalEntries: row.TotalEntries,
		SavedByYou:   row.SavedByYou,
	}
}

func sourceEntryRowToWire(row db.ListEntriesForSourceRow) EntryWire {
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

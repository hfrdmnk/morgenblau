package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"morgenblau/internal/database/db"
)

// sourceEntriesLimit caps the per-source entry list at 200; pagination is deferred until a real source needs more.
const sourceEntriesLimit int64 = 200

// SubscriptionDetailWire extends the list wire with detail-page stats, kept separate so the list response doesn't carry always-zero fields.
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
type SourceEntriesReader interface {
	GetUserSubscription(ctx context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error)
	ListEntriesForSource(ctx context.Context, arg db.ListEntriesForSourceParams) ([]db.ListEntriesForSourceRow, error)
}

// SubscriptionGetHandler returns a subscription with feed stats; the frequency bucket uses the same rule as the list endpoint.
func SubscriptionGetHandler(reader SourceDetailReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid rkey")
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
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		writeJSON(w, sourceDetailRowToWire(row, now))
	})
}

// SubscriptionEntriesHandler returns up to sourceEntriesLimit newest entries; ownership is verified explicitly so a requester can't probe other users' rkeys.
func SubscriptionEntriesHandler(reader SourceEntriesReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid rkey")
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
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		rows, err := reader.ListEntriesForSource(r.Context(), db.ListEntriesForSourceParams{
			Did:     didStr,
			FeedUrl: sub.FeedUrl,
			Limit:   sourceEntriesLimit,
		})
		if err != nil {
			slog.Warn("/api/subscriptions/{rkey}/entries: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
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
	wire := subscriptionStatsRowToWire(subscriptionStatsFields{
		Rkey:             row.Rkey,
		AtUri:            row.AtUri,
		FeedUrl:          row.FeedUrl,
		Kind:             row.Kind,
		Title:            row.Title,
		IsPrimary:        row.IsPrimary,
		Tags:             row.Tags,
		SiteUrl:          row.SiteUrl,
		IconUrl:          row.IconUrl,
		CatalogTitle:     row.CatalogTitle,
		LastPublishedAt:  row.LastPublishedAt,
		FirstPublishedAt: row.FirstPublishedAt,
		Count7d:          row.Count7d,
		Count28d:         row.Count28d,
		Count56d:         row.Count56d,
		Count84d:         row.Count84d,
	}, now)
	return SubscriptionDetailWire{
		SubscriptionWire: wire,
		TotalEntries:     row.TotalEntries,
		SavedByYou:       row.SavedByYou,
	}
}

func sourceEntryRowToWire(row db.ListEntriesForSourceRow) EntryWire {
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

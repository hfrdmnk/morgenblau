package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"morgenblau/internal/database/db"
	"morgenblau/internal/tags"
)

// SourcesReader is the narrow read used by the list endpoint; it carries the per-feed entry stats the sources card renders.
type SourcesReader interface {
	ListUserSourcesWithStats(ctx context.Context, arg db.ListUserSourcesWithStatsParams) ([]db.ListUserSourcesWithStatsRow, error)
}

// SubscriptionsListHandler returns subscriptions joined with feed metadata; the frequency bucket is computed here, not in SQL, so the rule lives in one place.
func SubscriptionsListHandler(reader SourcesReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		now := time.Now().UTC()
		params := db.ListUserSourcesWithStatsParams{
			Did:       sess.Data.AccountDID.String(),
			Now:       now.Format(time.RFC3339),
			Cutoff7d:  now.AddDate(0, 0, -7).Format(time.RFC3339),
			Cutoff28d: now.AddDate(0, 0, -28).Format(time.RFC3339),
			Cutoff56d: now.AddDate(0, 0, -56).Format(time.RFC3339),
			Cutoff84d: now.AddDate(0, 0, -84).Format(time.RFC3339),
		}
		rows, err := reader.ListUserSourcesWithStats(r.Context(), params)
		if err != nil {
			slog.Warn("/api/subscriptions: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		out := make([]SubscriptionWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, sourceRowToWire(row, now))
		}
		writeJSON(w, out)
	})
}

// --- GET /api/subscriptions/tags ---

// TagsReader is the narrow read used by the "my tags" endpoint.
type TagsReader interface {
	ListUserSubscriptionTags(ctx context.Context, did string) ([]*string, error)
}

type tagsResponse struct {
	Tags []string `json:"tags"`
}

// SubscriptionsTagsHandler returns the distinct, case-insensitively deduped and sorted union of tags across the user's subscriptions, always as a JSON array, never null.
func SubscriptionsTagsHandler(reader TagsReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rows, err := reader.ListUserSubscriptionTags(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/subscriptions/tags: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		seen := map[string]struct{}{}
		out := []string{}
		for _, row := range rows {
			for _, tag := range tags.Unmarshal(row) {
				key := strings.ToLower(tag)
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, tag)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i]) < strings.ToLower(out[j])
		})
		writeJSON(w, tagsResponse{Tags: out})
	})
}

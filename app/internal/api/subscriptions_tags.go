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
	"morgenblau/internal/websource"
)

type SourcesReader interface {
	YouTubeFeedHealth
	ListUserSourcesWithStats(ctx context.Context, arg db.ListUserSourcesWithStatsParams) ([]db.ListUserSourcesWithStatsRow, error)
}

func SubscriptionsListHandler(reader SourcesReader, webFeeds websource.FeedPrefixes) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		now := time.Now().UTC()
		params := db.ListUserSourcesWithStatsParams{
			Did:                 sess.Data.AccountDID.String(),
			Now:                 now.Format(time.RFC3339),
			Cutoff7d:            now.AddDate(0, 0, -7).Format(time.RFC3339),
			Cutoff28d:           now.AddDate(0, 0, -28).Format(time.RFC3339),
			Cutoff56d:           now.AddDate(0, 0, -56).Format(time.RFC3339),
			Cutoff84d:           now.AddDate(0, 0, -84).Format(time.RFC3339),
			WebFeedPrefix:       webFeeds.Current,
			LegacyWebFeedPrefix: webFeeds.Legacy,
		}
		rows, err := reader.ListUserSourcesWithStats(r.Context(), params)
		if err != nil {
			slog.Warn("/api/subscriptions: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		down, err := youtubeDown(r.Context(), reader, now)
		if err != nil {
			slog.Warn("/api/subscriptions: youtube health failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		out := make([]SubscriptionWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, sourceRowToWire(row, now, down))
		}
		writeJSON(w, out)
	})
}

type TagsReader interface {
	ListUserSubscriptionTags(ctx context.Context, did string) ([]*string, error)
}

type tagsResponse struct {
	Tags []string `json:"tags"`
}

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
		var lists [][]string
		for _, row := range rows {
			lists = append(lists, tags.Unmarshal(row))
		}
		out := distinctTags(lists...)
		writeJSON(w, tagsResponse{Tags: out})
	})
}

func distinctTags(lists ...[]string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, list := range lists {
		for _, tag := range list {
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
	return out
}

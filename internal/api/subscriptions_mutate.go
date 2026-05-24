package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
)

// IndexRkeyReader fetches a single Tier-1 row + supports the dedupe probe
// already defined on IndexReader.
type IndexRkeyReader interface {
	IndexReader
	GetUserSubscription(ctx context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error)
}

// IndexDeleter is the slice of writes the DELETE handler depends on.
type IndexDeleter interface {
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
}

type patchRequest struct {
	Title *string `json:"title"`
}

// SubscriptionsPatchHandler updates metadata on the user's subscription via
// putRecord. Metadata-only — no fetch dispatch.
func SubscriptionsPatchHandler(reader IndexRkeyReader, writer IndexWriter, pds atprepo.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			http.Error(w, "rkey is required", http.StatusBadRequest)
			return
		}
		didStr := sess.Data.AccountDID.String()

		row, err := reader.GetUserSubscription(r.Context(), db.GetUserSubscriptionParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Could be "doesn't exist" or "exists but belongs to another user."
				// Both collapse to 403 to avoid leaking existence — see acceptance criteria.
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			slog.Warn("/api/subscriptions PATCH: load failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		var body patchRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		// Decide what's actually changing.
		changed := false
		newTitle := row.Title
		if body.Title != nil {
			if changedString(row.Title, *body.Title) {
				newTitle = nilIfEmpty(*body.Title)
				changed = true
			}
		}

		if !changed {
			// No diff — return the existing record without a PDS hit.
			writeJSON(w, rowToWire(row))
			return
		}

		// Build the new record body. PDS putRecord replaces atomically.
		record := map[string]any{
			"feedUrl":   row.FeedUrl,
			"createdAt": row.CreatedAt,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		if newTitle != nil {
			record["title"] = *newTitle
		}

		// TODO(blue.morgen lexicon): once the blue.morgen.feed.subscription
		// lexicon is published as a com.atproto.lexicon.schema record and
		// resolvable on the network, validate `record` before write. Use
		// lexicon.ValidateRecord(&catalog, obj, "blue.morgen.feed.subscription", 0)
		// after decoding with data.UnmarshalJSON. See SPEC.md <lexicons>.
		ref, err := pds.PutRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey, record)
		if err != nil {
			slog.Warn("/api/subscriptions PATCH: PDS put failed", "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if err := writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
			Did:       didStr,
			Rkey:      rkey,
			AtUri:     ref.URI,
			FeedUrl:   row.FeedUrl,
			Title:     newTitle,
			CreatedAt: row.CreatedAt,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("/api/subscriptions PATCH: Tier-1 upsert failed", "err", err)
		}
		row.Title = newTitle
		row.UpdatedAt = now
		row.AtUri = ref.URI
		writeJSON(w, rowToWire(row))
	})
}

// SubscriptionsDeleteHandler tombstones the PDS record and removes the
// Tier-1 row. Tier-2 feeds row is left alone — other users may still subscribe.
func SubscriptionsDeleteHandler(reader IndexRkeyReader, deleter IndexDeleter, pds atprepo.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			http.Error(w, "rkey is required", http.StatusBadRequest)
			return
		}
		didStr := sess.Data.AccountDID.String()
		_, err := reader.GetUserSubscription(r.Context(), db.GetUserSubscriptionParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			slog.Warn("/api/subscriptions DELETE: load failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey); err != nil {
			slog.Warn("/api/subscriptions DELETE: PDS delete failed", "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		if err := deleter.DeleteUserSubscription(r.Context(), db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey}); err != nil {
			slog.Warn("/api/subscriptions DELETE: Tier-1 delete failed", "err", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func changedString(old *string, next string) bool {
	if old == nil {
		return next != ""
	}
	return *old != next
}

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/standardfeed"
	"morgenblau/internal/tags"
)

// IndexRkeyReader fetches a single Tier-1 row + supports the dedupe probe
// already defined on IndexReader. GetFeed backs the rss PATCH's siteUrl
// preservation.
type IndexRkeyReader interface {
	IndexReader
	GetUserSubscription(ctx context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error)
	GetFeed(ctx context.Context, feedURL string) (db.Feed, error)
}

// IndexDeleter is the slice of writes the DELETE handler depends on.
type IndexDeleter interface {
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
}

type patchRequest struct {
	Title   *string   `json:"title"`
	Primary *bool     `json:"primary"`
	Tags    *[]string `json:"tags"`
	FeedURL *string   `json:"feedUrl"`
}

// patchResponse is the updated subscription wire plus, when the feed URL
// changed, the id of the fetch job dispatched for the new feed so the client
// can poll it.
type patchResponse struct {
	SubscriptionWire
	JobID string `json:"jobId,omitempty"`
}

// SubscriptionsPatchHandler updates the user's subscription via putRecord.
// Metadata edits (title, primary, tags) replace the record in place. A feedUrl
// change re-points the subscription to a different feed: it additionally upserts
// the Tier-2 catalog row and dispatches a fetch_one_feed for the new feed (the
// add path's contract, applied to an existing record).
func SubscriptionsPatchHandler(reader IndexRkeyReader, writer IndexWriter, pds atprepo.Writer, disp FetchDispatcher) http.Handler {
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

		// A standardfeed source has no feed URL to re-point — its identity
		// is the publication at-uri on the standard record.
		if row.Kind == "standardfeed" && body.FeedURL != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"message": "feed URL cannot be changed for a publication source"})
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

		newPrimary := row.IsPrimary
		if body.Primary != nil {
			if want := boolToInt64(*body.Primary); want != row.IsPrimary {
				newPrimary = want
				changed = true
			}
		}

		newTags := row.Tags
		var newTagsSlice []string
		if body.Tags != nil {
			newTagsSlice = normalizeTags(*body.Tags)
			if !tagsEqual(newTagsSlice, tags.Unmarshal(row.Tags)) {
				newTags = tags.Marshal(newTagsSlice)
				changed = true
			} else {
				newTagsSlice = tags.Unmarshal(row.Tags)
			}
		} else {
			newTagsSlice = tags.Unmarshal(row.Tags)
		}

		// A feedUrl change re-points the subscription to a different feed.
		newFeedURL := row.FeedUrl
		feedChanged := false
		if body.FeedURL != nil {
			if candidate := strings.TrimSpace(*body.FeedURL); candidate != "" && candidate != row.FeedUrl {
				// The add path resolves feeds through feedfinder; PATCH re-points
				// to a client-computed URL (e.g. the Shorts toggle) and must stay
				// fast, so validate format only. A dead-but-well-formed feed still
				// surfaces via the dispatched fetch.
				if !isValidFeedURL(candidate) {
					writeJSONStatus(w, http.StatusBadRequest, map[string]string{"message": "invalid feed URL"})
					return
				}
				// Guard against colliding with another of the user's
				// subscriptions (the (did, feed_url) pair is unique).
				if other, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
					Did:     didStr,
					FeedUrl: candidate,
				}); err == nil {
					if other.Rkey != rkey {
						http.Error(w, "already subscribed to that feed", http.StatusConflict)
						return
					}
				} else if !errors.Is(err, sql.ErrNoRows) {
					slog.Warn("/api/subscriptions PATCH: feed dedupe probe failed", "err", err)
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				newFeedURL = candidate
				feedChanged = true
				changed = true
			}
		}

		if !changed {
			// No diff — return the existing record without a PDS hit.
			writeJSON(w, patchResponse{SubscriptionWire: rowToWire(row)})
			return
		}

		if row.Kind == "standardfeed" {
			// Metadata rides the blue.morgen sidecar; the standard existence
			// record is never touched by edits, so no scope gate here — the
			// old blue.morgen grant suffices.
			now := time.Now().UTC().Format(time.RFC3339)
			record := map[string]any{
				"source":    sourceUnion(row.Kind, row.FeedUrl, ""),
				"createdAt": row.CreatedAt,
				"updatedAt": now,
			}
			if newTitle != nil {
				record["title"] = *newTitle
			}
			if newPrimary != 0 {
				record["primary"] = true
			}
			if len(newTagsSlice) > 0 {
				record["tags"] = newTagsSlice
			}
			var (
				sidecarRkey string
				err         error
			)
			if row.SidecarRkey != nil && *row.SidecarRkey != "" {
				sidecarRkey = *row.SidecarRkey
				_, err = pds.PutRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), sidecarRkey, record)
			} else {
				// First customization: create the sidecar lazily.
				var ref *atprepo.RecordRef
				ref, err = pds.CreateRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), record)
				if err == nil {
					sidecarRkey = atprepo.RkeyFromATURI(ref.URI)
				}
			}
			if err != nil {
				slog.Warn("/api/subscriptions PATCH: PDS sidecar write failed", "err", err)
				http.Error(w, "upstream PDS error", http.StatusBadGateway)
				return
			}
			if err := writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
				Did:         didStr,
				Rkey:        rkey,
				AtUri:       row.AtUri,
				FeedUrl:     row.FeedUrl,
				Kind:        row.Kind,
				SidecarRkey: &sidecarRkey,
				Title:       newTitle,
				IsPrimary:   newPrimary,
				Tags:        newTags,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   now,
			}); err != nil {
				slog.Warn("/api/subscriptions PATCH: Tier-1 upsert failed", "err", err)
			}
			row.Title = newTitle
			row.IsPrimary = newPrimary
			row.Tags = newTags
			row.SidecarRkey = &sidecarRkey
			row.UpdatedAt = now
			writeJSON(w, patchResponse{SubscriptionWire: rowToWire(row)})
			return
		}

		// Build the new record body. PDS putRecord replaces atomically, so the
		// feed's site URL must be re-attached or a metadata edit strips it.
		siteURL := ""
		if feed, err := reader.GetFeed(r.Context(), newFeedURL); err == nil {
			siteURL = derefStr(feed.SiteUrl)
		}
		record := map[string]any{
			"source":    sourceUnion(row.Kind, newFeedURL, siteURL),
			"createdAt": row.CreatedAt,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		if newTitle != nil {
			record["title"] = *newTitle
		}
		if newPrimary != 0 {
			record["primary"] = true
		}
		if len(newTagsSlice) > 0 {
			record["tags"] = newTagsSlice
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
		// Re-pointed feed: ensure the Tier-2 catalog row exists before the
		// Tier-1 row references it (feed_url FK), mirroring the POST contract.
		if feedChanged {
			if err := writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
				FeedUrl:   newFeedURL,
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				slog.Error("/api/subscriptions PATCH: Tier-2 upsert failed (PDS write already succeeded — next sync_user will reconcile)", "err", err)
			}
		}
		if err := writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
			Did:       didStr,
			Rkey:      rkey,
			AtUri:     ref.URI,
			FeedUrl:   newFeedURL,
			Title:     newTitle,
			IsPrimary: newPrimary,
			Tags:      newTags,
			CreatedAt: row.CreatedAt,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("/api/subscriptions PATCH: Tier-1 upsert failed", "err", err)
		}
		row.Title = newTitle
		row.IsPrimary = newPrimary
		row.Tags = newTags
		row.FeedUrl = newFeedURL
		row.UpdatedAt = now
		row.AtUri = ref.URI

		resp := patchResponse{SubscriptionWire: rowToWire(row)}
		if feedChanged {
			// Dispatch a fetch for the now-current feed; the id lets the client
			// poll /api/jobs/active and refresh once content lands.
			resp.JobID = disp.StartFetchOneFeed(sess.Data.AccountDID, newFeedURL)
		}
		writeJSON(w, resp)
	})
}

// RepoWriterLister is the PDS surface the DELETE handler needs: record
// writes plus the own-repo listing used to sweep duplicate standard records.
type RepoWriterLister interface {
	atprepo.Writer
	atprepo.Lister
}

// SubscriptionsDeleteHandler tombstones the PDS record(s) and removes the
// Tier-1 row. Tier-2 feeds row is left alone — other users may still subscribe.
// For standardfeed sources it deletes EVERY standard record matching the
// publication (other apps may have written duplicates; leaving one would
// resurrect the subscription on the next reconcile) plus the sidecar.
func SubscriptionsDeleteHandler(reader IndexRkeyReader, deleter IndexDeleter, pds RepoWriterLister) http.Handler {
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
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			slog.Warn("/api/subscriptions DELETE: load failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if row.Kind == "standardfeed" {
			if !requireStandardWrite(w, sess) {
				return
			}
			records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(standardfeed.CollectionSubscription))
			if err != nil {
				slog.Warn("/api/subscriptions DELETE: standard collection list failed", "err", err)
				http.Error(w, "upstream PDS error", http.StatusBadGateway)
				return
			}
			for _, rec := range records {
				publication, _ := rec.Value["publication"].(string)
				if publication != row.FeedUrl {
					continue
				}
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(standardfeed.CollectionSubscription), atprepo.RkeyFromATURI(rec.URI)); err != nil {
					slog.Warn("/api/subscriptions DELETE: standard record delete failed", "uri", rec.URI, "err", err)
					http.Error(w, "upstream PDS error", http.StatusBadGateway)
					return
				}
			}
			if row.SidecarRkey != nil && *row.SidecarRkey != "" {
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), *row.SidecarRkey); err != nil {
					slog.Warn("/api/subscriptions DELETE: sidecar delete failed", "err", err)
					http.Error(w, "upstream PDS error", http.StatusBadGateway)
					return
				}
			}
		} else {
			if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey); err != nil {
				slog.Warn("/api/subscriptions DELETE: PDS delete failed", "err", err)
				http.Error(w, "upstream PDS error", http.StatusBadGateway)
				return
			}
		}
		if err := deleter.DeleteUserSubscription(r.Context(), db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey}); err != nil {
			slog.Warn("/api/subscriptions DELETE: Tier-1 delete failed", "err", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// isValidFeedURL accepts only absolute http(s) URLs with a host. Format-only:
// it doesn't confirm the URL resolves to a real feed (that's the fetcher's job,
// on the safehttp client that blocks private/loopback targets).
func isValidFeedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

func changedString(old *string, next string) bool {
	if old == nil {
		return next != ""
	}
	return *old != next
}

// tagsEqual reports whether two tag slices are identical in content and order.
func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

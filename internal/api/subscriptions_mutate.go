package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
	"morgenblau/internal/tags"
)

// IndexRkeyReader adds the rkey lookup and GetFeed (for siteUrl preservation) atop IndexReader.
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

// patchResponse adds JobID so the client can poll the dispatched fetch after a feed URL change.
type patchResponse struct {
	SubscriptionWire
	JobID string `json:"jobId,omitempty"`
}

// SubscriptionsPatchHandler updates the subscription in place; a feedUrl change re-points to a new feed, mirroring the add path (Tier-2 upsert plus fetch dispatch).
func SubscriptionsPatchHandler(reader IndexRkeyReader, writer IndexWriter, pds atprepo.Writer, disp FetchDispatcher, memo DiscoverInvalidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "rkey is required")
			return
		}
		didStr := sess.Data.AccountDID.String()

		row, err := reader.GetUserSubscription(r.Context(), db.GetUserSubscriptionParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// 404 for both "doesn't exist" and "exists but isn't yours," to avoid leaking existence.
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/subscriptions PATCH: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		var body patchRequest
		if !decodeJSON(w, r, &body) {
			return
		}

		// A standardfeed source has no feed URL to re-point; its identity is the publication at-uri on the standard record.
		if row.Kind == "standardfeed" && body.FeedURL != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "feed URL cannot be changed for a publication source")
			return
		}

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

		newFeedURL := row.FeedUrl
		feedChanged := false
		if body.FeedURL != nil {
			if candidate := strings.TrimSpace(*body.FeedURL); candidate != "" && candidate != row.FeedUrl {
				// PATCH re-points to a client-computed URL (e.g. the Shorts toggle) and must stay fast, so this validates format only; a dead-but-well-formed feed surfaces via the dispatched fetch instead.
				if !isValidFeedURL(candidate) {
					writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid feed URL")
					return
				}
				// Guard against colliding with another of the user's subscriptions; (did, feed_url) is unique.
				if other, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
					Did:     didStr,
					FeedUrl: candidate,
				}); err == nil {
					if other.Rkey != rkey {
						writeError(w, http.StatusConflict, codeConflict, "already subscribed to that feed")
						return
					}
				} else if !errors.Is(err, sql.ErrNoRows) {
					slog.Warn("/api/subscriptions PATCH: feed dedupe probe failed", "err", err)
					writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
					return
				}
				newFeedURL = candidate
				feedChanged = true
				changed = true
			}
		}

		if !changed {
			// No diff: return the existing record without a PDS hit.
			writeJSON(w, patchResponse{SubscriptionWire: rowToWire(row)})
			return
		}

		if row.Kind == "standardfeed" {
			// Metadata rides the blue.morgen sidecar; the standard record is never touched, so no scope gate is needed here.
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
			if err := lexicon.ValidateRecord(subscriptionCollection, record); err != nil {
				slog.Warn("/api/subscriptions PATCH: sidecar record failed lexicon validation", "err", err)
				writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
				return
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
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return
			}
			mirrorOrRepair(r.Context(), disp, sess, "/api/subscriptions PATCH: standardfeed Tier-1 upsert", func() error {
				return writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
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
				})
			})
			row.Title = newTitle
			row.IsPrimary = newPrimary
			row.Tags = newTags
			row.SidecarRkey = &sidecarRkey
			row.UpdatedAt = now
			invalidateDiscover(memo, didStr)
			writeJSON(w, patchResponse{SubscriptionWire: rowToWire(row)})
			return
		}

		// PDS putRecord replaces atomically, so the feed's site URL must be re-attached or a metadata edit strips it.
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

		if err := lexicon.ValidateRecord(subscriptionCollection, record); err != nil {
			slog.Warn("/api/subscriptions PATCH: record failed lexicon validation", "err", err)
			writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
			return
		}
		ref, err := pds.PutRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey, record)
		if err != nil {
			slog.Warn("/api/subscriptions PATCH: PDS put failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		// Ensure the Tier-2 catalog row exists before Tier-1 references it (feed_url FK), mirroring the POST contract.
		if feedChanged {
			mirrorOrRepair(r.Context(), disp, sess, "/api/subscriptions PATCH: Tier-2 upsert", func() error {
				return writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
					FeedUrl:   newFeedURL,
					CreatedAt: now,
					UpdatedAt: now,
				})
			})
		}
		mirrorOrRepair(r.Context(), disp, sess, "/api/subscriptions PATCH: Tier-1 upsert", func() error {
			return writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
				Did:       didStr,
				Rkey:      rkey,
				AtUri:     ref.URI,
				FeedUrl:   newFeedURL,
				Title:     newTitle,
				IsPrimary: newPrimary,
				Tags:      newTags,
				CreatedAt: row.CreatedAt,
				UpdatedAt: now,
			})
		})
		row.Title = newTitle
		row.IsPrimary = newPrimary
		row.Tags = newTags
		row.FeedUrl = newFeedURL
		row.UpdatedAt = now
		row.AtUri = ref.URI
		invalidateDiscover(memo, didStr)

		resp := patchResponse{SubscriptionWire: rowToWire(row)}
		if feedChanged {
			// The job id lets the client poll /api/jobs/active and refresh once content lands.
			resp.JobID = disp.StartFetchOneFeed(sess.Data.AccountDID, newFeedURL)
		}
		writeJSON(w, resp)
	})
}

// RepoWriterLister is the PDS surface the DELETE handler needs: writes plus listing to sweep duplicate standard records.
type RepoWriterLister interface {
	atprepo.Writer
	atprepo.Lister
}

// SubscriptionsDeleteHandler tombstones the PDS record(s) and removes the Tier-1 row; the Tier-2 feeds row stays since other users may still subscribe.
// For standardfeed it also sweeps every duplicate standard record for the publication, since another app may have written one and leaving it would resurrect the subscription on reconcile.
func SubscriptionsDeleteHandler(reader IndexRkeyReader, deleter IndexDeleter, pds RepoWriterLister, disp RepairDispatcher, memo DiscoverInvalidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "rkey is required")
			return
		}
		didStr := sess.Data.AccountDID.String()
		row, err := reader.GetUserSubscription(r.Context(), db.GetUserSubscriptionParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/subscriptions DELETE: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		if row.Kind == "standardfeed" {
			if !requireStandardWrite(w, sess) {
				return
			}
			records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(standardfeed.CollectionSubscription))
			if err != nil {
				slog.Warn("/api/subscriptions DELETE: standard collection list failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return
			}
			for _, rec := range records {
				publication, _ := rec.Value["publication"].(string)
				if publication != row.FeedUrl {
					continue
				}
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(standardfeed.CollectionSubscription), atprepo.RkeyFromATURI(rec.URI)); err != nil {
					slog.Warn("/api/subscriptions DELETE: standard record delete failed", "uri", rec.URI, "err", err)
					writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
					return
				}
			}
			if row.SidecarRkey != nil && *row.SidecarRkey != "" {
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), *row.SidecarRkey); err != nil {
					slog.Warn("/api/subscriptions DELETE: sidecar delete failed", "err", err)
					writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
					return
				}
			}
		} else {
			if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey); err != nil {
				slog.Warn("/api/subscriptions DELETE: PDS delete failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return
			}
		}
		mirrorOrRepair(r.Context(), disp, sess, "/api/subscriptions DELETE: Tier-1 delete", func() error {
			return deleter.DeleteUserSubscription(r.Context(), db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey})
		})
		invalidateDiscover(memo, didStr)
		w.WriteHeader(http.StatusNoContent)
	})
}

// isValidFeedURL checks format only; resolution goes through the fetcher's safehttp client, which blocks private/loopback targets.
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

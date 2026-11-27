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
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/tags"
)

type IndexRkeyReader interface {
	IndexReader
	GetUserSubscription(ctx context.Context, arg db.GetUserSubscriptionParams) (db.UserSubscription, error)
	GetFeed(ctx context.Context, feedURL string) (db.Feed, error)
}

type IndexDeleter interface {
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
}

type patchRequest struct {
	Title   *string   `json:"title"`
	Primary *bool     `json:"primary"`
	Tags    *[]string `json:"tags"`
	FeedURL *string   `json:"feedUrl"`
}

type subscriptionResponse struct {
	SubscriptionWire
	JobID string `json:"jobId,omitempty"`
}

func SubscriptionsPatchHandler(reader IndexRkeyReader, writer IndexWriter, pds atprepo.Writer, disp FetchDispatcher) http.Handler {
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
			slog.Warn("/api/subscriptions PATCH: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		var body patchRequest
		if !decodeJSON(w, r, &body) {
			return
		}

		if row.Kind == "standardfeed" && body.FeedURL != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "feed URL cannot be changed for a publication source")
			return
		}

		changed := false
		newTitle := row.Title
		if body.Title != nil {
			if *body.Title != "" && strings.TrimSpace(*body.Title) == "" {
				writeError(w, http.StatusBadRequest, codeInvalidRequest, "title cannot be blank")
				return
			}
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
				if !isWellFormedFeedURL(candidate) {
					writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid feed URL")
					return
				}
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
				candidateSite := ""
				if feed, err := reader.GetFeed(r.Context(), candidate); err == nil {
					candidateSite = derefStr(feed.SiteUrl)
				}
				if rejectCrossKindSite(w, r, reader, didStr, "rss", rssSiblingKey(candidateSite, candidate), "/api/subscriptions PATCH") {
					return
				}
				newFeedURL = candidate
				feedChanged = true
				changed = true
			}
		}

		if !changed {
			writeJSON(w, subscriptionResponse{SubscriptionWire: rowToWire(row)})
			return
		}

		if row.Kind == "standardfeed" {
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
			existingSidecarRkey := ""
			if row.SidecarRkey != nil {
				existingSidecarRkey = *row.SidecarRkey
			}
			result, ok := commitThenMirror(r.Context(), disp, sess, "/api/subscriptions PATCH: standardfeed Tier-1 upsert", func() (sidecarWriteResult, bool) {
				return writeSidecarPair(r.Context(), w, sess, pds, sidecarWriteSpec{
					Sidecar:             record,
					SidecarCollection:   syntax.NSID(subscriptionCollection),
					ExistingSidecarRkey: existingSidecarRkey,
					SidecarOp:           "/api/subscriptions PATCH: PDS sidecar write failed",
				})
			}, func(result sidecarWriteResult) error {
				return writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
					Did:         didStr,
					Rkey:        rkey,
					AtUri:       row.AtUri,
					FeedUrl:     row.FeedUrl,
					Kind:        row.Kind,
					SidecarRkey: &result.SidecarRkey,
					Title:       newTitle,
					IsPrimary:   newPrimary,
					Tags:        newTags,
					CreatedAt:   row.CreatedAt,
					UpdatedAt:   now,
				})
			})
			if !ok {
				return
			}
			sidecarRkey := result.SidecarRkey
			row.Title = newTitle
			row.IsPrimary = newPrimary
			row.Tags = newTags
			row.SidecarRkey = &sidecarRkey
			row.UpdatedAt = now
			writeJSON(w, subscriptionResponse{SubscriptionWire: rowToWire(row)})
			return
		}

		siteURL := ""
		if feed, err := reader.GetFeed(r.Context(), newFeedURL); err == nil {
			siteURL = derefStr(feed.SiteUrl)
		}
		if channel := feedfinder.YouTubeChannelURL(newFeedURL); feedChanged && siteURL == "" && channel != "" && channel == feedfinder.YouTubeChannelURL(row.FeedUrl) {
			if feed, err := reader.GetFeed(r.Context(), row.FeedUrl); err == nil {
				siteURL = derefStr(feed.SiteUrl)
			}
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
		now := time.Now().UTC().Format(time.RFC3339)
		ref, ok := commitThenMirror(r.Context(), disp, sess, "/api/subscriptions PATCH: Tier-1 upsert", func() (*atprepo.RecordRef, bool) {
			ref, err := pds.PutRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey, record)
			if err != nil {
				slog.Warn("/api/subscriptions PATCH: PDS put failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return nil, false
			}
			return ref, true
		}, func(ref *atprepo.RecordRef) error {
			if feedChanged {
				if err := writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
					FeedUrl:   newFeedURL,
					SiteUrl:   nilIfEmpty(siteURL),
					CreatedAt: now,
					UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
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
		if !ok {
			return
		}
		row.Title = newTitle
		row.IsPrimary = newPrimary
		row.Tags = newTags
		row.FeedUrl = newFeedURL
		row.UpdatedAt = now
		row.AtUri = ref.URI
		resp := subscriptionResponse{SubscriptionWire: rowToWire(row)}
		if feedChanged {
			resp.JobID = disp.StartFetchOneFeed(sess.Data.AccountDID, newFeedURL)
		}
		writeJSON(w, resp)
	})
}

type RepoWriterLister interface {
	atprepo.Writer
	atprepo.Lister
}

func SubscriptionsDeleteHandler(reader IndexRkeyReader, deleter IndexDeleter, pds RepoWriterLister, disp RepairDispatcher) http.Handler {
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

		if row.Kind == "standardfeed" && !requireStandardWrite(w, sess) {
			return
		}
		if _, ok := commitThenMirror(r.Context(), disp, sess, "/api/subscriptions DELETE: Tier-1 delete", func() (struct{}, bool) {
			if row.Kind == "standardfeed" {
				return struct{}{}, deleteStandardSubscription(r.Context(), w, sess, pds, row.FeedUrl, derefStr(row.SidecarRkey))
			}
			if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), rkey); err != nil {
				slog.Warn("/api/subscriptions DELETE: PDS delete failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return struct{}{}, false
			}
			return struct{}{}, true
		}, func(struct{}) error {
			return deleter.DeleteUserSubscription(r.Context(), db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey})
		}); !ok {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func isWellFormedFeedURL(raw string) bool {
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

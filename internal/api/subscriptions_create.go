package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
	"morgenblau/internal/tags"
)

// --- POST /api/subscriptions ---

// addRequest is one source, so a failure never follows a committed sibling: feedUrl (rss) or publication (standardfeed); for standardfeed, empty title/primary/tags mean no sidecar.
type addRequest struct {
	FeedURL     string   `json:"feedUrl"`
	Publication string   `json:"publication"`
	Title       string   `json:"title"`
	SiteURL     string   `json:"siteUrl"`
	Primary     bool     `json:"primary"`
	Tags        []string `json:"tags"`
}

// IndexReader is the read slice of *db.Queries, narrow enough for handler tests to stub without sqlite.
type IndexReader interface {
	ListUserSubscriptions(ctx context.Context, did string) ([]db.UserSubscription, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
	ListUserSubscriptionsWithSiteURL(ctx context.Context, did string) ([]db.ListUserSubscriptionsWithSiteURLRow, error)
}

// IndexWriter is the write slice, kept distinct from IndexReader so the GET handler can depend on a narrower interface.
type IndexWriter interface {
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
}

// FetchDispatcher dispatches fetch_one_feed per subscription, or a sync_user reconcile when local writes diverge from PDS (the source of truth); the returned id lets the client's RefreshPill poll.
type FetchDispatcher interface {
	RepairDispatcher
	StartFetchOneFeed(did syntax.DID, feedURL string) string
}

// SubscriptionsCreateHandler dedupes, makes one PDS commit, mirrors Tier-2 then Tier-1, then dispatches fetch_one_feed.
func SubscriptionsCreateHandler(
	reader IndexReader,
	writer IndexWriter,
	pds atprepo.Writer,
	disp FetchDispatcher,
) http.Handler {
	clock := syntax.NewTIDClock(uint(rand.IntN(1024)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var item addRequest
		if !decodeJSON(w, r, &item) {
			return
		}
		item.FeedURL = strings.TrimSpace(item.FeedURL)
		item.Publication = strings.TrimSpace(item.Publication)
		switch {
		case item.FeedURL == "" && item.Publication == "":
			writeFieldErrors(w, map[string]string{"feedUrl": "Feed URL is required"})
			return
		case item.FeedURL != "" && item.Publication != "":
			writeFieldErrors(w, map[string]string{"publication": "feedUrl and publication are mutually exclusive"})
			return
		case item.Publication != "":
			if _, err := syntax.ParseATURI(item.Publication); err != nil {
				writeFieldErrors(w, map[string]string{"publication": "publication must be an at:// URI"})
				return
			}
		}
		isStandard := item.Publication != ""
		if isStandard && !requireStandardWrite(w, sess) {
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		didStr := sess.Data.AccountDID.String()
		// The catalog key (feed URL for rss, publication at-uri for standardfeed) keys Tier-2, Tier-1, dedupe, and the fetch job.
		key := item.FeedURL
		kind := "rss"
		if isStandard {
			key = item.Publication
			kind = "standardfeed"
		}

		// Step 1: dedupe guard.
		if row, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
			Did:     didStr,
			FeedUrl: key,
		}); err == nil {
			writeJSON(w, subscriptionResponse{SubscriptionWire: rowToWire(row)})
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("/api/subscriptions: dedupe probe failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		siteKey := rssSiblingKey(item.SiteURL, key)
		if isStandard {
			siteKey = siblingKey(item.SiteURL)
		}
		if rejectCrossKindSite(w, r, reader, didStr, kind, siteKey, "/api/subscriptions") {
			return
		}

		// Step 2: validate, then the request's one PDS commit; an already present pair commits nothing and is only mirrored.
		tagList := normalizeTags(item.Tags)
		var (
			commit func() (createdSubscription, bool)
			source map[string]any
		)
		if isStandard {
			source = sourceUnion(kind, key, "")
			customized := item.Title != "" || item.Primary || len(tagList) > 0
			var sidecar map[string]any
			if customized {
				sidecar = standardSidecar(source, now, item, tagList)
				if err := lexicon.ValidateRecord(subscriptionCollection, sidecar); err != nil {
					slog.Warn("/api/subscriptions: sidecar failed lexicon validation", "err", err)
					writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
					return
				}
			}
			standardRecord, sidecarRecord, ok := preflightStandardSubscription(r.Context(), w, sess, pds, key)
			if !ok {
				return
			}
			commit = func() (createdSubscription, bool) {
				var out createdSubscription
				if standardRecord != nil {
					out.ref = &atprepo.RecordRef{URI: standardRecord.URI, CID: standardRecord.CID}
					if sidecarRecord != nil {
						out.sidecarRkey = stringPtr(atprepo.RkeyFromATURI(sidecarRecord.URI))
						item.Title, item.Primary, tagList = sidecarMetadata(sidecarRecord.Value)
						return out, true
					}
					if !customized {
						return out, true
					}
					result, ok := writeSidecarPair(r.Context(), w, sess, pds, sidecarWriteSpec{
						Sidecar:           sidecar,
						SidecarCollection: syntax.NSID(subscriptionCollection),
						SidecarCreateRkey: syntax.RecordKey(clock.Next().String()),
						SidecarOp:         "/api/subscriptions: standard sidecar create failed",
					})
					out.sidecarRkey = &result.SidecarRkey
					return out, ok
				}
				// The existence record is the portable standard subscription. A customized pair is one PDS commit.
				spec := sidecarWriteSpec{
					Existence:           map[string]any{"publication": key, "createdAt": now},
					ExistenceCollection: syntax.NSID(standardfeed.CollectionSubscription),
					ExistenceRkey:       syntax.RecordKey(clock.Next().String()),
					ExistenceOp:         "/api/subscriptions: standard record create failed",
				}
				if customized {
					spec.Sidecar = sidecar
					spec.SidecarCollection = syntax.NSID(subscriptionCollection)
					spec.SidecarCreateRkey = syntax.RecordKey(clock.Next().String())
					spec.SidecarOp = "/api/subscriptions: atomic standard subscription and sidecar write failed"
				}
				result, ok := writeSidecarPair(r.Context(), w, sess, pds, spec)
				out.ref = result.ExistenceRef
				if result.SidecarRkey != "" {
					out.sidecarRkey = &result.SidecarRkey
				}
				return out, ok
			}
		} else {
			// Title was resolver-prefilled client-side; the user may have overridden it before submit.
			source = sourceUnion(kind, key, item.SiteURL)
			record := map[string]any{
				"source":    source,
				"createdAt": now,
			}
			if item.Title != "" {
				record["title"] = item.Title
			}
			if item.Primary {
				record["primary"] = true
			}
			if len(tagList) > 0 {
				record["tags"] = tagList
			}
			if err := lexicon.ValidateRecord(subscriptionCollection, record); err != nil {
				slog.Warn("/api/subscriptions: record failed lexicon validation", "err", err)
				writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
				return
			}
			commit = func() (createdSubscription, bool) {
				ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), record)
				if err != nil {
					slog.Warn("/api/subscriptions: PDS create failed", "err", err)
					writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
					return createdSubscription{}, false
				}
				return createdSubscription{ref: ref}, true
			}
		}

		// Step 3: Tier-2 catalog upsert, then Tier-1. Title stays nil on Tier-2; feeds.title is the cached publication name, owned by the fetch pipeline.
		created, ok := commitThenMirror(r.Context(), disp, sess, "/api/subscriptions: mirror", commit, func(created createdSubscription) error {
			if err := writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
				FeedUrl:   key,
				Kind:      kind,
				SiteUrl:   nilIfEmpty(item.SiteURL),
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				return err
			}
			return writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
				Did:         didStr,
				Rkey:        atprepo.RkeyFromATURI(created.ref.URI),
				AtUri:       created.ref.URI,
				FeedUrl:     key,
				Kind:        kind,
				SidecarRkey: created.sidecarRkey,
				Title:       nilIfEmpty(item.Title),
				IsPrimary:   boolToInt64(item.Primary),
				Tags:        tags.Marshal(tagList),
				CreatedAt:   now,
				UpdatedAt:   now,
			})
		})
		if !ok {
			return
		}

		value := map[string]any{
			"source":    source,
			"createdAt": now,
		}
		if item.Title != "" {
			value["title"] = item.Title
		}
		if item.Primary {
			value["primary"] = true
		}
		if len(tagList) > 0 {
			value["tags"] = tagList
		}
		wire := SubscriptionWire{
			URI:     created.ref.URI,
			CID:     created.ref.CID,
			Rkey:    atprepo.RkeyFromATURI(created.ref.URI),
			Kind:    kind,
			FeedURL: key,
			Title:   item.Title,
			SiteURL: item.SiteURL,
			Primary: item.Primary,
			Tags:    tagList,
			Value:   value,
		}
		if isStandard {
			wire.Publication = key
		}

		// Step 4: dispatch fetch_one_feed (async).
		writeJSON(w, subscriptionResponse{SubscriptionWire: wire, JobID: disp.StartFetchOneFeed(sess.Data.AccountDID, key)})
	})
}

// createdSubscription is what the create commit leaves on the PDS, for the mirror and the response.
type createdSubscription struct {
	ref         *atprepo.RecordRef
	sidecarRkey *string
}

func preflightStandardSubscription(ctx context.Context, w http.ResponseWriter, sess *session.Session, pds atprepo.Writer, publication string) (*atprepo.ListedRecord, *atprepo.ListedRecord, bool) {
	lister, ok := pds.(atprepo.Lister)
	if !ok {
		slog.Warn("/api/subscriptions: PDS writer cannot preflight Standardfeed records")
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return nil, nil, false
	}
	standard, err := lister.ListRecords(ctx, sess, syntax.NSID(standardfeed.CollectionSubscription))
	if err != nil {
		slog.Warn("/api/subscriptions: standard record preflight failed", "err", err)
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return nil, nil, false
	}
	sidecars, err := lister.ListRecords(ctx, sess, syntax.NSID(subscriptionCollection))
	if err != nil {
		slog.Warn("/api/subscriptions: sidecar preflight failed", "err", err)
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return nil, nil, false
	}
	var existence *atprepo.ListedRecord
	for i := range standard {
		if standard[i].Value["publication"] != publication {
			continue
		}
		rkey := atprepo.RkeyFromATURI(standard[i].URI)
		if rkey == "" {
			slog.Warn("/api/subscriptions: matching standard record had an invalid at-uri", "uri", standard[i].URI)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return nil, nil, false
		}
		if existence == nil || rkey < atprepo.RkeyFromATURI(existence.URI) {
			existence = &standard[i]
		}
	}
	var sidecar *atprepo.ListedRecord
	for i := range sidecars {
		source, ok := sidecars[i].Value["source"].(map[string]any)
		if !ok || source["$type"] != lexicon.SourceStandard || source["publication"] != publication {
			continue
		}
		rkey := atprepo.RkeyFromATURI(sidecars[i].URI)
		if rkey == "" {
			slog.Warn("/api/subscriptions: matching sidecar had an invalid at-uri", "uri", sidecars[i].URI)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return nil, nil, false
		}
		if sidecar == nil || rkey > atprepo.RkeyFromATURI(sidecar.URI) {
			sidecar = &sidecars[i]
		}
	}
	return existence, sidecar, true
}

func standardSidecar(source map[string]any, createdAt string, item addRequest, tagList []string) map[string]any {
	record := map[string]any{"source": source, "createdAt": createdAt}
	if item.Title != "" {
		record["title"] = item.Title
	}
	if item.Primary {
		record["primary"] = true
	}
	if len(tagList) > 0 {
		record["tags"] = tagList
	}
	return record
}

func sidecarMetadata(record map[string]any) (string, bool, []string) {
	title, _ := record["title"].(string)
	primary, _ := record["primary"].(bool)
	var rawTags []string
	switch values := record["tags"].(type) {
	case []string:
		rawTags = values
	case []any:
		for _, value := range values {
			if tag, ok := value.(string); ok {
				rawTags = append(rawTags, tag)
			}
		}
	}
	return title, primary, normalizeTags(rawTags)
}

func stringPtr(value string) *string { return &value }

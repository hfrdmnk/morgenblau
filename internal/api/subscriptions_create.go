package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/standardfeed"
	"morgenblau/internal/tags"
)

// --- POST /api/subscriptions ---

type addRequest struct {
	Subscriptions []addItem `json:"subscriptions"`
}

// addItem carries either feedUrl (rss) or publication (standardfeed), exactly one; for standardfeed, empty title/primary/tags mean no sidecar record.
type addItem struct {
	FeedURL     string   `json:"feedUrl"`
	Publication string   `json:"publication"`
	Title       string   `json:"title"`
	SiteURL     string   `json:"siteUrl"`
	Primary     bool     `json:"primary"`
	Tags        []string `json:"tags"`
}

type addResponse struct {
	Records []SubscriptionWire `json:"records"`
	JobIDs  []string           `json:"jobIds"`
}

// IndexReader is the read slice of *db.Queries, narrow enough for handler tests to stub without sqlite.
type IndexReader interface {
	ListUserSubscriptions(ctx context.Context, did string) ([]db.UserSubscription, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
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

// SubscriptionsCreateHandler dedupes, writes to PDS, upserts Tier-2 then Tier-1, then dispatches fetch_one_feed.
func SubscriptionsCreateHandler(
	reader IndexReader,
	writer IndexWriter,
	pds atprepo.Writer,
	disp FetchDispatcher,
	memo DiscoverInvalidator,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body addRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		fieldErrors := make(map[string]string)
		hasStandard := false
		for i, item := range body.Subscriptions {
			feedURL := strings.TrimSpace(item.FeedURL)
			publication := strings.TrimSpace(item.Publication)
			switch {
			case feedURL == "" && publication == "":
				fieldErrors["subscriptions."+strconv.Itoa(i)+".feedUrl"] = "Feed URL is required"
			case feedURL != "" && publication != "":
				fieldErrors["subscriptions."+strconv.Itoa(i)+".publication"] = "feedUrl and publication are mutually exclusive"
			case publication != "":
				if _, err := syntax.ParseATURI(publication); err != nil {
					fieldErrors["subscriptions."+strconv.Itoa(i)+".publication"] = "publication must be an at:// URI"
				}
				hasStandard = true
			}
		}
		if len(fieldErrors) > 0 {
			writeFieldErrors(w, fieldErrors)
			return
		}
		if len(body.Subscriptions) == 0 {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "no subscriptions submitted")
			return
		}
		// Gate the whole batch before any write: a mid-batch 403 would leave earlier items created and later ones dropped.
		if hasStandard && !requireStandardWrite(w, sess) {
			return
		}
		// Belt and suspenders: an rss feed and a publication for the same site in one batch is the double-subscribe the picker already guards against.
		kindByKey := make(map[string]string, len(body.Subscriptions))
		for _, item := range body.Subscriptions {
			var key, kind string
			if strings.TrimSpace(item.Publication) != "" {
				key, kind = siblingKey(item.SiteURL), "standardfeed"
			} else {
				key, kind = rssSiblingKey(item.SiteURL, item.FeedURL), "rss"
			}
			if key == "" {
				continue
			}
			if prev, ok := kindByKey[key]; ok && prev != kind {
				writeError(w, http.StatusConflict, codeConflict, "Pick either the RSS feed or the ATProto publication for a site, not both")
				return
			}
			kindByKey[key] = kind
		}

		out := addResponse{Records: make([]SubscriptionWire, 0, len(body.Subscriptions)), JobIDs: []string{}}
		now := time.Now().UTC().Format(time.RFC3339)
		didStr := sess.Data.AccountDID.String()

		for _, item := range body.Subscriptions {
			item.FeedURL = strings.TrimSpace(item.FeedURL)
			item.Publication = strings.TrimSpace(item.Publication)
			isStandard := item.Publication != ""
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
				out.Records = append(out.Records, rowToWire(row))
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				slog.Warn("/api/subscriptions: dedupe probe failed", "err", err)
				writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
				return
			}

			// Step 2: PDS write(s).
			tagList := normalizeTags(item.Tags)
			var (
				ref         *atprepo.RecordRef
				sidecarRkey *string
				source      map[string]any
			)
			if isStandard {
				source = sourceUnion(kind, key, "")
				// The existence record is the portable standard subscription.
				spec := sidecarWriteSpec{
					Existence:           map[string]any{"publication": key, "createdAt": now},
					ExistenceCollection: syntax.NSID(standardfeed.CollectionSubscription),
					ExistenceOp:         "/api/subscriptions: standard record create failed",
				}
				// Lazy blue.morgen sidecar, only when metadata was customized; written second so a failure still leaves an adoptable standard record.
				if item.Title != "" || item.Primary || len(tagList) > 0 {
					sidecar := map[string]any{
						"source":    source,
						"createdAt": now,
					}
					if item.Title != "" {
						sidecar["title"] = item.Title
					}
					if item.Primary {
						sidecar["primary"] = true
					}
					if len(tagList) > 0 {
						sidecar["tags"] = tagList
					}
					spec.Sidecar = sidecar
					spec.SidecarCollection = syntax.NSID(subscriptionCollection)
					spec.SidecarOp = "/api/subscriptions: sidecar create failed (standard record already on PDS; reconcile will adopt it)"
				}
				result, ok := writeSidecarPair(r.Context(), w, sess, pds, spec)
				if !ok {
					return
				}
				ref = result.ExistenceRef
				if result.SidecarRkey != "" {
					sidecarRkey = &result.SidecarRkey
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
				var err error
				ref, err = pds.CreateRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), record)
				if err != nil {
					slog.Warn("/api/subscriptions: PDS create failed", "err", err)
					writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
					return
				}
			}
			rkey := atprepo.RkeyFromATURI(ref.URI)

			// Step 3: Tier-2 catalog upsert. Title stays nil; feeds.title is the cached publication name, owned by the fetch pipeline.
			titlePtr := nilIfEmpty(item.Title)
			siteURLPtr := nilIfEmpty(item.SiteURL)
			mirrorOrRepair(r.Context(), disp, sess, "/api/subscriptions: Tier-2 upsert", func() error {
				return writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
					FeedUrl:   key,
					Kind:      kind,
					SiteUrl:   siteURLPtr,
					CreatedAt: now,
					UpdatedAt: now,
				})
			})

			// Step 4: Tier-1 index upsert.
			mirrorOrRepair(r.Context(), disp, sess, "/api/subscriptions: Tier-1 upsert", func() error {
				return writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
					Did:         didStr,
					Rkey:        rkey,
					AtUri:       ref.URI,
					FeedUrl:     key,
					Kind:        kind,
					SidecarRkey: sidecarRkey,
					Title:       titlePtr,
					IsPrimary:   boolToInt64(item.Primary),
					Tags:        tags.Marshal(tagList),
					CreatedAt:   now,
					UpdatedAt:   now,
				})
			})

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
				URI:     ref.URI,
				CID:     ref.CID,
				Rkey:    rkey,
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
			out.Records = append(out.Records, wire)

			// Step 5: dispatch fetch_one_feed (async).
			jobID := disp.StartFetchOneFeed(sess.Data.AccountDID, key)
			out.JobIDs = append(out.JobIDs, jobID)
		}

		// A batch of pure dedupe hits changed nothing the suggestion pool reads.
		if len(out.JobIDs) > 0 {
			invalidateDiscover(memo, didStr)
		}
		writeJSON(w, out)
	})
}

package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
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
) http.Handler {
	clock := syntax.NewTIDClock(uint(rand.IntN(1024)))
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
				if standardRecord != nil {
					ref = &atprepo.RecordRef{URI: standardRecord.URI, CID: standardRecord.CID}
					if sidecarRecord != nil {
						sidecarRkey = stringPtr(atprepo.RkeyFromATURI(sidecarRecord.URI))
						item.Title, item.Primary, tagList = sidecarMetadata(sidecarRecord.Value)
					} else if customized {
						spec := sidecarWriteSpec{
							Sidecar:           sidecar,
							SidecarCollection: syntax.NSID(subscriptionCollection),
							SidecarCreateRkey: syntax.RecordKey(clock.Next().String()),
							SidecarOp:         "/api/subscriptions: standard sidecar create failed",
						}
						result, ok := writeSidecarPair(r.Context(), w, sess, pds, spec)
						if !ok {
							return
						}
						sidecarRkey = &result.SidecarRkey
					}
				} else {
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
					if !ok {
						return
					}
					ref = result.ExistenceRef
					if result.SidecarRkey != "" {
						sidecarRkey = &result.SidecarRkey
					}
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

		writeJSON(w, out)
	})
}

func preflightStandardSubscription(ctx context.Context, w http.ResponseWriter, sess *oauth.ClientSession, pds atprepo.Writer, publication string) (*atprepo.ListedRecord, *atprepo.ListedRecord, bool) {
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
		if !ok || source["$type"] != "blue.morgen.feed.subscription#standardPublication" || source["publication"] != publication {
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

func standardSidecar(source map[string]any, createdAt string, item addItem, tagList []string) map[string]any {
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

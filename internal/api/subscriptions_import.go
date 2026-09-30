package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/session"
	"morgenblau/internal/tags"
)

func SubscriptionsImportPrepareHandler(pds atprepo.Lister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body struct {
			Provider string `json:"provider"`
			OPML     string `json:"opml"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Provider == "opml" {
			plan, err := parseOPML(body.OPML)
			if err != nil {
				writeError(w, 400, codeInvalidRequest, err.Error())
				return
			}
			writeJSON(w, plan)
			return
		}
		collection := ""
		switch body.Provider {
		case "skyreader":
			collection = "app.skyreader.feed.subscription"
		case "glean":
			collection = "at.glean.subscription"
		default:
			writeError(w, 400, codeInvalidRequest, "Choose OPML, Skyreader, or Glean")
			return
		}
		records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(collection))
		if err != nil {
			writeError(w, 502, codeUpstreamError, "Could not read subscriptions from your PDS")
			return
		}
		var sources []importSource
		var warnings []string
		for _, record := range records {
			v := record.Value
			if body.Provider == "skyreader" {
				if kind := recordString(v, "sourceType"); kind != "" && kind != "rss" {
					warnings = append(warnings, "Skipped a Skyreader source that is not an RSS or Atom feed")
					continue
				}
			}
			s := importSource{FeedURL: recordString(v, "feedUrl"), Title: recordString(v, "title"), Tags: []string{recordString(v, "category")}}
			if body.Provider == "skyreader" {
				s.SiteURL = recordString(v, "siteUrl")
				if title := recordString(v, "customTitle"); title != "" {
					s.Title = title
				}
				s.Tags = append(s.Tags, recordTags(v)...)
			}
			sources = append(sources, s)
		}
		plan := prepareImportSources(sources)
		plan.Warnings = append(plan.Warnings, warnings...)
		writeJSON(w, plan)
	})
}

type importFailure struct {
	FeedURL string `json:"feedUrl"`
	Message string `json:"message"`
}

type importResult struct {
	Added     int             `json:"added"`
	Updated   int             `json:"updated"`
	Unchanged int             `json:"unchanged"`
	Failures  []importFailure `json:"failures"`
}

type ImportRepo interface {
	atprepo.Lister
	GetLatestCommit(context.Context, *session.Session) (string, error)
	CreateRecordIfCommit(context.Context, *session.Session, syntax.NSID, map[string]any, string) (*atprepo.RecordRef, error)
	PutRecordIfCID(context.Context, *session.Session, syntax.NSID, string, map[string]any, string) (*atprepo.RecordRef, error)
}

type ImportIndex interface {
	SnapshotImportedSubscription(context.Context, string, string) (*db.UserSubscription, error)
	MirrorImportedSubscription(context.Context, *db.UserSubscription, db.UpsertFeedParams, db.UpsertUserSubscriptionParams) error
}

type importSnapshot struct {
	head   string
	record atprepo.ListedRecord
	found  bool
	local  *db.UserSubscription
}

func SubscriptionsImportHandler(writer ImportIndex, pds ImportRepo, disp FetchDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body struct {
			Sources []importSource `json:"sources"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		// Small batches leave time for the response within the server's 30-second write deadline.
		if len(body.Sources) == 0 || len(body.Sources) > 5 {
			writeError(w, 400, codeInvalidRequest, "Import between 1 and 5 sources per batch")
			return
		}
		plan := prepareImportSources(body.Sources)
		if len(plan.Warnings) != 0 {
			writeError(w, 400, codeInvalidRequest, strings.Join(plan.Warnings, "; "))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		result := importResult{Failures: []importFailure{}}
		for _, source := range plan.Sources {
			status, err := importSubscription(ctx, sess, source, writer, pds, disp)
			if err != nil {
				result.Failures = append(result.Failures, importFailure{FeedURL: source.FeedURL, Message: err.Error()})
				continue
			}
			switch status {
			case "added":
				result.Added++
			case "updated":
				result.Updated++
			case "unchanged":
				result.Unchanged++
			}
		}
		writeJSON(w, result)
	})
}

func importSubscription(ctx context.Context, sess *session.Session, incoming importSource, writer ImportIndex, pds ImportRepo, disp FetchDispatcher) (string, error) {
	for range 3 {
		baseline, err := writer.SnapshotImportedSubscription(ctx, sess.Data.AccountDID.String(), incoming.FeedURL)
		if err != nil {
			return "", fmt.Errorf("Could not read the local subscription index. Please retry.")
		}
		// Capturing the head before listing makes absence checks safe against competing or delayed creates.
		head, err := pds.GetLatestCommit(ctx, sess)
		if err != nil {
			return "", fmt.Errorf("Could not read your PDS repository head. Please retry.")
		}
		records, err := pds.ListRecords(ctx, sess, syntax.NSID(subscriptionCollection))
		if err != nil {
			return "", fmt.Errorf("Could not read existing subscriptions from your PDS. Please retry.")
		}
		old, found, err := existingImportSubscription(records, incoming.FeedURL)
		if err != nil {
			return "", err
		}
		status, err := writeImportSubscription(ctx, sess, incoming, importSnapshot{head: head, record: old, found: found, local: baseline}, writer, pds, disp)
		if !isImportConflict(err) {
			return status, err
		}
	}
	return "", fmt.Errorf("Your subscriptions changed during import. Retry to merge with the latest records.")
}

func isImportConflict(err error) bool {
	var apiErr *atclient.APIError
	return errors.As(err, &apiErr) && apiErr.Name == "InvalidSwap"
}

func existingImportSubscription(records []atprepo.ListedRecord, feedURL string) (atprepo.ListedRecord, bool, error) {
	var chosen atprepo.ListedRecord
	found, invalid := false, false
	for _, record := range records {
		s, ok := rssImportSource(record.Value)
		if !ok || s.FeedURL != feedURL {
			continue
		}
		if err := lexicon.ValidateRecordLenient(subscriptionCollection, record.Value); err != nil {
			invalid = true
			continue
		}
		if !found || record.URI < chosen.URI {
			chosen, found = record, true
		}
	}
	if !found && invalid {
		return chosen, false, fmt.Errorf("An existing subscription has invalid metadata. Repair or remove it in your PDS before importing this source.")
	}
	return chosen, found, nil
}

func writeImportSubscription(ctx context.Context, sess *session.Session, incoming importSource, snapshot importSnapshot, writer ImportIndex, pds ImportRepo, disp FetchDispatcher) (string, error) {
	old, found := snapshot.record, snapshot.found
	now := time.Now().UTC().Format(time.RFC3339)
	record := importRecord(incoming, now)
	status := "added"
	ref := &atprepo.RecordRef{URI: old.URI, CID: old.CID}
	if found {
		merged, err := mergeImportTags(recordTags(old.Value), incoming.Tags)
		if err != nil {
			return "", err
		}
		record = maps.Clone(old.Value)
		status = "unchanged"
		if !slices.Equal(merged, recordTags(old.Value)) {
			record["tags"] = merged
			status = "updated"
		}
	}
	if status != "unchanged" {
		if err := lexicon.ValidateRecord(subscriptionCollection, record); err != nil {
			return "", fmt.Errorf("Source metadata exceeds the subscription limits")
		}
		var err error
		if found {
			ref, err = pds.PutRecordIfCID(ctx, sess, syntax.NSID(subscriptionCollection), atprepo.RkeyFromATURI(old.URI), record, old.CID)
		} else {
			ref, err = pds.CreateRecordIfCommit(ctx, sess, syntax.NSID(subscriptionCollection), record, snapshot.head)
		}
		if err != nil {
			if isImportConflict(err) {
				return "", err
			}
			slog.Warn("subscription import: PDS write failed", "err", err)
			return "", fmt.Errorf("Could not confirm the PDS write. Retry safely to check or finish this source.")
		}
	}
	source, _ := rssImportSource(record)
	primary, _ := record["primary"].(bool)
	mirrorOrRepair(ctx, disp, sess, "subscription import: mirror", func() error {
		return writer.MirrorImportedSubscription(ctx, snapshot.local,
			db.UpsertFeedParams{FeedUrl: source.FeedURL, Kind: "rss", SiteUrl: nilIfEmpty(source.SiteURL), CreatedAt: now, UpdatedAt: now},
			db.UpsertUserSubscriptionParams{
				Did: sess.Data.AccountDID.String(), Rkey: atprepo.RkeyFromATURI(ref.URI), AtUri: ref.URI,
				FeedUrl: source.FeedURL, Kind: "rss", Title: nilIfEmpty(source.Title), IsPrimary: boolToInt64(primary), Tags: tags.Marshal(source.Tags), CreatedAt: recordString(record, "createdAt"), UpdatedAt: now,
			})
	})
	// A previous create may have committed without reaching its fetch dispatch.
	disp.StartFetchOneFeed(sess.Data.AccountDID, source.FeedURL)
	return status, nil
}

func SubscriptionsExportHandler(pds atprepo.Lister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(subscriptionCollection))
		if err != nil {
			writeError(w, 502, codeUpstreamError, "Could not read subscriptions from your PDS")
			return
		}
		var sources []importSource
		for _, record := range records {
			if source, ok := rssImportSource(record.Value); ok {
				sources = append(sources, source)
			}
		}
		opml, err := encodeOPML(sources)
		if err != nil {
			writeError(w, 500, codeInternalError, "Could not create the OPML file")
			return
		}
		writeJSON(w, map[string]string{"opml": opml})
	})
}

func rssImportSource(record map[string]any) (importSource, bool) {
	source, ok := record["source"].(map[string]any)
	if !ok || source["$type"] != sourceTypeRSS {
		return importSource{}, false
	}
	return importSource{FeedURL: recordString(source, "feedUrl"), SiteURL: recordString(source, "siteUrl"), Title: recordString(record, "title"), Tags: recordTags(record)}, true
}

func recordString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func recordTags(record map[string]any) []string {
	switch values := record["tags"].(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if tag, ok := value.(string); ok {
				out = append(out, tag)
			}
		}
		return out
	default:
		return nil
	}
}

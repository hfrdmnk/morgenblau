package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/middleware/auth"
)

const subscriptionCollection = "app.skyreader.feed.subscription"

// SubscriptionWire is the on-the-wire shape returned by GET / POST.
type SubscriptionWire struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid,omitempty"`
	Value map[string]any `json:"value"`
	// Embedded sugar for the frontend so callers don't dig into Value.
	Rkey    string `json:"rkey"`
	FeedURL string `json:"feedUrl"`
	Title   string `json:"title,omitempty"`
	SiteURL string `json:"siteUrl,omitempty"`
}

// IndexReader is the slice of *db.Queries we use for reads. Defined so handler
// tests can stub the database without spinning up sqlite.
type IndexReader interface {
	ListUserSubscriptions(ctx context.Context, did string) ([]db.UserSubscription, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
}

// IndexWriter is the slice used for writes. Same store, distinct interface so
// the GET handler can take a narrower dependency.
type IndexWriter interface {
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
}

// FeedFinder is the slice of *feedfinder.Finder we depend on.
type FeedFinder interface {
	Resolve(ctx context.Context, url string) ([]feedfinder.Candidate, error)
}

// FetchDispatcher fires off a fetch_one_feed job per added subscription. The
// returned id surfaces back to the client so the RefreshPill can poll.
type FetchDispatcher interface {
	StartFetchOneFeed(ctx context.Context, did syntax.DID, feedURL string) string
}

// SubscriptionsListHandler returns the user's Tier-1 index entries as a
// flat list matching the legacy PDS-pass-through shape — frontend code
// doesn't need to change shape, only source.
func SubscriptionsListHandler(reader IndexReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rows, err := reader.ListUserSubscriptions(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/subscriptions: list failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]SubscriptionWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, rowToWire(row))
		}
		writeJSON(w, out)
	})
}

func rowToWire(row db.UserSubscription) SubscriptionWire {
	value := map[string]any{
		"feedUrl": row.FeedUrl,
	}
	title := ""
	if row.Title != nil {
		value["title"] = *row.Title
		title = *row.Title
	}
	if row.CustomTitle != nil {
		value["customTitle"] = *row.CustomTitle
	}
	return SubscriptionWire{
		URI:     row.AtUri,
		Value:   value,
		Rkey:    row.Rkey,
		FeedURL: row.FeedUrl,
		Title:   title,
	}
}

// --- POST /api/subscriptions/resolve ---

type resolveRequest struct {
	URL string `json:"url"`
}

type resolveResponse struct {
	Candidates            []feedfinder.Candidate `json:"candidates"`
	ExistingSubscriptions []existingSubMeta      `json:"existingSubscriptions"`
}

type existingSubMeta struct {
	FeedURL string  `json:"feedUrl"`
	Title   *string `json:"title"`
}

// SubscriptionsResolveHandler turns a pasted URL into feed candidates and
// flags any candidates that the user is already subscribed to.
func SubscriptionsResolveHandler(reader IndexReader, finder FeedFinder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var body resolveRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		body.URL = strings.TrimSpace(body.URL)
		if body.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}

		cands, err := finder.Resolve(r.Context(), body.URL)
		if err != nil {
			slog.Warn("/api/subscriptions/resolve: finder failed", "url", body.URL, "err", err)
			writeJSONStatus(w, http.StatusBadGateway, map[string]string{"message": "Couldn't reach that URL"})
			return
		}

		existing := make([]existingSubMeta, 0)
		for _, c := range cands {
			row, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
				Did:     sess.Data.AccountDID.String(),
				FeedUrl: c.FeedURL,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				slog.Warn("/api/subscriptions/resolve: index probe failed", "err", err)
				continue
			}
			existing = append(existing, existingSubMeta{FeedURL: row.FeedUrl, Title: row.Title})
		}
		if cands == nil {
			cands = []feedfinder.Candidate{}
		}
		writeJSON(w, resolveResponse{Candidates: cands, ExistingSubscriptions: existing})
	})
}

// --- POST /api/subscriptions ---

type addRequest struct {
	Subscriptions []addItem `json:"subscriptions"`
}

type addItem struct {
	FeedURL     string `json:"feedUrl"`
	Title       string `json:"title"`
	CustomTitle string `json:"customTitle"`
	SiteURL     string `json:"siteUrl"`
}

type addResponse struct {
	Records []SubscriptionWire `json:"records"`
	JobIDs  []string           `json:"jobIds"`
}

// SubscriptionsCreateHandler implements the Choice A write contract:
// dedupe → PDS createRecord → Tier-1 upsert → Tier-2 upsert → dispatch
// fetch_one_feed. Returns the new records and the dispatched job ids so the
// frontend can show the pill immediately.
func SubscriptionsCreateHandler(
	reader IndexReader,
	writer IndexWriter,
	pds atprepo.Writer,
	disp FetchDispatcher,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var body addRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		fieldErrors := make(map[string]string)
		for i, item := range body.Subscriptions {
			if strings.TrimSpace(item.FeedURL) == "" {
				fieldErrors["subscriptions."+itoa(i)+".feedUrl"] = "Feed URL is required"
			}
		}
		if len(fieldErrors) > 0 {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"errors": fieldErrors})
			return
		}
		if len(body.Subscriptions) == 0 {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"message": "no subscriptions submitted"})
			return
		}

		out := addResponse{Records: make([]SubscriptionWire, 0, len(body.Subscriptions)), JobIDs: []string{}}
		now := time.Now().UTC().Format(time.RFC3339)
		didStr := sess.Data.AccountDID.String()

		for _, item := range body.Subscriptions {
			// Step 1: dedupe guard. If a Tier-1 row already maps this DID
			// to this feed_url, return the existing record idempotently.
			if row, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
				Did:     didStr,
				FeedUrl: item.FeedURL,
			}); err == nil {
				out.Records = append(out.Records, rowToWire(row))
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				slog.Warn("/api/subscriptions: dedupe probe failed", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			// Step 2: PDS write. title = canonical (from resolver), customTitle = user override.
			record := map[string]any{
				"feedUrl":   item.FeedURL,
				"createdAt": now,
			}
			if item.Title != "" {
				record["title"] = item.Title
			}
			if item.CustomTitle != "" {
				record["customTitle"] = item.CustomTitle
			}
			if item.SiteURL != "" {
				record["siteUrl"] = item.SiteURL
			}
			ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), record)
			if err != nil {
				slog.Warn("/api/subscriptions: PDS create failed", "err", err)
				http.Error(w, "upstream PDS error", http.StatusBadGateway)
				return
			}
			rkey := rkeyFromATURI(ref.URI)

			// Step 3: Tier-2 catalog upsert (dedup by canonical URL).
			titlePtr := nilIfEmpty(item.Title)
			customTitlePtr := nilIfEmpty(item.CustomTitle)
			siteURLPtr := nilIfEmpty(item.SiteURL)
			if err := writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
				FeedUrl:   item.FeedURL,
				SiteUrl:   siteURLPtr,
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				slog.Error("/api/subscriptions: Tier-2 upsert failed (PDS write already succeeded — next sync_user will reconcile)", "err", err)
			}

			// Step 4: Tier-1 index upsert.
			if err := writer.UpsertUserSubscription(r.Context(), db.UpsertUserSubscriptionParams{
				Did:         didStr,
				Rkey:        rkey,
				AtUri:       ref.URI,
				FeedUrl:     item.FeedURL,
				Title:       titlePtr,
				CustomTitle: customTitlePtr,
				CreatedAt:   now,
				UpdatedAt:   now,
			}); err != nil {
				slog.Error("/api/subscriptions: Tier-1 upsert failed (PDS write already succeeded — next sync_user will reconcile)", "err", err)
			}

			value := map[string]any{
				"feedUrl":   item.FeedURL,
				"siteUrl":   item.SiteURL,
				"createdAt": now,
			}
			if item.Title != "" {
				value["title"] = item.Title
			}
			if item.CustomTitle != "" {
				value["customTitle"] = item.CustomTitle
			}
			out.Records = append(out.Records, SubscriptionWire{
				URI:     ref.URI,
				CID:     ref.CID,
				Rkey:    rkey,
				FeedURL: item.FeedURL,
				Title:   item.Title,
				SiteURL: item.SiteURL,
				Value:   value,
			})

			// Step 5: dispatch fetch_one_feed (async).
			jobID := disp.StartFetchOneFeed(r.Context(), sess.Data.AccountDID, item.FeedURL)
			out.JobIDs = append(out.JobIDs, jobID)
		}

		writeJSON(w, out)
	})
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func itoa(i int) string {
	// minimal local helper so we don't pull strconv just for this one site.
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// rkeyFromATURI extracts the rkey segment from an at-uri like
// at://did:plc:alice/app.skyreader.feed.subscription/3la123.
func rkeyFromATURI(uri string) string {
	parts := strings.Split(uri, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// --- Legacy PDS pass-through retained for transition period only ---

type Lister interface {
	ListRecords(ctx context.Context, did syntax.DID, collection string, sess *oauth.ClientSession) ([]map[string]any, error)
}

func SubscriptionsHandler(lister Lister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		records, err := lister.ListRecords(r.Context(), sess.Data.AccountDID, subscriptionCollection, sess)
		if err != nil {
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		if records == nil {
			records = []map[string]any{}
		}
		writeJSON(w, records)
	})
}

type PDSLister struct{}

type listRecordsResp struct {
	Records []map[string]any `json:"records"`
	Cursor  string           `json:"cursor"`
}

func (PDSLister) ListRecords(ctx context.Context, did syntax.DID, collection string, sess *oauth.ClientSession) ([]map[string]any, error) {
	client := sess.APIClient()
	var out listRecordsResp
	params := map[string]any{
		"repo":       did.String(),
		"collection": collection,
	}
	if err := client.Get(ctx, syntax.NSID("com.atproto.repo.listRecords"), params, &out); err != nil {
		return nil, err
	}
	return out.Records, nil
}

var (
	_ Lister              = PDSLister{}
	_ atclient.AuthMethod = (*oauth.ClientSession)(nil)
)

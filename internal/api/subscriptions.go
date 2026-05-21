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

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/middleware/auth"
)

const subscriptionCollection = "app.skyreader.feed.subscription"

// SubscriptionWire is the on-the-wire shape returned by GET / POST. The list
// endpoint additionally fills FaviconURL, Frequency, and LastPublishedAt;
// POST leaves those empty (the caller polls /api/subscriptions to pick them
// up after the first fetch lands).
type SubscriptionWire struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid,omitempty"`
	Value map[string]any `json:"value"`
	// Embedded sugar for the frontend so callers don't dig into Value.
	Rkey            string `json:"rkey"`
	FeedURL         string `json:"feedUrl"`
	Title           string `json:"title,omitempty"`
	SiteURL         string `json:"siteUrl,omitempty"`
	FaviconURL      string `json:"faviconUrl,omitempty"`
	Frequency       string `json:"frequency,omitempty"`
	LastPublishedAt string `json:"lastPublishedAt,omitempty"`
}

// IndexReader is the slice of *db.Queries we use for reads. Defined so handler
// tests can stub the database without spinning up sqlite.
type IndexReader interface {
	ListUserSubscriptions(ctx context.Context, did string) ([]db.UserSubscription, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
}

// SourcesReader is the narrow read used by the list endpoint — it carries the
// per-feed entry stats the sources card renders.
type SourcesReader interface {
	ListUserSourcesWithStats(ctx context.Context, arg db.ListUserSourcesWithStatsParams) ([]db.ListUserSourcesWithStatsRow, error)
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

// FetchDispatcher fires off a fetch_one_feed job per added subscription, and
// can promote a full sync_user when local writes diverge from PDS (PDS is the
// source of truth, so the next reconcile must converge). The returned id
// surfaces back to the client so the RefreshPill can poll.
type FetchDispatcher interface {
	StartFetchOneFeed(did syntax.DID, feedURL string) string
	StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

// SubscriptionsListHandler returns the user's subscriptions joined with feed
// metadata and a derived frequency bucket. The bucket is computed here (not
// in SQL) so the rule lives in one place.
func SubscriptionsListHandler(reader SourcesReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		params := db.ListUserSourcesWithStatsParams{
			Did:       sess.Data.AccountDID.String(),
			Now:       now.Format(time.RFC3339),
			Cutoff7d:  now.AddDate(0, 0, -7).Format(time.RFC3339),
			Cutoff28d: now.AddDate(0, 0, -28).Format(time.RFC3339),
			Cutoff56d: now.AddDate(0, 0, -56).Format(time.RFC3339),
			Cutoff84d: now.AddDate(0, 0, -84).Format(time.RFC3339),
		}
		rows, err := reader.ListUserSourcesWithStats(r.Context(), params)
		if err != nil {
			slog.Warn("/api/subscriptions: list failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]SubscriptionWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, sourceRowToWire(row, now))
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

func sourceRowToWire(row db.ListUserSourcesWithStatsRow, now time.Time) SubscriptionWire {
	value := map[string]any{"feedUrl": row.FeedUrl}
	title := ""
	if row.Title != nil {
		value["title"] = *row.Title
		title = *row.Title
	}
	if row.CustomTitle != nil {
		value["customTitle"] = *row.CustomTitle
	}
	siteURL := ""
	if row.SiteUrl != nil {
		value["siteUrl"] = *row.SiteUrl
		siteURL = *row.SiteUrl
	}
	faviconURL := ""
	if row.IconUrl != nil {
		faviconURL = *row.IconUrl
	}
	lastPublished := asString(row.LastPublishedAt)
	firstPublished := asString(row.FirstPublishedAt)
	return SubscriptionWire{
		URI:             row.AtUri,
		Value:           value,
		Rkey:            row.Rkey,
		FeedURL:         row.FeedUrl,
		Title:           title,
		SiteURL:         siteURL,
		FaviconURL:      faviconURL,
		Frequency:       frequencyBucket(firstPublished, row.Count7d, row.Count28d, row.Count56d, row.Count84d, now),
		LastPublishedAt: lastPublished,
	}
}

// frequencyBucket implements the cadence rule:
//   - noPosts   — no entries at all
//   - new       — first-ever post is within 30 days (and ≥1 post exists)
//   - daily     — ≥5 posts in last 7 days
//   - weekly    — ≥3 posts in last 28 days
//   - biweekly  — ≥3 posts in last 56 days
//   - monthly   — ≥2 posts in last 84 days
//   - irregular — anything else
//
// Highest-cadence bucket wins. "New" overrides everything else when applicable.
func frequencyBucket(firstPublishedAt string, c7, c28, c56, c84 int64, now time.Time) string {
	if firstPublishedAt == "" {
		return "noPosts"
	}
	if t, err := time.Parse(time.RFC3339, firstPublishedAt); err == nil {
		if now.Sub(t) <= 30*24*time.Hour {
			return "new"
		}
	}
	switch {
	case c7 >= 5:
		return "daily"
	case c28 >= 3:
		return "weekly"
	case c56 >= 3:
		return "biweekly"
	case c84 >= 2:
		return "monthly"
	default:
		return "irregular"
	}
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
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
			rkey := atprepo.RkeyFromATURI(ref.URI)

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
				slog.Error("/api/subscriptions: Tier-1 upsert failed; dispatching sync_user to reconcile from PDS", "err", err)
				if _, derr := disp.StartManualRefresh(r.Context(), sess.Data.AccountDID, sess.Data.SessionID); derr != nil {
					slog.Warn("/api/subscriptions: sync_user dispatch failed", "err", derr)
				}
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
			jobID := disp.StartFetchOneFeed(sess.Data.AccountDID, item.FeedURL)
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

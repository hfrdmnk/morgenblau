package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
	"morgenblau/internal/standardfeed"
	"morgenblau/internal/tags"
)

const (
	subscriptionCollection = lexicon.Subscription
	sourceTypeRSS          = lexicon.SourceRSS
	sourceTypeStandard     = lexicon.SourceStandard
)

// sourceUnion rebuilds the record's `source` variant. The catalog key doubles
// as the variant payload: feed URL for rss, publication at-uri for
// standardfeed. siteURL rides the rss variant when known (ignored for
// standardfeed); callers must pass it so a rebuild never drops it.
func sourceUnion(kind, catalogKey, siteURL string) map[string]any {
	if kind == "standardfeed" {
		return map[string]any{"$type": sourceTypeStandard, "publication": catalogKey}
	}
	source := map[string]any{"$type": sourceTypeRSS, "feedUrl": catalogKey}
	if siteURL != "" {
		source["siteUrl"] = siteURL
	}
	return source
}

// wireKind normalizes a stored kind for the wire; anything but standardfeed
// (including the zero value on rows predating the column) reads as rss.
func wireKind(kind string) string {
	if kind == "standardfeed" {
		return "standardfeed"
	}
	return "rss"
}

// requireStandardWrite gates site.standard.graph.* writes on the session's
// grant. Sessions minted before the scope change get a 403 the frontend
// turns into a calm re-auth prompt.
func requireStandardWrite(w http.ResponseWriter, sess *oauth.ClientSession) bool {
	if scopes.HasStandardWrite(sess) {
		return true
	}
	writeJSONStatus(w, http.StatusForbidden, map[string]string{
		"code":    "reauth_required",
		"message": "sign in again to enable ATProto subscriptions",
	})
	return false
}

// SubscriptionWire is the on-the-wire shape returned by GET / POST. The list
// endpoint additionally fills FaviconURL, Frequency, and LastPublishedAt;
// POST leaves those empty (the caller polls /api/subscriptions to pick them
// up after the first fetch lands).
type SubscriptionWire struct {
	URI   string         `json:"uri"`
	CID   string         `json:"cid,omitempty"`
	Value map[string]any `json:"value"`
	// Embedded sugar for the frontend so callers don't dig into Value.
	Rkey            string   `json:"rkey"`
	Kind            string   `json:"kind"`
	FeedURL         string   `json:"feedUrl"`
	Publication     string   `json:"publication,omitempty"`
	Title           string   `json:"title,omitempty"`
	SiteURL         string   `json:"siteUrl,omitempty"`
	FaviconURL      string   `json:"faviconUrl,omitempty"`
	Frequency       string   `json:"frequency,omitempty"`
	LastPublishedAt string   `json:"lastPublishedAt,omitempty"`
	Primary         bool     `json:"primary"`
	Tags            []string `json:"tags,omitempty"`
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
		"source": sourceUnion(row.Kind, row.FeedUrl, ""),
	}
	title := ""
	if row.Title != nil {
		value["title"] = *row.Title
		title = *row.Title
	}
	primary := row.IsPrimary != 0
	tagList := tags.Unmarshal(row.Tags)
	if primary {
		value["primary"] = true
	}
	if len(tagList) > 0 {
		value["tags"] = tagList
	}
	kind := wireKind(row.Kind)
	wire := SubscriptionWire{
		URI:     row.AtUri,
		Value:   value,
		Rkey:    row.Rkey,
		Kind:    kind,
		FeedURL: row.FeedUrl,
		Title:   title,
		Primary: primary,
		Tags:    tagList,
	}
	if kind == "standardfeed" {
		wire.Publication = row.FeedUrl
	}
	return wire
}

func sourceRowToWire(row db.ListUserSourcesWithStatsRow, now time.Time) SubscriptionWire {
	value := map[string]any{"source": sourceUnion(row.Kind, row.FeedUrl, derefStr(row.SiteUrl))}
	title := ""
	if row.Title != nil {
		value["title"] = *row.Title
		title = *row.Title
	}
	if title == "" && row.CatalogTitle != nil {
		// No user override: fall back to the catalog title (the cached
		// publication name for standardfeed sources).
		title = *row.CatalogTitle
	}
	siteURL := ""
	if row.SiteUrl != nil {
		siteURL = *row.SiteUrl
	}
	faviconURL := ""
	if row.IconUrl != nil {
		faviconURL = *row.IconUrl
	}
	primary := row.IsPrimary != 0
	tagList := tags.Unmarshal(row.Tags)
	if primary {
		value["primary"] = true
	}
	if len(tagList) > 0 {
		value["tags"] = tagList
	}
	lastPublished := asString(row.LastPublishedAt)
	firstPublished := asString(row.FirstPublishedAt)
	kind := wireKind(row.Kind)
	wire := SubscriptionWire{
		URI:             row.AtUri,
		Value:           value,
		Rkey:            row.Rkey,
		Kind:            kind,
		FeedURL:         row.FeedUrl,
		Title:           title,
		SiteURL:         siteURL,
		FaviconURL:      faviconURL,
		Frequency:       frequencyBucket(firstPublished, row.Count7d, row.Count28d, row.Count56d, row.Count84d, now),
		LastPublishedAt: lastPublished,
		Primary:         primary,
		Tags:            tagList,
	}
	if kind == "standardfeed" {
		wire.Publication = row.FeedUrl
	}
	return wire
}

// frequencyBucket implements the cadence rule:
//   - noPosts   — no entries at all
//   - daily     — ≥5 posts in last 7 days
//   - weekly    — ≥3 posts in last 28 days
//   - biweekly  — ≥3 posts in last 56 days
//   - monthly   — ≥2 posts in last 84 days
//   - new       — no cadence bucket fires, but the oldest entry we have is
//     within 30 days (not enough history to classify yet)
//   - irregular — anything else
//
// Cadence wins over "new": a feed posting daily still reads as daily even if
// we only started ingesting it last week. firstPublishedAt is MIN(published_at)
// over rows we've stored, so it reflects our ingestion window, not the feed's
// debut.
func frequencyBucket(firstPublishedAt string, c7, c28, c56, c84 int64, now time.Time) string {
	if firstPublishedAt == "" {
		return "noPosts"
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
	}
	if t, err := time.Parse(time.RFC3339, firstPublishedAt); err == nil {
		if now.Sub(t) <= 30*24*time.Hour {
			return "new"
		}
	}
	return "irregular"
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

// candidateWire is a finder candidate plus the sibling annotation: set when
// the user already subscribes to the same site under the OTHER kind.
type candidateWire struct {
	feedfinder.Candidate
	SubscribedVia *subscribedVia `json:"subscribedVia,omitempty"`
}

type subscribedVia struct {
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
}

type resolveResponse struct {
	Candidates            []candidateWire   `json:"candidates"`
	ExistingSubscriptions []existingSubMeta `json:"existingSubscriptions"`
}

// existingSubMeta flags a candidate the user already subscribes to. FeedURL
// carries the catalog key — the publication at-uri for standardfeed rows —
// so the picker can match it against `publication ?? feedUrl`.
type existingSubMeta struct {
	FeedURL string  `json:"feedUrl"`
	Title   *string `json:"title"`
}

// ResolveReader is the read slice of the resolve handler: the per-candidate
// dedupe probe plus the sibling-guard join.
type ResolveReader interface {
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
	ListUserSubscriptionsWithSiteURL(ctx context.Context, did string) ([]db.ListUserSubscriptionsWithSiteURLRow, error)
}

// SubscriptionsResolveHandler turns a pasted URL into feed candidates, flags
// any the user already subscribes to, and annotates cross-kind siblings.
func SubscriptionsResolveHandler(reader ResolveReader, finder FeedFinder) http.Handler {
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

		// Sibling annotation is best-effort UX sugar — a failed join never
		// fails the resolve.
		siblings := map[string][]subscribedVia{}
		if subs, err := reader.ListUserSubscriptionsWithSiteURL(r.Context(), sess.Data.AccountDID.String()); err != nil {
			slog.Warn("/api/subscriptions/resolve: sibling join failed", "err", err)
		} else {
			for _, s := range subs {
				key := subscriptionSiblingKey(s)
				if key == "" {
					continue
				}
				title := ""
				if s.Title != nil && *s.Title != "" {
					title = *s.Title
				} else if s.CatalogTitle != nil {
					title = *s.CatalogTitle
				}
				siblings[key] = append(siblings[key], subscribedVia{Kind: wireKind(s.Kind), Title: title})
			}
		}

		existing := make([]existingSubMeta, 0)
		out := make([]candidateWire, 0, len(cands))
		for _, c := range cands {
			// Candidate identity: publication at-uri for standardfeed, feed
			// URL for rss — the same key the create path dedupes on.
			probeKey := c.FeedURL
			if c.Publication != "" {
				probeKey = c.Publication
			}
			row, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
				Did:     sess.Data.AccountDID.String(),
				FeedUrl: probeKey,
			})
			switch {
			case err == nil:
				existing = append(existing, existingSubMeta{FeedURL: row.FeedUrl, Title: row.Title})
			case !errors.Is(err, sql.ErrNoRows):
				slog.Warn("/api/subscriptions/resolve: index probe failed", "err", err)
			}

			wire := candidateWire{Candidate: c}
			key, kind := candidateSiblingKey(c)
			if key != "" {
				for _, via := range siblings[key] {
					if via.Kind != kind {
						v := via
						wire.SubscribedVia = &v
						break
					}
				}
			}
			out = append(out, wire)
		}
		writeJSON(w, resolveResponse{Candidates: out, ExistingSubscriptions: existing})
	})
}

// siblingKey normalizes a site URL for cross-kind matching: lowercase host
// minus "www.", plus the path minus its trailing slash. Host+path keeps
// shared-host publications (leaflet.pub/<pub>) from false matches.
func siblingKey(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	return host + strings.TrimRight(u.Path, "/")
}

// rssSiblingKey keys an rss source: its site URL, falling back to the feed
// URL's bare host (empty path) when no site URL is known.
func rssSiblingKey(siteURL, feedURL string) string {
	if key := siblingKey(siteURL); key != "" {
		return key
	}
	u, err := url.Parse(feedURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

func subscriptionSiblingKey(row db.ListUserSubscriptionsWithSiteURLRow) string {
	siteURL := ""
	if row.SiteUrl != nil {
		siteURL = *row.SiteUrl
	}
	if row.Kind == "standardfeed" {
		return siblingKey(siteURL)
	}
	return rssSiblingKey(siteURL, row.FeedUrl)
}

func candidateSiblingKey(c feedfinder.Candidate) (string, string) {
	kind := wireKind(c.Kind)
	if kind == "standardfeed" {
		return siblingKey(c.SiteURL), kind
	}
	return rssSiblingKey(c.SiteURL, c.FeedURL), kind
}

// --- POST /api/subscriptions ---

type addRequest struct {
	Subscriptions []addItem `json:"subscriptions"`
}

// addItem carries either feedUrl (rss) or publication (standardfeed) —
// exactly one. For standardfeed, title/primary/tags are sent only when the
// user customized them in the picker; defaults mean "no sidecar record".
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
		hasStandard := false
		for i, item := range body.Subscriptions {
			feedURL := strings.TrimSpace(item.FeedURL)
			publication := strings.TrimSpace(item.Publication)
			switch {
			case feedURL == "" && publication == "":
				fieldErrors["subscriptions."+itoa(i)+".feedUrl"] = "Feed URL is required"
			case feedURL != "" && publication != "":
				fieldErrors["subscriptions."+itoa(i)+".publication"] = "feedUrl and publication are mutually exclusive"
			case publication != "":
				if _, err := syntax.ParseATURI(publication); err != nil {
					fieldErrors["subscriptions."+itoa(i)+".publication"] = "publication must be an at:// URI"
				}
				hasStandard = true
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
		// Gate the whole batch before any write: a mid-batch 403 would leave
		// earlier items created and later ones silently dropped.
		if hasStandard && !requireStandardWrite(w, sess) {
			return
		}
		// Sibling belt: an rss feed and a publication for the same site in
		// one batch is exactly the double-subscribe the picker guards against.
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
				writeJSONStatus(w, http.StatusConflict, map[string]string{
					"message": "Pick either the RSS feed or the ATProto publication for a site, not both",
				})
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
			// The catalog key: feed URL for rss, publication at-uri for
			// standardfeed. Keys Tier-2, Tier-1, dedupe, and the fetch job.
			key := item.FeedURL
			kind := "rss"
			if isStandard {
				key = item.Publication
				kind = "standardfeed"
			}

			// Step 1: dedupe guard. If a Tier-1 row already maps this DID
			// to this catalog key, return the existing record idempotently.
			if row, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
				Did:     didStr,
				FeedUrl: key,
			}); err == nil {
				out.Records = append(out.Records, rowToWire(row))
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				slog.Warn("/api/subscriptions: dedupe probe failed", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
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
				// The existence record is the portable standard subscription.
				var err error
				ref, err = pds.CreateRecord(r.Context(), sess, syntax.NSID(standardfeed.CollectionSubscription), map[string]any{
					"publication": key,
					"createdAt":   now,
				})
				if err != nil {
					slog.Warn("/api/subscriptions: standard record create failed", "err", err)
					http.Error(w, "upstream PDS error", http.StatusBadGateway)
					return
				}
				// Lazy blue.morgen sidecar, only when the picker customized
				// metadata. Written second so a failure still leaves an
				// adoptable standard record for the next reconcile.
				if item.Title != "" || item.Primary || len(tagList) > 0 {
					source = sourceUnion(kind, key, "")
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
					scRef, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), sidecar)
					if err != nil {
						slog.Warn("/api/subscriptions: sidecar create failed (standard record already on PDS; reconcile will adopt it)", "err", err)
						http.Error(w, "upstream PDS error", http.StatusBadGateway)
						return
					}
					scRkey := atprepo.RkeyFromATURI(scRef.URI)
					sidecarRkey = &scRkey
				} else {
					source = sourceUnion(kind, key, "")
				}
			} else {
				// Single user-editable title; the resolver prefilled it
				// client-side, the user may have overridden before submit.
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
				// TODO(blue.morgen lexicon): once the blue.morgen.feed.subscription
				// lexicon is published as a com.atproto.lexicon.schema record and
				// resolvable on the network, validate `record` before write. Use
				// lexicon.ValidateRecord(&catalog, obj, "blue.morgen.feed.subscription", 0)
				// after decoding with data.UnmarshalJSON. See SPEC.md <lexicons>.
				var err error
				ref, err = pds.CreateRecord(r.Context(), sess, syntax.NSID(subscriptionCollection), record)
				if err != nil {
					slog.Warn("/api/subscriptions: PDS create failed", "err", err)
					http.Error(w, "upstream PDS error", http.StatusBadGateway)
					return
				}
			}
			rkey := atprepo.RkeyFromATURI(ref.URI)

			// Step 3: Tier-2 catalog upsert (dedup by canonical key). Title
			// stays nil — feeds.title is the cached publication name, owned
			// by the fetch pipeline.
			titlePtr := nilIfEmpty(item.Title)
			siteURLPtr := nilIfEmpty(item.SiteURL)
			if err := writer.UpsertFeed(r.Context(), db.UpsertFeedParams{
				FeedUrl:   key,
				Kind:      kind,
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
				FeedUrl:     key,
				Kind:        kind,
				SidecarRkey: sidecarRkey,
				Title:       titlePtr,
				IsPrimary:   boolToInt64(item.Primary),
				Tags:        tags.Marshal(tagList),
				CreatedAt:   now,
				UpdatedAt:   now,
			}); err != nil {
				slog.Error("/api/subscriptions: Tier-1 upsert failed; dispatching sync_user to reconcile from PDS", "err", err)
				if _, derr := disp.StartManualRefresh(r.Context(), sess.Data.AccountDID, sess.Data.SessionID); derr != nil {
					slog.Warn("/api/subscriptions: sync_user dispatch failed", "err", derr)
				}
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

// --- GET /api/subscriptions/tags ---

// TagsReader is the narrow read used by the "my tags" endpoint.
type TagsReader interface {
	ListUserSubscriptionTags(ctx context.Context, did string) ([]*string, error)
}

type tagsResponse struct {
	Tags []string `json:"tags"`
}

// SubscriptionsTagsHandler returns the distinct union of tags across the
// session user's subscriptions: deduped case-insensitively (first-seen casing
// wins), sorted ascending case-insensitively. Always a JSON array, never null.
func SubscriptionsTagsHandler(reader TagsReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rows, err := reader.ListUserSubscriptionTags(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/subscriptions/tags: list failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		seen := map[string]struct{}{}
		out := []string{}
		for _, row := range rows {
			for _, tag := range tags.Unmarshal(row) {
				key := strings.ToLower(tag)
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, tag)
			}
		}
		sort.Slice(out, func(i, j int) bool {
			return strings.ToLower(out[i]) < strings.ToLower(out[j])
		})
		writeJSON(w, tagsResponse{Tags: out})
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

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

const maxTags = 10
const maxTagGraphemes = 64

// normalizeTags trims, drops blanks, dedupes case-insensitively (keeping the
// first-seen casing), drops any tag longer than 64 graphemes, and caps the
// result at 10. Order is preserved. The grapheme limit uses a rune count
// (len([]rune)) rather than a full Unicode segmentation pass — close enough for
// the lexicon's maxGraphemes:64 guard without pulling in a segmentation dep.
func normalizeTags(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || len([]rune(t)) > maxTagGraphemes {
			continue
		}
		key := strings.ToLower(t)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
		if len(out) == maxTags {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

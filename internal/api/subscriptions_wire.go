package api

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"

	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/oauth/scopes"
	"morgenblau/internal/tags"
)

const (
	subscriptionCollection = lexicon.Subscription
	sourceTypeRSS          = lexicon.SourceRSS
	sourceTypeStandard     = lexicon.SourceStandard
)

// sourceUnion rebuilds the record's source variant; the catalog key is the feed URL for rss or publication at-uri for standardfeed, and siteURL rides the rss variant only.
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

// wireKind normalizes kind for the wire; anything but standardfeed, including the pre-migration zero value, reads as rss.
func wireKind(kind string) string {
	if kind == "standardfeed" {
		return "standardfeed"
	}
	return "rss"
}

// requireStandardWrite gates site.standard.graph.* writes on the session's scope; pre-migration sessions get a 403 the frontend turns into a re-auth prompt.
func requireStandardWrite(w http.ResponseWriter, sess *oauth.ClientSession) bool {
	if scopes.HasStandardWrite(sess) {
		return true
	}
	writeError(w, http.StatusForbidden, codeReauthRequired, "sign in again to enable ATProto subscriptions")
	return false
}

// mutedAfterConsecutiveFailures is the SPEC <feed-sources> failure-handling threshold: 20 consecutive failures silently mutes a feed.
const mutedAfterConsecutiveFailures = 20

// SubscriptionWire is the on-the-wire shape for GET/POST; POST leaves FaviconURL, Frequency, and LastPublishedAt empty until the client polls GET after the first fetch.
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
	LastFetchedAt   string   `json:"lastFetchedAt,omitempty"`
	Muted           bool     `json:"muted,omitempty"`
	Primary         bool     `json:"primary"`
	Tags            []string `json:"tags,omitempty"`
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

// subscriptionStatsFields is the field set shared by sourceRowToWire and sourceDetailRowToWire, which differ only in their source query (list vs. single-subscription).
type subscriptionStatsFields struct {
	Rkey                string
	AtUri               string
	FeedUrl             string
	Kind                string
	Title               *string
	IsPrimary           int64
	Tags                *string
	SiteUrl             *string
	IconUrl             *string
	CatalogTitle        *string
	LastFetchedAt       *string
	ConsecutiveFailures int64
	LastPublishedAt     any
	FirstPublishedAt    any
	Count7d             int64
	Count28d            int64
	Count56d            int64
	Count84d            int64
}

func sourceRowToWire(row db.ListUserSourcesWithStatsRow, now time.Time) SubscriptionWire {
	return subscriptionStatsRowToWire(subscriptionStatsFields{
		Rkey:                row.Rkey,
		AtUri:               row.AtUri,
		FeedUrl:             row.FeedUrl,
		Kind:                row.Kind,
		Title:               row.Title,
		IsPrimary:           row.IsPrimary,
		Tags:                row.Tags,
		SiteUrl:             row.SiteUrl,
		IconUrl:             row.IconUrl,
		CatalogTitle:        row.CatalogTitle,
		LastFetchedAt:       row.LastFetchedAt,
		ConsecutiveFailures: row.ConsecutiveFailures,
		LastPublishedAt:     row.LastPublishedAt,
		FirstPublishedAt:    row.FirstPublishedAt,
		Count7d:             row.Count7d,
		Count28d:            row.Count28d,
		Count56d:            row.Count56d,
		Count84d:            row.Count84d,
	}, now)
}

func subscriptionStatsRowToWire(f subscriptionStatsFields, now time.Time) SubscriptionWire {
	value := map[string]any{"source": sourceUnion(f.Kind, f.FeedUrl, derefStr(f.SiteUrl))}
	title := ""
	if f.Title != nil {
		value["title"] = *f.Title
		title = *f.Title
	}
	if title == "" && f.CatalogTitle != nil {
		// No user override: fall back to the catalog title (cached publication name for standardfeed).
		title = *f.CatalogTitle
	}
	siteURL := ""
	if f.SiteUrl != nil {
		siteURL = *f.SiteUrl
	}
	faviconURL := ""
	if f.IconUrl != nil {
		faviconURL = *f.IconUrl
	}
	primary := f.IsPrimary != 0
	tagList := tags.Unmarshal(f.Tags)
	if primary {
		value["primary"] = true
	}
	if len(tagList) > 0 {
		value["tags"] = tagList
	}
	lastPublished := asString(f.LastPublishedAt)
	firstPublished := asString(f.FirstPublishedAt)
	kind := wireKind(f.Kind)
	wire := SubscriptionWire{
		URI:             f.AtUri,
		Value:           value,
		Rkey:            f.Rkey,
		Kind:            kind,
		FeedURL:         f.FeedUrl,
		Title:           title,
		SiteURL:         siteURL,
		FaviconURL:      faviconURL,
		Frequency:       frequencyBucket(firstPublished, f.Count7d, f.Count28d, f.Count56d, f.Count84d, now),
		LastPublishedAt: lastPublished,
		LastFetchedAt:   derefStr(f.LastFetchedAt),
		Muted:           f.ConsecutiveFailures >= mutedAfterConsecutiveFailures,
		Primary:         primary,
		Tags:            tagList,
	}
	if kind == "standardfeed" {
		wire.Publication = f.FeedUrl
	}
	return wire
}

// frequencyBucket: cadence outranks "new" (a daily feed reads daily even if ingestion just started).
// firstPublishedAt is MIN(published_at) over stored rows, so it reflects our ingestion window, not the feed's real debut.
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

// siblingKey normalizes a site URL for cross-kind matching (lowercase host minus "www.", plus path minus trailing slash); host+path avoids false matches on shared-host publications like leaflet.pub/<pub>.
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

// normalizeTags approximates the lexicon's maxGraphemes:64 guard with a rune count (len([]rune)) instead of full Unicode segmentation, close enough without pulling in a dependency.
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

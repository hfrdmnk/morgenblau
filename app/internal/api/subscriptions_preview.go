package api

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"

	"morgenblau/internal/fetcher"
	"morgenblau/internal/socialspec"
	"morgenblau/internal/standardfeed"
)

const previewLimit = 3

// --- POST /api/subscriptions/preview ---

type previewRequest struct {
	FeedURL     string `json:"feedUrl"`
	Publication string `json:"publication"`
}

type previewItem struct {
	Title       string `json:"title"`
	PublishedAt string `json:"publishedAt,omitempty"`
	URL         string `json:"url,omitempty"`
	ContentHTML string `json:"contentHtml,omitempty"`
}

type previewResponse struct {
	Items []previewItem `json:"items"`
}

// FeedFetcher is the slice of *fetcher.Fetcher the preview needs; it shares the sync pipeline's guarded client and worker pool.
type FeedFetcher interface {
	Fetch(ctx context.Context, rawURL string, state fetcher.FeedState) (*fetcher.Result, error)
}

// DocumentLister is the slice of *standardfeed.Client the preview needs.
type DocumentLister interface {
	GetPublication(ctx context.Context, rawURI string) (*standardfeed.Publication, error)
	ListDocuments(ctx context.Context, pubURI string) ([]standardfeed.Document, error)
}

// SubscriptionsPreviewHandler returns a candidate's newest posts before the reader follows it; nothing is stored.
func SubscriptionsPreviewHandler(feeds FeedFetcher, docs DocumentLister) http.Handler {
	sanitizer := bluemonday.UGCPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSession(w, r); !ok {
			return
		}
		var body previewRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		feedURL, pub := strings.TrimSpace(body.FeedURL), strings.TrimSpace(body.Publication)
		if (feedURL == "") == (pub == "") {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "Send either feedUrl or publication")
			return
		}
		items, err := previewItems(r.Context(), feeds, docs, feedURL, pub)
		if err != nil {
			slog.Warn("/api/subscriptions/preview: upstream failed", "feed", feedURL+pub, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "Couldn't load recent posts")
			return
		}
		// Undated items sort last; feeds list newest first more often than not, so the stable sort keeps their order.
		slices.SortStableFunc(items, func(a, b previewItem) int { return cmp.Compare(b.PublishedAt, a.PublishedAt) })
		items = items[:min(len(items), previewLimit)]
		// Sanitizing only the kept items spares a full-content feed's hundred bodies.
		for i := range items {
			items[i].ContentHTML = sanitizer.Sanitize(items[i].ContentHTML)
		}
		writeJSON(w, previewResponse{Items: items})
	})
}

// Bodies follow the sync pipelines, so a previewed post reads as it will in the newspaper; they are unsanitized until the handler trims the list.
func previewItems(ctx context.Context, feeds FeedFetcher, docs DocumentLister, feedURL, pub string) ([]previewItem, error) {
	items := []previewItem{}
	if pub != "" {
		publication, err := docs.GetPublication(ctx, pub)
		if err != nil {
			return nil, err
		}
		list, err := docs.ListDocuments(ctx, pub)
		if err != nil {
			return nil, err
		}
		for _, d := range list {
			item := previewItem{
				Title:       strings.TrimSpace(d.Title),
				PublishedAt: d.PublishedAt,
				ContentHTML: cmp.Or(d.Description, d.TextContent),
			}
			if d.Path != "" && publication.URL != "" {
				item.URL = publication.URL + "/" + strings.TrimPrefix(d.Path, "/")
			}
			items = append(items, item)
		}
		return items, nil
	}
	res, err := feeds.Fetch(ctx, feedURL, fetcher.FeedState{})
	if err != nil {
		return nil, err
	}
	if res.Feed == nil {
		return items, nil
	}
	for _, it := range res.Feed.Items {
		item := previewItem{
			Title:       strings.TrimSpace(it.Title),
			URL:         it.Link,
			ContentHTML: cmp.Or(strings.TrimSpace(it.Content), strings.TrimSpace(it.Description)),
		}
		if at := cmp.Or(it.PublishedParsed, it.UpdatedParsed); at != nil {
			item.PublishedAt = at.UTC().Format(time.RFC3339)
		}
		items = append(items, item)
	}
	return items, nil
}

// --- POST /api/subscriptions/preview/article ---

type previewArticleRequest struct {
	URL string `json:"url"`
}

type previewArticleResponse struct {
	ContentHTML string `json:"contentHtml"`
}

// SubscriptionsPreviewArticleHandler extracts a previewed post the way the reader does, without an entry to cache it on.
// httpClient must be the safehttp client: the URL comes from a feed the reader has not followed yet.
func SubscriptionsPreviewArticleHandler(httpClient *http.Client) http.Handler {
	sanitizer := bluemonday.UGCPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSession(w, r); !ok {
			return
		}
		var body previewArticleRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		u, err := url.Parse(strings.TrimSpace(body.URL))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "url must be a website address")
			return
		}
		content, err := extractReadable(r.Context(), httpClient, u.String(), sanitizer)
		if err != nil {
			slog.Warn("/api/subscriptions/preview/article: extract failed", "url", u.String(), "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "Couldn't load the article")
			return
		}
		writeJSON(w, previewArticleResponse{ContentHTML: content})
	})
}

// --- POST /api/subscriptions/profile ---

type profileRequest struct {
	URL string `json:"url"`
}

type profileFollow struct {
	socialspec.Follow
	Rkey string `json:"rkey,omitempty"`
}

type profileWire struct {
	Name      string               `json:"name,omitempty"`
	Summary   string               `json:"summary,omitempty"`
	AboutHTML string               `json:"aboutHtml,omitempty"`
	Contact   []socialspec.Contact `json:"contact,omitempty"`
	Following []profileFollow      `json:"following,omitempty"`
}

type profileResponse struct {
	Profile *profileWire `json:"profile"`
}

// ProfileFetcher reads a site's SocialSpec files; nil, nil means it publishes none.
type ProfileFetcher func(ctx context.Context, origin string) (*socialspec.Profile, error)

// profileTimeout keeps a slow site from holding the add-source sheet, which waits for the profile alongside resolve.
const profileTimeout = 6 * time.Second

// SubscriptionsProfileHandler returns the SocialSpec profile of a pasted URL's site, flagging the follows the reader already has.
func SubscriptionsProfileHandler(reader subscriptionByFeedURL, fetch ProfileFetcher, aliases FeedAliases) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body profileRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		origin, ok := webOrigin(body.URL)
		if !ok {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "url must be a website address")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), profileTimeout)
		defer cancel()
		profile, err := fetch(ctx, origin)
		if err != nil || profile == nil {
			if err != nil {
				slog.Warn("/api/subscriptions/profile: fetch failed", "origin", origin, "err", err)
			}
			writeJSON(w, profileResponse{})
			return
		}
		wire := &profileWire{Name: profile.Name, Summary: profile.Summary, AboutHTML: profile.AboutHTML, Contact: profile.Contact}
		did := sess.Data.AccountDID.String()
		for _, f := range profile.Following {
			follow := profileFollow{Follow: f}
			if row, err := findSubscription(r.Context(), reader, did, f.FeedURL, aliases); err == nil {
				follow.Rkey = row.Rkey
			}
			wire.Following = append(wire.Following, follow)
		}
		writeJSON(w, profileResponse{Profile: wire})
	})
}

func webOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

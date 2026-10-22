package websource

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"morgenblau/internal/database/db"
)

type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	Description string  `xml:"description,omitempty"`
	PubDate     string  `xml:"pubDate,omitempty"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

const (
	feedPath       = "/feed/"
	legacyFeedPath = "/web/"
)

// FeedPaths are the server paths a generated feed is served under, its id following; each needs a public route.
var FeedPaths = []string{feedPath, legacyFeedPath}

func feedID(path string) (string, bool) {
	for _, prefix := range FeedPaths {
		if id, ok := strings.CutPrefix(path, prefix); ok && id != "" && !strings.Contains(id, "/") {
			return id, true
		}
	}
	return "", false
}

// servedStampEvery bounds the writes anonymous fetches can cause; against FeedSweepGrace an hourly stamp loses nothing.
const servedStampEvery = time.Hour

// FeedHandler serves GET /feed/<id> and its legacy alias /web/<id> as RSS 2.0 to Morgenblau's own in-process fetcher; it
// reads the id from the path so it also runs outside a mux pattern.
func (s *Service) FeedHandler() http.Handler { return s.feedHandler(false) }

// PublicFeedHandler serves the same feed to outside feed readers and records that one still follows the source.
func (s *Service) PublicFeedHandler() http.Handler { return s.feedHandler(true) }

func (s *Service) feedHandler(public bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := feedID(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		src, err := s.read.GetWebSource(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			slog.Error("web feed: load source", "id", id, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if public {
			s.markServed(r.Context(), src)
		}
		rows, err := s.read.ListWebSourceFeedItems(r.Context(), db.ListWebSourceFeedItemsParams{SourceID: id, Limit: feedItems})
		if err != nil {
			slog.Error("web feed: load items", "id", id, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		body, lastModified, err := renderRSS(src, rows)
		if err != nil {
			slog.Error("web feed: render", "id", id, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sum := sha256.Sum256(body)
		etag := `"` + hex.EncodeToString(sum[:16]) + `"`
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
		if notModified(r, etag, lastModified) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(body)
	})
}

// markServed never fails the response: a missed stamp is retried by the reader's next fetch.
func (s *Service) markServed(ctx context.Context, src db.WebSource) {
	now := s.now().UTC()
	if src.ServedAt != nil {
		if last, err := time.Parse(timeLayout, *src.ServedAt); err == nil && now.Sub(last) < servedStampEvery {
			return
		}
	}
	served := db.MarkWebSourceServedParams{ID: src.ID, ServedAt: ptr(formatTime(now))}
	if err := db.New(s.writer).MarkWebSourceServed(ctx, served); err != nil {
		slog.Warn("web feed: mark served", "id", src.ID, "err", err)
	}
}

// renderRSS also returns when the served items last changed: the newest first sighting among them, since setup and polls stamp every item they add.
func renderRSS(src db.WebSource, rows []db.ListWebSourceFeedItemsRow) ([]byte, time.Time, error) {
	lastModified, _ := time.Parse(timeLayout, src.CreatedAt)
	doc := rssDoc{Version: "2.0", Channel: rssChannel{
		Title:       src.Title,
		Link:        src.SiteUrl,
		Description: "Posts from " + src.ListingUrl + ", collected by Morgenblau",
		Items:       []rssItem{},
	}}
	for _, r := range rows {
		if seen, err := time.Parse(timeLayout, r.FirstSeenAt); err == nil && seen.After(lastModified) {
			lastModified = seen
		}
		it := feedItem(r)
		item := rssItem{Title: it.Title, Link: it.URL, GUID: rssGUID{IsPermaLink: "true", Value: it.URL}, Description: it.Excerpt}
		if !it.Date.IsZero() {
			item.PubDate = it.Date.UTC().Format(time.RFC1123Z)
		}
		doc.Channel.Items = append(doc.Channel.Items, item)
	}
	body, err := xml.Marshal(doc)
	if err != nil {
		return nil, time.Time{}, err
	}
	return append([]byte(xml.Header), body...), lastModified.UTC(), nil
}

// notModified follows RFC 9110: If-None-Match decides alone when present.
func notModified(r *http.Request, etag string, lastModified time.Time) bool {
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		for tag := range strings.SplitSeq(inm, ",") {
			tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
			if tag == "*" || tag == etag {
				return true
			}
		}
		return false
	}
	since, err := http.ParseTime(r.Header.Get("If-Modified-Since"))
	return err == nil && !lastModified.Truncate(time.Second).After(since)
}

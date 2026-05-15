package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"

	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
)

// EntryReader reads entries and verifies subscription ownership.
type EntryReader interface {
	GetFeedEntry(ctx context.Context, id int64) (db.FeedEntry, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
}

// EntryExtractWriter persists readability-extracted bodies.
type EntryExtractWriter interface {
	UpdateFeedEntryExtractedBody(ctx context.Context, arg db.UpdateFeedEntryExtractedBodyParams) error
}

// EntryHandler returns the full entry for a session user. The handler also
// resolves the source title/site for convenience so frontend code doesn't
// need a second round-trip.
func EntryHandler(reader EntryReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, sess, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}
		_ = sess
		writeJSON(w, entryRowToWire(entry))
	})
}

// EntryExtractHandler runs readability on entry.url, sanitizes the result,
// persists it on the row, and returns the freshly-extracted entry. Subsequent
// calls return the cached extraction without re-fetching.
func EntryExtractHandler(reader EntryReader, writer EntryExtractWriter) http.Handler {
	sanitizer := bluemonday.UGCPolicy()
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, _, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}

		if entry.ExtractedBody != nil && *entry.ExtractedBody != "" {
			writeJSON(w, entryRowToWire(entry))
			return
		}

		extracted, err := extractReadable(r.Context(), httpClient, entry.Url, sanitizer)
		if err != nil {
			slog.Warn("/api/entries/{id}/extract: extract failed", "url", entry.Url, "err", err)
			http.Error(w, "extraction failed", http.StatusBadGateway)
			return
		}

		if err := writer.UpdateFeedEntryExtractedBody(r.Context(), db.UpdateFeedEntryExtractedBodyParams{
			ExtractedBody: &extracted,
			ID:            entry.ID,
		}); err != nil {
			slog.Warn("/api/entries/{id}/extract: persist failed", "err", err)
		}
		entry.ExtractedBody = &extracted
		writeJSON(w, entryRowToWire(entry))
	})
}

// loadAndAuthorize fetches the entry by id and verifies the session user is
// subscribed to its feed. Returns false (and writes the error) on any failure.
func loadAndAuthorize(w http.ResponseWriter, r *http.Request, reader EntryReader) (db.FeedEntry, any, bool) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil || sess.Data == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return db.FeedEntry{}, nil, false
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return db.FeedEntry{}, nil, false
	}
	entry, err := reader.GetFeedEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return db.FeedEntry{}, nil, false
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return db.FeedEntry{}, nil, false
	}
	// Authorization: the requester must subscribe to this entry's feed.
	_, err = reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
		Did:     sess.Data.AccountDID.String(),
		FeedUrl: entry.FeedUrl,
	})
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return db.FeedEntry{}, nil, false
	}
	return entry, sess, true
}

func entryRowToWire(row db.FeedEntry) EntryWire {
	body := row.ContentHtml
	if row.ExtractedBody != nil && *row.ExtractedBody != "" {
		body = row.ExtractedBody
	}
	src := SourceMeta{FeedURL: row.FeedUrl}
	return EntryWire{
		ID:          row.ID,
		Title:       row.Title,
		URL:         row.Url,
		ContentType: row.ContentType,
		PublishedAt: row.PublishedAt,
		Source:      src,
		Body:        body,
		Metadata:    row.Metadata,
	}
}

func extractReadable(ctx context.Context, client *http.Client, rawURL string, sanitizer *bluemonday.Policy) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Morgenblau/0.1 (+https://morgen.blue/about; bot@morgen.blue) Go-http-client/1.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("upstream " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", err
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	article, err := readability.FromReader(strings.NewReader(string(body)), parsedURL)
	if err != nil {
		return "", err
	}
	return sanitizer.Sanitize(article.Content), nil
}

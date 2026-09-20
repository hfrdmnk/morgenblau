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

	readability "github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"

	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/newsletter"
	"morgenblau/internal/safehttp"
)

// EntryReader reads entries, verifies subscription ownership, and looks up whether the requester saved this entry.
type EntryReader interface {
	GetFeedEntryBySlug(ctx context.Context, slug string) (db.FeedEntry, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
	GetFeed(ctx context.Context, feedURL string) (db.Feed, error)
	GetUserSaveByItemURL(ctx context.Context, arg db.GetUserSaveByItemURLParams) (db.UserSave, error)
}

// EntryExtractWriter persists readability-extracted bodies.
type EntryExtractWriter interface {
	UpdateFeedEntryExtractedBody(ctx context.Context, arg db.UpdateFeedEntryExtractedBodyParams) error
}

type newsletterEntryReader interface {
	GetMessageBySlug(context.Context, string, string) (newsletter.Message, error)
}

// EntryHandler returns the full entry plus source metadata and saved-state, avoiding a second round-trip for the frontend.
func EntryHandler(reader EntryReader, privateReaders ...newsletterEntryReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(privateReaders) > 0 && privateReaders[0] != nil {
			sess, ok := requireSession(w, r)
			if !ok {
				return
			}
			message, err := privateReaders[0].GetMessageBySlug(r.Context(), sess.Data.AccountDID.String(), r.PathValue("slug"))
			if err == nil {
				privateResponse(w)
				writeJSON(w, newsletterMessageToWire(message))
				return
			}
			if !errors.Is(err, newsletter.ErrNotFound) {
				privateResponse(w)
				writeNewsletterError(w, err)
				return
			}
		}
		entry, sub, feed, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}
		saved := lookupSavedState(r.Context(), reader, entry.Url)
		writeJSON(w, entryRowToWire(entry, sub, feed, saved))
	})
}

// EntryExtractHandler runs readability extraction on entry.url, sanitizes it, and caches the result.
// httpClient must be a safehttp-built client, since entry.url is attacker-controlled and could pivot to internal hosts.
func EntryExtractHandler(reader EntryReader, writer EntryExtractWriter, httpClient *http.Client) http.Handler {
	sanitizer := bluemonday.UGCPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, sub, feed, ok := loadAndAuthorize(w, r, reader)
		if !ok {
			return
		}

		saved := lookupSavedState(r.Context(), reader, entry.Url)

		if entry.ExtractedBody != nil && *entry.ExtractedBody != "" {
			writeJSON(w, entryRowToWire(entry, sub, feed, saved))
			return
		}

		// Path-less standardfeed documents have no canonical URL to extract from (body was prefilled at ingest), so return as-is rather than fetching "".
		if entry.Url == "" {
			writeJSON(w, entryRowToWire(entry, sub, feed, saved))
			return
		}

		extracted, err := extractReadable(r.Context(), httpClient, entry.Url, sanitizer)
		if err != nil {
			slog.Warn("/api/entries/{id}/extract: extract failed", "url", entry.Url, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "extraction failed")
			return
		}

		if err := writer.UpdateFeedEntryExtractedBody(r.Context(), db.UpdateFeedEntryExtractedBodyParams{
			ExtractedBody: &extracted,
			ID:            entry.ID,
		}); err != nil {
			slog.Warn("/api/entries/{id}/extract: persist failed", "err", err)
		}
		entry.ExtractedBody = &extracted
		writeJSON(w, entryRowToWire(entry, sub, feed, saved))
	})
}

// loadAndAuthorize fetches the entry by slug, verifies feed subscription, and resolves feed metadata, writing the error and returning false on any failure.
func loadAndAuthorize(w http.ResponseWriter, r *http.Request, reader EntryReader) (db.FeedEntry, db.UserSubscription, db.Feed, bool) {
	sess, ok := requireSession(w, r)
	if !ok {
		return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
	}
	slug := r.PathValue("slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid slug")
		return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
	}
	entry, err := reader.GetFeedEntryBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
		}
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
	}
	// Not subscribed collapses to 404, same as every other missing-or-not-owned resource.
	sub, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
		Did:     sess.Data.AccountDID.String(),
		FeedUrl: entry.FeedUrl,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
		}
		writeError(w, http.StatusNotFound, codeNotFound, "not found")
		return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
	}
	// A missing feed row is unexpected (a subscription implies one) but doesn't fail the request; the frontend renders fallbacks.
	feed, err := reader.GetFeed(r.Context(), entry.FeedUrl)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return db.FeedEntry{}, db.UserSubscription{}, db.Feed{}, false
	}
	return entry, sub, feed, true
}

func entryRowToWire(row db.FeedEntry, sub db.UserSubscription, feed db.Feed, saved *SavedState) EntryWire {
	body := row.ContentHtml
	if row.ExtractedBody != nil && *row.ExtractedBody != "" {
		body = row.ExtractedBody
	}
	source := buildSourceMeta(row.FeedUrl, displayTitle(sub.Title, feed.Title), feed.SiteUrl, feed.IconUrl)
	source.Rkey = sub.Rkey
	entryURL := row.Url
	return EntryWire{
		ID:          row.ID,
		EntrySlug:   row.EntrySlug,
		Title:       row.Title,
		URL:         &entryURL,
		ContentType: row.ContentType,
		PublishedAt: row.PublishedAt,
		Source:      source,
		Body:        body,
		Metadata:    row.Metadata,
		SavedState:  saved,
	}
}

// lookupSavedState returns the save record for itemURL, or nil on a missing row or any other error (the save button degrades to unsaved rather than failing the page).
func lookupSavedState(ctx context.Context, reader EntryReader, itemURL string) *SavedState {
	sess := auth.SessionFromContext(ctx)
	if sess == nil || sess.Data == nil {
		return nil
	}
	row, err := reader.GetUserSaveByItemURL(ctx, db.GetUserSaveByItemURLParams{
		Did:     sess.Data.AccountDID.String(),
		ItemUrl: itemURL,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("entries: saved-state lookup failed", "err", err)
		}
		return nil
	}
	return &SavedState{Rkey: row.Rkey}
}

func extractReadable(ctx context.Context, client *http.Client, rawURL string, sanitizer *bluemonday.Policy) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
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

package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/standardfeed"
)

type StandardfeedSource interface {
	GetPublication(ctx context.Context, uri string) (*standardfeed.Publication, error)
	ListDocuments(ctx context.Context, pubURI string) ([]standardfeed.Document, error)
}

type stdPipelineQueries interface {
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	SetFeedIconURL(ctx context.Context, arg db.SetFeedIconURLParams) error
	UpdateFeedFetchState(ctx context.Context, arg db.UpdateFeedFetchStateParams) error
	ListFeedEntriesForDiff(ctx context.Context, feedURL string) ([]db.ListFeedEntriesForDiffRow, error)
	UpsertStandardfeedEntry(ctx context.Context, arg db.UpsertStandardfeedEntryParams) error
	DeleteFeedEntry(ctx context.Context, arg db.DeleteFeedEntryParams) error
}

type StandardfeedPipeline struct {
	source    StandardfeedSource
	queries   stdPipelineQueries
	sanitizer *bluemonday.Policy
	now       func() time.Time
	runTx     func(ctx context.Context, fn func(stdPipelineQueries) error) error
}

func NewStandardfeedPipeline(source StandardfeedSource, q stdPipelineQueries) *StandardfeedPipeline {
	p := &StandardfeedPipeline{
		source:    source,
		queries:   q,
		sanitizer: bluemonday.UGCPolicy(),
		now:       time.Now,
	}
	p.runTx = func(ctx context.Context, fn func(stdPipelineQueries) error) error {
		return fn(p.queries)
	}
	return p
}

func (p *StandardfeedPipeline) WithTxRunner(w *sql.DB) *StandardfeedPipeline {
	p.runTx = func(ctx context.Context, fn func(stdPipelineQueries) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return p
}

func (p *StandardfeedPipeline) FetchAndStore(ctx context.Context, pubURI string) error {
	pub, err := p.source.GetPublication(ctx, pubURI)
	if err != nil {
		return err
	}
	docs, err := p.source.ListDocuments(ctx, pubURI)
	if err != nil {
		return err
	}

	nowStr := p.now().UTC().Format(time.RFC3339)

	// SQLite backs out only a failing statement and keeps the transaction active, so a failed entry write leaves its siblings in the batch.
	// https://www.sqlite.org/lang_conflict.html
	return p.runTx(ctx, func(q stdPipelineQueries) error {
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   pubURI,
			Kind:      "standardfeed",
			SiteUrl:   nilIfEmpty(pub.URL),
			Title:     nilIfEmpty(pub.Name),
			CreatedAt: nowStr,
			UpdatedAt: nowStr,
		}); err != nil {
			slog.Warn("standardpipeline: feed upsert failed", "publication", pubURI, "err", err)
		}
		if pub.IconURL != "" {
			if err := q.SetFeedIconURL(ctx, db.SetFeedIconURLParams{
				IconUrl:       &pub.IconURL,
				IconFetchedAt: &nowStr,
				UpdatedAt:     nowStr,
				FeedUrl:       pubURI,
			}); err != nil {
				slog.Warn("standardpipeline: icon persist failed", "publication", pubURI, "err", err)
			}
		}
		if err := q.UpdateFeedFetchState(ctx, db.UpdateFeedFetchStateParams{
			LastFetchedAt: &nowStr,
			UpdatedAt:     nowStr,
			FeedUrl:       pubURI,
		}); err != nil {
			slog.Warn("standardpipeline: fetch state update failed", "publication", pubURI, "err", err)
		}

		local, err := q.ListFeedEntriesForDiff(ctx, pubURI)
		if err != nil {
			return err
		}
		localCID := make(map[string]*string, len(local))
		for _, row := range local {
			localCID[row.Guid] = row.RecordCid
		}

		remote := make(map[string]bool, len(docs))
		for _, doc := range docs {
			remote[doc.URI] = true
			if doc.Title == "" || doc.PublishedAt == "" {
				slog.Warn("standardpipeline: skipping malformed document", "uri", doc.URI)
				continue
			}
			if cid, known := localCID[doc.URI]; known && cid != nil && *cid == doc.CID {
				continue
			}
			if err := q.UpsertStandardfeedEntry(ctx, p.entryParams(pub, doc, pubURI, nowStr)); err != nil {
				slog.Warn("standardpipeline: entry upsert failed", "publication", pubURI, "doc", doc.URI, "err", err)
			}
		}

		for guid := range localCID {
			if remote[guid] {
				continue
			}
			if err := q.DeleteFeedEntry(ctx, db.DeleteFeedEntryParams{FeedUrl: pubURI, Guid: guid}); err != nil {
				slog.Warn("standardpipeline: entry delete failed", "publication", pubURI, "guid", guid, "err", err)
			}
		}
		return nil
	})
}

func (p *StandardfeedPipeline) entryParams(pub *standardfeed.Publication, doc standardfeed.Document, pubURI, nowStr string) db.UpsertStandardfeedEntryParams {
	summary := doc.Description
	if summary == "" {
		summary = doc.TextContent
	}

	var extracted *string
	if doc.Path == "" && doc.TextContent != "" {
		extracted = nilIfEmpty(plaintextToHTML(doc.TextContent))
	}

	var metaJSON *string
	if doc.CoverImageURL != "" {
		if raw, err := json.Marshal(map[string]any{"image": doc.CoverImageURL}); err == nil {
			s := string(raw)
			metaJSON = &s
		}
	}

	cid := doc.CID
	return db.UpsertStandardfeedEntryParams{
		FeedUrl:       pubURI,
		Guid:          doc.URI,
		EntrySlug:     EntrySlug(pubURI, doc.URI),
		Url:           canonicalDocumentURL(pub.URL, doc.Path),
		Title:         nilIfEmpty(strings.TrimSpace(doc.Title)),
		ContentHtml:   nilIfEmpty(p.sanitizer.Sanitize(summary)),
		ContentType:   "blogpost",
		PublishedAt:   normalizeTime(doc.PublishedAt, p.now()),
		FetchedAt:     nowStr,
		Metadata:      metaJSON,
		ExtractedBody: extracted,
		RecordCid:     &cid,
	}
}

func canonicalDocumentURL(baseURL, path string) string {
	if path == "" || baseURL == "" {
		return ""
	}
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func plaintextToHTML(text string) string {
	blocks := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	var b strings.Builder
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(block), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

func normalizeTime(raw string, fallback time.Time) string {
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw)); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return fallback.UTC().Format(time.RFC3339)
}

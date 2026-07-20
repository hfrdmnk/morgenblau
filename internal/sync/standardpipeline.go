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
	"morgenblau/internal/discoverlang"
	"morgenblau/internal/standardfeed"
)

// StandardfeedSource is the slice of *standardfeed.Client the pipeline uses.
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

// StandardfeedPipeline implements FeedFetcher for standardfeed rows: a
// listRecords diff where documents missing upstream are hard-deleted.
type StandardfeedPipeline struct {
	source    StandardfeedSource
	queries   stdPipelineQueries
	sanitizer *bluemonday.Policy
	now       func() time.Time
	detector  discoverlang.Detector
	runTx     func(ctx context.Context, fn func(stdPipelineQueries) error) error
}

func NewStandardfeedPipeline(source StandardfeedSource, q stdPipelineQueries) *StandardfeedPipeline {
	p := &StandardfeedPipeline{
		source:    source,
		queries:   q,
		sanitizer: bluemonday.UGCPolicy(),
		now:       time.Now,
		detector:  discoverlang.NewDetector(),
	}
	// Default: no transaction (keeps fake-based tests working).
	p.runTx = func(ctx context.Context, fn func(stdPipelineQueries) error) error {
		return fn(p.queries)
	}
	return p
}

// WithTxRunner commits each publication's diff batch in one transaction on the writer pool.
func (p *StandardfeedPipeline) WithTxRunner(w *sql.DB) *StandardfeedPipeline {
	p.runTx = func(ctx context.Context, fn func(stdPipelineQueries) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return p
}

func (p *StandardfeedPipeline) FetchAndStore(ctx context.Context, pubURI string) error {
	// Both network calls precede the transaction: deletes only run against a
	// complete remote snapshot, and the writer connection is never held across them.
	pub, err := p.source.GetPublication(ctx, pubURI)
	if err != nil {
		return err
	}
	docs, err := p.source.ListDocuments(ctx, pubURI)
	if err != nil {
		return err
	}

	nowStr := p.now().UTC().Format(time.RFC3339)

	// SPEC <discovery>: only RSS feeds carry a language tag; standardfeed docs get content-only detection.
	language := languageOrNil(p.detector, standardLanguageSample(docs), "")

	// The diff read rides inside the tx for a consistent snapshot; per-entry
	// write errors are tolerated (log-and-continue) so one bad document doesn't roll back the batch.
	return p.runTx(ctx, func(q stdPipelineQueries) error {
		if err := q.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   pubURI,
			Kind:      "standardfeed",
			SiteUrl:   nilIfEmpty(pub.URL),
			Title:     nilIfEmpty(pub.Name),
			Language:  language,
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
			return err // in-tx read failure: roll back the batch
		}
		localCID := make(map[string]*string, len(local))
		for _, row := range local {
			localCID[row.Guid] = row.RecordCid
		}

		remote := make(map[string]bool, len(docs))
		for _, doc := range docs {
			// Marked present before validation: a transiently-malformed record still
			// exists upstream, so it must not trip the delete sweep and drop a good cached entry.
			remote[doc.URI] = true
			if doc.Title == "" || doc.PublishedAt == "" {
				slog.Warn("standardpipeline: skipping malformed document", "uri", doc.URI)
				continue
			}
			if cid, known := localCID[doc.URI]; known && cid != nil && *cid == doc.CID {
				continue // unchanged; never touch the row (protects cached extractions)
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

	// Path-less documents prefill extracted_body with plaintext so the reader's
	// cache short-circuit serves it without a fetch; path-ful documents get NULL so a CID change forces re-extraction.
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

// canonicalDocumentURL returns "" for a path-less document (no canonical URL).
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

// plaintextToHTML escapes and paragraph-wraps plaintext so the reader renders textContent fallbacks legibly.
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

// standardLanguageSample builds a plain-text sample for language detection, bounded the same way languageSample bounds RSS items.
func standardLanguageSample(docs []standardfeed.Document) string {
	var b strings.Builder
	for i, doc := range docs {
		if i >= languageSampleMaxItems || b.Len() >= languageSampleMaxBytes {
			break
		}
		b.WriteString(doc.Title)
		b.WriteString(" ")
		summary := doc.Description
		if summary == "" {
			summary = doc.TextContent
		}
		b.WriteString(summary)
		b.WriteString(" ")
	}
	sample := b.String()
	if len(sample) > languageSampleMaxBytes {
		sample = sample[:languageSampleMaxBytes]
	}
	return sample
}

// normalizeTime reformats an atproto datetime as UTC RFC3339 so published_at sorts lexicographically alongside RSS entries; unparsable values fall back to now.
func normalizeTime(raw string, fallback time.Time) string {
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw)); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return fallback.UTC().Format(time.RFC3339)
}

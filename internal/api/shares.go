package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/sharemeta"
	"morgenblau/internal/standardfeed"
)

const (
	shareCollection     = lexicon.Share
	recommendCollection = standardfeed.CollectionRecommend
)

// maxCommentGraphemes mirrors the share lexicon's comment cap; rune count approximates graphemes, same as normalizeTags.
const maxCommentGraphemes = 3000

// ShareWire is the on-the-wire share shape; GET /api/shares additionally joins entry title/slug when cached.
type ShareWire struct {
	URI       string `json:"uri,omitempty"`
	CID       string `json:"cid,omitempty"`
	Rkey      string `json:"rkey"`
	Kind      string `json:"kind"`
	ItemURL   string `json:"itemUrl,omitempty"`
	Document  string `json:"document,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"createdAt"`
	Title     string `json:"title,omitempty"`
	TargetURL string `json:"targetUrl,omitempty"`
	EntrySlug string `json:"entrySlug,omitempty"`
}

// SharesIndexReader is the slice of db.Queries the share handlers read from.
type SharesIndexReader interface {
	GetFeedEntryBySlug(ctx context.Context, slug string) (db.FeedEntry, error)
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
	GetUserShare(ctx context.Context, arg db.GetUserShareParams) (db.UserShare, error)
	GetUserShareByItemURL(ctx context.Context, arg db.GetUserShareByItemURLParams) (db.UserShare, error)
	GetUserShareByDocument(ctx context.Context, arg db.GetUserShareByDocumentParams) (db.UserShare, error)
	ListUserShares(ctx context.Context, did string) ([]db.ListUserSharesRow, error)
}

// SharesIndexWriter is the slice used for writes.
type SharesIndexWriter interface {
	UpsertUserShare(ctx context.Context, arg db.UpsertUserShareParams) error
	DeleteUserShare(ctx context.Context, arg db.DeleteUserShareParams) error
}

// --- POST /api/shares ---

type sharesCreateRequest struct {
	EntrySlug string `json:"entrySlug"`
	Comment   string `json:"comment"`
}

// SharesCreateHandler picks the record model by subscription kind: rss writes one share record; standardfeed writes a recommend plus a lazy comment sidecar. Idempotent on itemUrl (rss) or document (standardfeed).
func SharesCreateHandler(reader SharesIndexReader, writer SharesIndexWriter, pds atprepo.Writer, disp RepairDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body sharesCreateRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.EntrySlug = strings.TrimSpace(body.EntrySlug)
		body.Comment = strings.TrimSpace(body.Comment)
		if body.EntrySlug == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "entrySlug is required")
			return
		}
		if len([]rune(body.Comment)) > maxCommentGraphemes {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "comment is too long")
			return
		}

		entry, err := reader.GetFeedEntryBySlug(r.Context(), body.EntrySlug)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/shares: entry load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		sub, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
			Did:     sess.Data.AccountDID.String(),
			FeedUrl: entry.FeedUrl,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Not subscribed collapses to 404, same as every other missing-or-not-owned resource.
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/shares: authorize failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if wireKind(sub.Kind) == "standardfeed" {
			shareStandardfeed(w, r, reader, writer, pds, disp, sess, entry, body.Comment, now)
			return
		}
		shareRSS(w, r, reader, writer, pds, disp, sess, entry, body.Comment, now)
	})
}

func shareRSS(
	w http.ResponseWriter, r *http.Request,
	reader SharesIndexReader, writer SharesIndexWriter, pds atprepo.Writer, disp RepairDispatcher,
	sess *oauth.ClientSession, entry db.FeedEntry, comment, now string,
) {
	didStr := sess.Data.AccountDID.String()

	// itemUrl is lexicon-required and the dedupe key; without it, repeat POSTs would each write an undeduped invalid record.
	if entry.Url == "" {
		writeError(w, http.StatusUnprocessableEntity, codeUnprocessable, "this item has no link to share")
		return
	}

	// Dedupe on (did, itemUrl): a second share of the same item is idempotent.
	if existing, err := reader.GetUserShareByItemURL(r.Context(), db.GetUserShareByItemURLParams{
		Did:     didStr,
		ItemUrl: nilIfEmpty(entry.Url),
	}); err == nil {
		writeJSON(w, shareRowToWire(existing))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("/api/shares: rss dedupe probe failed", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return
	}

	record := map[string]any{"itemUrl": entry.Url, "createdAt": now}
	if entry.FeedUrl != "" {
		record["feedUrl"] = entry.FeedUrl
	}
	if comment != "" {
		record["comment"] = comment
	}
	if err := lexicon.ValidateRecord(shareCollection, record); err != nil {
		slog.Warn("/api/shares: rss record failed lexicon validation", "err", err)
		writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
		return
	}
	ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(shareCollection), record)
	if err != nil {
		slog.Warn("/api/shares: rss PDS create failed", "err", err)
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return
	}
	rkey := atprepo.RkeyFromATURI(ref.URI)

	mirrorOrRepair(r.Context(), disp, sess, "/api/shares: rss Tier-1 upsert", func() error {
		return writer.UpsertUserShare(r.Context(), db.UpsertUserShareParams{
			Did:       didStr,
			Rkey:      rkey,
			AtUri:     ref.URI,
			Kind:      "rss",
			ItemUrl:   nilIfEmpty(entry.Url),
			Comment:   nilIfEmpty(comment),
			FeedUrl:   nilIfEmpty(entry.FeedUrl),
			CreatedAt: now,
			UpdatedAt: now,
		})
	})

	writeJSONStatus(w, http.StatusCreated, ShareWire{
		URI:       ref.URI,
		CID:       ref.CID,
		Rkey:      rkey,
		Kind:      "rss",
		ItemURL:   entry.Url,
		Comment:   comment,
		CreatedAt: now,
		Title:     derefStr(entry.Title),
		TargetURL: entry.Url,
		EntrySlug: entry.EntrySlug,
	})
}

func shareStandardfeed(
	w http.ResponseWriter, r *http.Request,
	reader SharesIndexReader, writer SharesIndexWriter, pds atprepo.Writer, disp RepairDispatcher,
	sess *oauth.ClientSession, entry db.FeedEntry, comment, now string,
) {
	if !requireStandardWrite(w, sess) {
		return
	}
	didStr := sess.Data.AccountDID.String()
	document := entry.Guid // the entry guid is the document at-uri

	// Dedupe on (did, document): a second recommend of the same document is idempotent, comment or not.
	if existing, err := reader.GetUserShareByDocument(r.Context(), db.GetUserShareByDocumentParams{
		Did:      didStr,
		Document: &document,
	}); err == nil {
		writeJSON(w, shareRowToWire(existing))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("/api/shares: standardfeed dedupe probe failed", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return
	}

	// A path-less document has no URL for the comment sidecar's required itemUrl, so recommend-only is fine but recommend+comment is not.
	if entry.Url == "" && comment != "" {
		writeError(w, http.StatusUnprocessableEntity, codeUnprocessable, "this document has no link to comment on; share it without a comment")
		return
	}

	// Recommend is created first (the existence authority); if the sidecar then fails, reconcile can still adopt the durable recommend comment-less.
	recRef, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(recommendCollection), map[string]any{
		"document":  document,
		"createdAt": now,
	})
	if err != nil {
		slog.Warn("/api/shares: recommend PDS create failed", "err", err)
		writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
		return
	}
	recRkey := atprepo.RkeyFromATURI(recRef.URI)

	var sidecarRkey *string
	if comment != "" {
		record := map[string]any{
			"itemUrl":   entry.Url,
			"document":  document,
			"comment":   comment,
			"createdAt": now,
		}
		if entry.FeedUrl != "" {
			record["feedUrl"] = entry.FeedUrl
		}
		if err := lexicon.ValidateRecord(shareCollection, record); err != nil {
			slog.Warn("/api/shares: comment sidecar record failed lexicon validation", "err", err)
			writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
			return
		}
		scRef, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(shareCollection), record)
		if err != nil {
			slog.Warn("/api/shares: comment sidecar PDS create failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		rk := atprepo.RkeyFromATURI(scRef.URI)
		sidecarRkey = &rk
	}

	mirrorOrRepair(r.Context(), disp, sess, "/api/shares: standardfeed Tier-1 upsert", func() error {
		return writer.UpsertUserShare(r.Context(), db.UpsertUserShareParams{
			Did:         didStr,
			Rkey:        recRkey,
			AtUri:       recRef.URI,
			Kind:        "standardfeed",
			ItemUrl:     nilIfEmpty(entry.Url),
			Document:    &document,
			Comment:     nilIfEmpty(comment),
			FeedUrl:     nilIfEmpty(entry.FeedUrl),
			SidecarRkey: sidecarRkey,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	})

	writeJSONStatus(w, http.StatusCreated, ShareWire{
		URI:       recRef.URI,
		CID:       recRef.CID,
		Rkey:      recRkey,
		Kind:      "standardfeed",
		ItemURL:   entry.Url,
		Document:  document,
		Comment:   comment,
		CreatedAt: now,
		Title:     derefStr(entry.Title),
		TargetURL: entry.Url,
		EntrySlug: entry.EntrySlug,
	})
}

// --- DELETE /api/shares/{rkey} ---

// SharesDeleteHandler tombstones the share; for standardfeed it also sweeps every recommend for the document (a stray duplicate would let reconcile resurrect the share) plus the comment sidecar.
func SharesDeleteHandler(reader SharesIndexReader, writer SharesIndexWriter, pds RepoWriterLister, disp RepairDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "rkey is required")
			return
		}
		didStr := sess.Data.AccountDID.String()
		row, err := reader.GetUserShare(r.Context(), db.GetUserShareParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Collapse "not yours" and "doesn't exist" to 404, same as saves.
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/shares DELETE: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		if wireKind(row.Kind) == "standardfeed" {
			if !requireStandardWrite(w, sess) {
				return
			}
			// Leaving a duplicate recommend would let the next reconcile re-adopt it and resurrect the share.
			document := derefStr(row.Document)
			records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(recommendCollection))
			if err != nil {
				slog.Warn("/api/shares DELETE: recommend list failed", "err", err)
				writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
				return
			}
			for _, rec := range records {
				if doc, _ := rec.Value["document"].(string); doc != document {
					continue
				}
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(recommendCollection), atprepo.RkeyFromATURI(rec.URI)); err != nil {
					slog.Warn("/api/shares DELETE: recommend PDS delete failed", "uri", rec.URI, "err", err)
					writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
					return
				}
			}
			if row.SidecarRkey != nil && *row.SidecarRkey != "" {
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(shareCollection), *row.SidecarRkey); err != nil {
					slog.Warn("/api/shares DELETE: sidecar PDS delete failed", "err", err)
					writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
					return
				}
			}
		} else if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(shareCollection), rkey); err != nil {
			slog.Warn("/api/shares DELETE: rss PDS delete failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}

		mirrorOrRepair(r.Context(), disp, sess, "/api/shares DELETE: Tier-1 delete", func() error {
			return writer.DeleteUserShare(r.Context(), db.DeleteUserShareParams{Did: didStr, Rkey: rkey})
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

// --- GET /api/shares ---

// SharesListHandler returns the user's shares, newest first, joining entry title/slug when the entry is still cached.
func SharesListHandler(reader SharesIndexReader, metadata ShareMetadataResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rows, err := reader.ListUserShares(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/shares GET: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		targets := make([]sharemeta.Target, len(rows))
		for i, row := range rows {
			targets[i] = sharemeta.Target{
				ItemURL: derefStr(row.ItemUrl), Document: derefStr(row.Document),
			}
		}
		resolved := resolveShareMetadata(r.Context(), metadata, targets)
		out := make([]ShareWire, 0, len(rows))
		for i, row := range rows {
			out = append(out, shareListRowToWire(row, resolved[i]))
		}
		writeJSON(w, out)
	})
}

func shareRowToWire(row db.UserShare) ShareWire {
	return ShareWire{
		URI:       row.AtUri,
		Rkey:      row.Rkey,
		Kind:      wireKind(row.Kind),
		ItemURL:   derefStr(row.ItemUrl),
		Document:  derefStr(row.Document),
		Comment:   derefStr(row.Comment),
		CreatedAt: row.CreatedAt,
	}
}

func shareListRowToWire(row db.ListUserSharesRow, metadata sharemeta.Metadata) ShareWire {
	wire := ShareWire{
		URI:       row.AtUri,
		Rkey:      row.Rkey,
		Kind:      wireKind(row.Kind),
		ItemURL:   derefStr(row.ItemUrl),
		Document:  derefStr(row.Document),
		Comment:   derefStr(row.Comment),
		CreatedAt: row.CreatedAt,
		Title:     derefStr(row.EntryTitle),
		EntrySlug: derefStr(row.EntrySlug),
	}
	if metadata.Title != "" {
		wire.Title = metadata.Title
	}
	if metadata.TargetURL != "" {
		wire.TargetURL = metadata.TargetURL
	}
	if metadata.EntrySlug != "" {
		wire.EntrySlug = metadata.EntrySlug
	}
	return wire
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

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

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/standardfeed"
)

const (
	shareCollection     = lexicon.Share
	recommendCollection = standardfeed.CollectionRecommend
)

// maxCommentGraphemes mirrors the blue.morgen.feed.share lexicon's comment cap.
// Rune-count is close enough without a full Unicode segmentation dep — same
// approximation normalizeTags uses.
const maxCommentGraphemes = 3000

// ShareWire is the on-the-wire share shape. POST returns the freshly-created
// record; GET /api/shares returns the list with entry title/slug joined in for
// document shares whose entry is still cached.
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

// SharesCreateHandler shares a feed item. The subscription's kind selects the
// record model: an rss item becomes a single blue.morgen.feed.share record; a
// Standardfeed document becomes a site.standard.graph.recommend existence
// record plus a lazy blue.morgen.feed.share comment sidecar (only when the user
// wrote a comment). Idempotent per (did, itemUrl) for rss and (did, document)
// for standardfeed.
func SharesCreateHandler(reader SharesIndexReader, writer SharesIndexWriter, pds atprepo.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var body sharesCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		body.EntrySlug = strings.TrimSpace(body.EntrySlug)
		body.Comment = strings.TrimSpace(body.Comment)
		if body.EntrySlug == "" {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"message": "entrySlug is required"})
			return
		}
		if len([]rune(body.Comment)) > maxCommentGraphemes {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"message": "comment is too long"})
			return
		}

		// Load + authorize: the entry must exist and the requester must be
		// subscribed to its feed. The subscription's kind picks the record model.
		entry, err := reader.GetFeedEntryBySlug(r.Context(), body.EntrySlug)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			slog.Warn("/api/shares: entry load failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sub, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
			Did:     sess.Data.AccountDID.String(),
			FeedUrl: entry.FeedUrl,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			slog.Warn("/api/shares: authorize failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if wireKind(sub.Kind) == "standardfeed" {
			shareStandardfeed(w, r, reader, writer, pds, sess, entry, body.Comment, now)
			return
		}
		shareRSS(w, r, reader, writer, pds, sess, entry, body.Comment, now)
	})
}

func shareRSS(
	w http.ResponseWriter, r *http.Request,
	reader SharesIndexReader, writer SharesIndexWriter, pds atprepo.Writer,
	sess *oauth.ClientSession, entry db.FeedEntry, comment, now string,
) {
	didStr := sess.Data.AccountDID.String()

	// itemUrl is lexicon-required and the dedupe key; a link-less entry would
	// write an invalid record that never dedupes, so repeat POSTs would pile up.
	if entry.Url == "" {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{
			"message": "this item has no link to share",
		})
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
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	record := map[string]any{"itemUrl": entry.Url, "createdAt": now}
	if entry.FeedUrl != "" {
		record["feedUrl"] = entry.FeedUrl
	}
	if comment != "" {
		record["comment"] = comment
	}
	ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(shareCollection), record)
	if err != nil {
		slog.Warn("/api/shares: rss PDS create failed", "err", err)
		http.Error(w, "upstream PDS error", http.StatusBadGateway)
		return
	}
	rkey := atprepo.RkeyFromATURI(ref.URI)

	if err := writer.UpsertUserShare(r.Context(), db.UpsertUserShareParams{
		Did:       didStr,
		Rkey:      rkey,
		AtUri:     ref.URI,
		Kind:      "rss",
		ItemUrl:   nilIfEmpty(entry.Url),
		Comment:   nilIfEmpty(comment),
		FeedUrl:   nilIfEmpty(entry.FeedUrl),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		slog.Warn("/api/shares: rss Tier-1 upsert failed (PDS write succeeded)", "err", err)
	}

	writeJSONStatus(w, http.StatusCreated, ShareWire{
		URI:       ref.URI,
		CID:       ref.CID,
		Rkey:      rkey,
		Kind:      "rss",
		ItemURL:   entry.Url,
		Comment:   comment,
		CreatedAt: now,
	})
}

func shareStandardfeed(
	w http.ResponseWriter, r *http.Request,
	reader SharesIndexReader, writer SharesIndexWriter, pds atprepo.Writer,
	sess *oauth.ClientSession, entry db.FeedEntry, comment, now string,
) {
	if !requireStandardWrite(w, sess) {
		return
	}
	didStr := sess.Data.AccountDID.String()
	document := entry.Guid // the entry guid IS the document at-uri

	// Dedupe on (did, document): a second recommend of the same document is
	// idempotent, comment or not.
	if existing, err := reader.GetUserShareByDocument(r.Context(), db.GetUserShareByDocumentParams{
		Did:      didStr,
		Document: &document,
	}); err == nil {
		writeJSON(w, shareRowToWire(existing))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("/api/shares: standardfeed dedupe probe failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// A path-less document has no URL to satisfy the comment sidecar's required
	// itemUrl. Recommend-only is fine; recommend + comment is not.
	if entry.Url == "" && comment != "" {
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]string{
			"message": "this document has no link to comment on; share it without a comment",
		})
		return
	}

	// Recommend FIRST — the existence authority. If the sidecar then fails, the
	// recommend is already durable and reconcile adopts it comment-less.
	recRef, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(recommendCollection), map[string]any{
		"document":  document,
		"createdAt": now,
	})
	if err != nil {
		slog.Warn("/api/shares: recommend PDS create failed", "err", err)
		http.Error(w, "upstream PDS error", http.StatusBadGateway)
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
		scRef, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(shareCollection), record)
		if err != nil {
			slog.Warn("/api/shares: comment sidecar PDS create failed", "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		rk := atprepo.RkeyFromATURI(scRef.URI)
		sidecarRkey = &rk
	}

	if err := writer.UpsertUserShare(r.Context(), db.UpsertUserShareParams{
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
	}); err != nil {
		slog.Warn("/api/shares: standardfeed Tier-1 upsert failed (PDS write succeeded)", "err", err)
	}

	writeJSONStatus(w, http.StatusCreated, ShareWire{
		URI:       recRef.URI,
		CID:       recRef.CID,
		Rkey:      recRkey,
		Kind:      "standardfeed",
		ItemURL:   entry.Url,
		Document:  document,
		Comment:   comment,
		CreatedAt: now,
	})
}

// --- DELETE /api/shares/{rkey} ---

// SharesDeleteHandler tombstones the share on the PDS and removes the local row.
// For standardfeed shares it deletes EVERY recommend matching the document
// (a duplicate written by another app would otherwise resurrect the share on the
// next reconcile) plus the comment sidecar.
func SharesDeleteHandler(reader SharesIndexReader, writer SharesIndexWriter, pds RepoWriterLister) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rkey := r.PathValue("rkey")
		if rkey == "" {
			http.Error(w, "rkey is required", http.StatusBadRequest)
			return
		}
		didStr := sess.Data.AccountDID.String()
		row, err := reader.GetUserShare(r.Context(), db.GetUserShareParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Collapse "not yours" and "doesn't exist" to 403, same as saves.
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			slog.Warn("/api/shares DELETE: load failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if wireKind(row.Kind) == "standardfeed" {
			if !requireStandardWrite(w, sess) {
				return
			}
			// Sweep every recommend for this document; leaving a duplicate would
			// let the next reconcile re-adopt it and resurrect the share.
			document := derefStr(row.Document)
			records, err := pds.ListRecords(r.Context(), sess, syntax.NSID(recommendCollection))
			if err != nil {
				slog.Warn("/api/shares DELETE: recommend list failed", "err", err)
				http.Error(w, "upstream PDS error", http.StatusBadGateway)
				return
			}
			for _, rec := range records {
				if doc, _ := rec.Value["document"].(string); doc != document {
					continue
				}
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(recommendCollection), atprepo.RkeyFromATURI(rec.URI)); err != nil {
					slog.Warn("/api/shares DELETE: recommend PDS delete failed", "uri", rec.URI, "err", err)
					http.Error(w, "upstream PDS error", http.StatusBadGateway)
					return
				}
			}
			if row.SidecarRkey != nil && *row.SidecarRkey != "" {
				if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(shareCollection), *row.SidecarRkey); err != nil {
					slog.Warn("/api/shares DELETE: sidecar PDS delete failed", "err", err)
					http.Error(w, "upstream PDS error", http.StatusBadGateway)
					return
				}
			}
		} else if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(shareCollection), rkey); err != nil {
			slog.Warn("/api/shares DELETE: rss PDS delete failed", "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}

		if err := writer.DeleteUserShare(r.Context(), db.DeleteUserShareParams{Did: didStr, Rkey: rkey}); err != nil {
			slog.Warn("/api/shares DELETE: Tier-1 delete failed", "err", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// --- GET /api/shares ---

// SharesListHandler returns the user's shares, newest first. Document shares
// carry the joined entry title/slug when the entry is still cached; dead
// entries fall back to itemUrl/document display on the frontend.
func SharesListHandler(reader SharesIndexReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rows, err := reader.ListUserShares(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/shares GET: list failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		out := make([]ShareWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, shareListRowToWire(row))
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

func shareListRowToWire(row db.ListUserSharesRow) ShareWire {
	return ShareWire{
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
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/lexicon"
)

const saveCollection = "blue.morgen.feed.save"

// SaveWire is the on-the-wire save shape; GET /api/saves additionally joins entry title/slug/target when cached. Only POST carries a cid, since user_saves has no cid column.
type SaveWire struct {
	URI       string `json:"uri,omitempty"`
	CID       string `json:"cid,omitempty"`
	Rkey      string `json:"rkey"`
	ItemURL   string `json:"itemUrl"`
	FeedURL   string `json:"feedUrl,omitempty"`
	CreatedAt string `json:"createdAt"`
	Title     string `json:"title,omitempty"`
	TargetURL string `json:"targetUrl,omitempty"`
	EntrySlug string `json:"entrySlug,omitempty"`
}

// SavesIndexReader is the slice of db.Queries the saves handlers use for reads.
type SavesIndexReader interface {
	GetUserSave(ctx context.Context, arg db.GetUserSaveParams) (db.UserSave, error)
	GetUserSaveByItemURL(ctx context.Context, arg db.GetUserSaveByItemURLParams) (db.UserSave, error)
	ListUserSaves(ctx context.Context, did string) ([]db.ListUserSavesRow, error)
}

// SavesIndexWriter is the slice used for writes.
type SavesIndexWriter interface {
	UpsertUserSave(ctx context.Context, arg db.UpsertUserSaveParams) error
	DeleteUserSave(ctx context.Context, arg db.DeleteUserSaveParams) error
}

// --- POST /api/saves ---

type savesCreateRequest struct {
	ItemURL string `json:"itemUrl"`
	FeedURL string `json:"feedUrl"`
}

// SavesCreateHandler writes a save record to the PDS and mirrors it into the Tier-1 cache; idempotent on (did, itemUrl).
func SavesCreateHandler(reader SavesIndexReader, writer SavesIndexWriter, pds atprepo.Writer, disp RepairDispatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body savesCreateRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.ItemURL = strings.TrimSpace(body.ItemURL)
		body.FeedURL = strings.TrimSpace(body.FeedURL)
		if body.ItemURL == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "itemUrl is required")
			return
		}
		didStr := sess.Data.AccountDID.String()

		if existing, err := reader.GetUserSaveByItemURL(r.Context(), db.GetUserSaveByItemURLParams{
			Did:     didStr,
			ItemUrl: body.ItemURL,
		}); err == nil {
			writeJSON(w, saveRowToWire(existing))
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("/api/saves: dedupe probe failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		record := map[string]any{
			"itemUrl":   body.ItemURL,
			"createdAt": now,
		}
		if body.FeedURL != "" {
			record["feedUrl"] = body.FeedURL
		}
		if err := lexicon.ValidateRecord(saveCollection, record); err != nil {
			slog.Warn("/api/saves: record failed lexicon validation", "err", err)
			writeError(w, http.StatusInternalServerError, codeInvalidRecord, "internal error")
			return
		}
		ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(saveCollection), record)
		if err != nil {
			slog.Warn("/api/saves: PDS create failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		rkey := atprepo.RkeyFromATURI(ref.URI)

		mirrorOrRepair(r.Context(), disp, sess, "/api/saves: Tier-1 upsert", func() error {
			return writer.UpsertUserSave(r.Context(), db.UpsertUserSaveParams{
				Did:       didStr,
				Rkey:      rkey,
				AtUri:     ref.URI,
				ItemUrl:   body.ItemURL,
				FeedUrl:   nilIfEmpty(body.FeedURL),
				CreatedAt: now,
				UpdatedAt: now,
			})
		})

		writeJSONStatus(w, http.StatusCreated, SaveWire{
			URI:       ref.URI,
			CID:       ref.CID,
			Rkey:      rkey,
			ItemURL:   body.ItemURL,
			FeedURL:   body.FeedURL,
			CreatedAt: now,
		})
	})
}

// --- DELETE /api/saves/{rkey} ---

// SavesDeleteHandler tombstones the PDS record and removes the Tier-1 row.
func SavesDeleteHandler(reader SavesIndexReader, writer SavesIndexWriter, pds atprepo.Writer, disp RepairDispatcher) http.Handler {
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
		_, err := reader.GetUserSave(r.Context(), db.GetUserSaveParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Collapse "not yours" and "doesn't exist" to 404 to avoid leaking existence.
				writeError(w, http.StatusNotFound, codeNotFound, "not found")
				return
			}
			slog.Warn("/api/saves DELETE: load failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(saveCollection), rkey); err != nil {
			slog.Warn("/api/saves DELETE: PDS delete failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "upstream PDS error")
			return
		}
		mirrorOrRepair(r.Context(), disp, sess, "/api/saves DELETE: Tier-1 delete", func() error {
			return writer.DeleteUserSave(r.Context(), db.DeleteUserSaveParams{Did: didStr, Rkey: rkey})
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

// --- GET /api/saves ---

// SavesListHandler returns the user's saves, newest first, joining entry title/slug/target when the entry is still cached.
func SavesListHandler(reader SavesIndexReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		rows, err := reader.ListUserSaves(r.Context(), sess.Data.AccountDID.String())
		if err != nil {
			slog.Warn("/api/saves GET: list failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		out := make([]SaveWire, 0, len(rows))
		for _, row := range rows {
			out = append(out, saveListRowToWire(row))
		}
		writeJSON(w, out)
	})
}

func saveListRowToWire(row db.ListUserSavesRow) SaveWire {
	// The save's own feedUrl is optional (a save can predate knowing the feed), so the joined entry's feed stands in.
	feedURL := derefStr(row.FeedUrl)
	if feedURL == "" {
		feedURL = derefStr(row.EntryFeedUrl)
	}
	return SaveWire{
		URI:       row.AtUri,
		Rkey:      row.Rkey,
		ItemURL:   row.ItemUrl,
		FeedURL:   feedURL,
		CreatedAt: row.CreatedAt,
		Title:     derefStr(row.EntryTitle),
		TargetURL: derefStr(row.EntryUrl),
		EntrySlug: derefStr(row.EntrySlug),
	}
}

func saveRowToWire(row db.UserSave) SaveWire {
	feedURL := ""
	if row.FeedUrl != nil {
		feedURL = *row.FeedUrl
	}
	return SaveWire{
		URI:       row.AtUri,
		Rkey:      row.Rkey,
		ItemURL:   row.ItemUrl,
		FeedURL:   feedURL,
		CreatedAt: row.CreatedAt,
	}
}

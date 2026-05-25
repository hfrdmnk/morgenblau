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

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
)

const saveCollection = "blue.morgen.feed.save"

// SaveWire is the on-the-wire shape returned by POST. Frontend only needs
// rkey to drive the saved-state UI; uri/cid included so downstream consumers
// (e.g. a future library page) can hydrate without a second lookup.
type SaveWire struct {
	URI       string `json:"uri"`
	CID       string `json:"cid,omitempty"`
	Rkey      string `json:"rkey"`
	ItemURL   string `json:"itemUrl"`
	FeedURL   string `json:"feedUrl,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// SavesIndexReader is the slice of db.Queries the create/delete handlers
// use for reads. Defined as an interface so handler tests can stub the DB.
type SavesIndexReader interface {
	GetUserSave(ctx context.Context, arg db.GetUserSaveParams) (db.UserSave, error)
	GetUserSaveByItemURL(ctx context.Context, arg db.GetUserSaveByItemURLParams) (db.UserSave, error)
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

// SavesCreateHandler writes a blue.morgen.feed.save record to the user's PDS
// and mirrors it into the Tier-1 cache. Idempotent on (did, itemUrl): a second
// call returns the existing record without a PDS hit.
func SavesCreateHandler(reader SavesIndexReader, writer SavesIndexWriter, pds atprepo.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var body savesCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		body.ItemURL = strings.TrimSpace(body.ItemURL)
		body.FeedURL = strings.TrimSpace(body.FeedURL)
		if body.ItemURL == "" {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"message": "itemUrl is required"})
			return
		}
		didStr := sess.Data.AccountDID.String()

		// Step 1: dedupe. If a Tier-1 row already maps this DID to this
		// itemUrl, return the existing record idempotently.
		if existing, err := reader.GetUserSaveByItemURL(r.Context(), db.GetUserSaveByItemURLParams{
			Did:     didStr,
			ItemUrl: body.ItemURL,
		}); err == nil {
			writeJSON(w, saveRowToWire(existing))
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("/api/saves: dedupe probe failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Step 2: PDS write.
		now := time.Now().UTC().Format(time.RFC3339)
		record := map[string]any{
			"itemUrl":   body.ItemURL,
			"createdAt": now,
		}
		if body.FeedURL != "" {
			record["feedUrl"] = body.FeedURL
		}
		// TODO(blue.morgen lexicon): validate against the published
		// blue.morgen.feed.save schema once it's resolvable on the network.
		ref, err := pds.CreateRecord(r.Context(), sess, syntax.NSID(saveCollection), record)
		if err != nil {
			slog.Warn("/api/saves: PDS create failed", "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		rkey := atprepo.RkeyFromATURI(ref.URI)

		// Step 3: Tier-1 cache upsert.
		if err := writer.UpsertUserSave(r.Context(), db.UpsertUserSaveParams{
			Did:       didStr,
			Rkey:      rkey,
			AtUri:     ref.URI,
			ItemUrl:   body.ItemURL,
			FeedUrl:   nilIfEmpty(body.FeedURL),
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			// PDS write already succeeded — log and continue. A later sync_user
			// will reconcile the local cache.
			slog.Warn("/api/saves: Tier-1 upsert failed (PDS write succeeded)", "err", err)
		}

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
func SavesDeleteHandler(reader SavesIndexReader, writer SavesIndexWriter, pds atprepo.Writer) http.Handler {
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
		_, err := reader.GetUserSave(r.Context(), db.GetUserSaveParams{Did: didStr, Rkey: rkey})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Collapse "not yours" and "doesn't exist" to 403 to avoid
				// leaking existence — same pattern as subscriptions DELETE.
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			slog.Warn("/api/saves DELETE: load failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := pds.DeleteRecord(r.Context(), sess, syntax.NSID(saveCollection), rkey); err != nil {
			slog.Warn("/api/saves DELETE: PDS delete failed", "err", err)
			http.Error(w, "upstream PDS error", http.StatusBadGateway)
			return
		}
		if err := writer.DeleteUserSave(r.Context(), db.DeleteUserSaveParams{Did: didStr, Rkey: rkey}); err != nil {
			slog.Warn("/api/saves DELETE: Tier-1 delete failed", "err", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
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

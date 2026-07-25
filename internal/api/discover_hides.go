package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverhide"
	"morgenblau/internal/feedkey"
)

const (
	maxDiscoverHideTargetKeyBytes = 2048
	maxDiscoverHidesPerUser       = 1000
)

// DiscoverHideWire is the wire shape for a hide mutation result.
type DiscoverHideWire struct {
	TargetKind  string `json:"targetKind"`
	TargetKey   string `json:"targetKey"`
	HiddenUntil string `json:"hiddenUntil"`
	HideCount   int64  `json:"hideCount"`
}

// DiscoverHidesReader reads the existing escalation count.
type DiscoverHidesReader interface {
	GetDiscoverHide(ctx context.Context, arg db.GetDiscoverHideParams) (db.DiscoverHide, error)
	CountDiscoverHidesForUser(ctx context.Context, did string) (int64, error)
}

// DiscoverHidesWriter persists the hide row; deliberately no PDS writer dependency anywhere in this handler, hide state is never a PDS record. SPEC <discovery> Hiding and rotation.
type DiscoverHidesWriter interface {
	UpsertDiscoverHide(ctx context.Context, arg db.UpsertDiscoverHideParams) error
}

type discoverHidesCreateRequest struct {
	TargetKind string `json:"targetKind"`
	TargetKey  string `json:"targetKey"`
}

// DiscoverHidesCreateHandler snoozes a source key or person DID for the session's user: 30 days on first hide, 180 on repeat. Ownership is enforced by threading the session did through every query, never by comparing a fetched row's did.
func DiscoverHidesCreateHandler(reader DiscoverHidesReader, writer DiscoverHidesWriter, memo DiscoverInvalidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body discoverHidesCreateRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		kind := discoverhide.TargetKind(body.TargetKind)
		if kind != discoverhide.TargetSource && kind != discoverhide.TargetPerson {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, `targetKind must be "source" or "person"`)
			return
		}
		if !validDiscoverHideTarget(kind, body.TargetKey) {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid targetKey")
			return
		}
		didStr := sess.Data.AccountDID.String()

		var existingCount int64
		isNewTarget := false
		existing, err := reader.GetDiscoverHide(r.Context(), db.GetDiscoverHideParams{
			Did:        didStr,
			TargetKind: string(kind),
			TargetKey:  body.TargetKey,
		})
		switch {
		case err == nil:
			existingCount = existing.HideCount
		case errors.Is(err, sql.ErrNoRows):
			isNewTarget = true
		default:
			slog.Warn("/api/discover/hides: lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if isNewTarget {
			count, err := reader.CountDiscoverHidesForUser(r.Context(), didStr)
			if err != nil {
				slog.Warn("/api/discover/hides: count failed", "err", err)
				writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
				return
			}
			if count >= maxDiscoverHidesPerUser {
				writeError(w, http.StatusUnprocessableEntity, codeInvalidRequest, "too many hidden items")
				return
			}
		}

		now := time.Now().UTC()
		hiddenUntil, hideCount := discoverhide.NextSnooze(existingCount, now)
		nowStr := now.Format(time.RFC3339)
		untilStr := hiddenUntil.Format(time.RFC3339)

		if err := writer.UpsertDiscoverHide(r.Context(), db.UpsertDiscoverHideParams{
			Did:         didStr,
			TargetKind:  string(kind),
			TargetKey:   body.TargetKey,
			HiddenUntil: untilStr,
			HideCount:   hideCount,
			CreatedAt:   nowStr,
			UpdatedAt:   nowStr,
		}); err != nil {
			slog.Warn("/api/discover/hides: upsert failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		invalidateDiscover(memo, didStr)

		writeJSONStatus(w, http.StatusCreated, DiscoverHideWire{
			TargetKind:  string(kind),
			TargetKey:   body.TargetKey,
			HiddenUntil: untilStr,
			HideCount:   hideCount,
		})
	})
}

func validDiscoverHideTarget(kind discoverhide.TargetKind, key string) bool {
	if key == "" || len(key) > maxDiscoverHideTargetKeyBytes || key != strings.TrimSpace(key) {
		return false
	}
	if kind == discoverhide.TargetPerson {
		_, err := syntax.ParseDID(key)
		return err == nil
	}
	if strings.HasPrefix(key, "at://") {
		uri, err := syntax.ParseATURI(key)
		return err == nil &&
			uri.Authority().IsDID() &&
			uri.Collection() != "" &&
			uri.RecordKey() != "" &&
			uri.Normalize().String() == key
	}
	parsed, err := url.Parse(key)
	return err == nil &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" &&
		parsed.User == nil &&
		parsed.Fragment == "" &&
		feedkey.Normalize(key) == key
}

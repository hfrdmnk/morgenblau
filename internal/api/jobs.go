package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/jobs"
	"morgenblau/internal/middleware/auth"
)

// JobSource is the slice of *jobs.Tracker the handlers depend on.
type JobSource interface {
	Get(id string, did syntax.DID) (*jobs.Job, error)
	ActiveForUser(did syntax.DID) *jobs.Job
}

// JobsGetHandler returns lifecycle status for the given job id, scoped to
// the requesting user. 404 for unknown, 403 across users.
func JobsGetHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		id := r.PathValue("id")
		j, err := src.Get(id, sess.Data.AccountDID)
		if errors.Is(err, jobs.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, jobs.ErrForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if err != nil {
			slog.Warn("/api/jobs/{id}: get failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, j)
	})
}

// JobsActiveHandler returns the user's most recent in-flight job, or null.
// Polled by the digest skeleton on /consume — keep the body tiny.
func JobsActiveHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		j := src.ActiveForUser(sess.Data.AccountDID)
		w.Header().Set("Content-Type", "application/json")
		if j == nil {
			_, _ = w.Write([]byte("null"))
			return
		}
		_ = json.NewEncoder(w).Encode(j)
	})
}

// SyncStarter is the slice of internal/sync the digest refresh endpoint
// depends on. The dispatch implementation lives in internal/sync; tests inject
// a fake.
type SyncStarter interface {
	StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

// DigestRefreshHandler creates a sync_user job with trigger="manual" and
// returns {jobId} immediately. The actual fan-out lives behind SyncStarter.
func DigestRefreshHandler(starter SyncStarter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		id, err := starter.StartManualRefresh(r.Context(), sess.Data.AccountDID, sess.Data.SessionID)
		if err != nil {
			slog.Warn("/api/digest/refresh: start failed", "err", err)
			http.Error(w, "could not start refresh", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"jobId": id})
	})
}

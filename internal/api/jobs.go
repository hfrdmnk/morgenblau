package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/jobs"
)

// JobSource is the slice of *jobs.Tracker the handlers depend on.
type JobSource interface {
	Get(id string, did syntax.DID) (*jobs.Job, error)
	ActiveForUser(did syntax.DID) *jobs.Job
}

// JobsGetHandler returns lifecycle status for the given job id; 404 for both unknown and cross-user so a probe can't tell them apart.
func JobsGetHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		j, err := src.Get(id, sess.Data.AccountDID)
		if errors.Is(err, jobs.ErrNotFound) {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return
		}
		if errors.Is(err, jobs.ErrForbidden) {
			writeError(w, http.StatusNotFound, codeNotFound, "not found")
			return
		}
		if err != nil {
			slog.Warn("/api/jobs/{id}: get failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		writeJSON(w, j)
	})
}

// JobsActiveHandler returns the user's most recent in-flight job, or null; polled by the digest skeleton, keep the body tiny.
func JobsActiveHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		writeJSON(w, src.ActiveForUser(sess.Data.AccountDID))
	})
}

// SyncStarter is the slice of internal/sync the digest refresh endpoint depends on.
type SyncStarter interface {
	StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

// DigestRefreshHandler creates a sync_user job with trigger="manual" and returns {jobId} immediately; fan-out lives behind SyncStarter.
func DigestRefreshHandler(starter SyncStarter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		id, err := starter.StartManualRefresh(r.Context(), sess.Data.AccountDID, sess.Data.SessionID)
		if err != nil {
			slog.Warn("/api/digest/refresh: start failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "could not start refresh")
			return
		}
		writeJSON(w, map[string]string{"jobId": id})
	})
}

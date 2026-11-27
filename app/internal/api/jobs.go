package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/jobs"
)

type JobSource interface {
	Get(id string, did syntax.DID) (*jobs.Job, error)
	ActiveForUser(did syntax.DID) *jobs.Job
	LatestSyncForUser(did syntax.DID) *jobs.Job
}

func JobsGetHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		j, err := src.Get(id, sess.Data.AccountDID)
		if errors.Is(err, jobs.ErrNotFound) || errors.Is(err, jobs.ErrForbidden) {
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

func JobsActiveHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		writeJSON(w, src.ActiveForUser(sess.Data.AccountDID))
	})
}

func JobsLatestHandler(src JobSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		writeJSON(w, src.LatestSyncForUser(sess.Data.AccountDID))
	})
}

type SyncStarter interface {
	StartManualRefresh(ctx context.Context, did syntax.DID, sessionID string) (string, error)
}

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

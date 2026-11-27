package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"morgenblau/internal/session"

	"morgenblau/internal/middleware/auth"
)

const (
	codeNotFound        = "not_found"
	codeInvalidRequest  = "invalid_request"
	codeReauthRequired  = "reauth_required"
	codeUpstreamError   = "upstream_error"
	codeInternalError   = "internal_error"
	codePayloadTooLarge = "payload_too_large"
	codeConflict        = "conflict"
	codeUnprocessable   = "unprocessable"
	codeInvalidRecord   = "invalid_record"
	codeUnavailable     = "unavailable"
	codeRateLimited     = "rate_limited"
	codeSetupInFlight   = "setup_in_flight"
	codeBrowserBuilt    = "browser_built"
	codeYouTubeDown     = "youtube_down"
)

type errorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSONStatus(w, status, errorEnvelope{Code: code, Message: message})
}

func NotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, codeNotFound, "not found")
}

func writeFieldErrors(w http.ResponseWriter, fieldErrors map[string]string) {
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{"errors": fieldErrors})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "invalid json")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requireSession(w http.ResponseWriter, r *http.Request) (*session.Session, bool) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil || sess.Data == nil {
		slog.Error("requireSession: no session in context (middleware bypassed?)", "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return nil, false
	}
	return sess, true
}

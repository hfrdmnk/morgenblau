package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"

	"morgenblau/internal/middleware/auth"
)

// Stable machine-readable error codes on every JSON error body; the frontend keys reauth classification and form errors off these slugs.
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
)

// errorEnvelope is the single error body shape: a stable machine `code` plus a human `message`.
type errorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError writes {"code","message"} at the given status.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSONStatus(w, status, errorEnvelope{Code: code, Message: message})
}

// writeFieldErrors emits the 400 {"errors": {...}} shape the add-dialog form binds to.
func writeFieldErrors(w http.ResponseWriter, fieldErrors map[string]string) {
	writeJSONStatus(w, http.StatusBadRequest, map[string]any{"errors": fieldErrors})
}

// decodeJSON decodes the body into v; returns false (after writing the response) on malformed JSON (400) or a body over the auth middleware's MaxBytesReader limit (413).
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

// writeJSON writes v as a 200 JSON body.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONStatus writes v as a JSON body at the given status.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// requireSession fetches the session the auth middleware injects; a nil session means the middleware was bypassed (a wiring bug, not a client error).
func requireSession(w http.ResponseWriter, r *http.Request) (*oauth.ClientSession, bool) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil || sess.Data == nil {
		slog.Error("requireSession: no session in context (middleware bypassed?)", "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return nil, false
	}
	return sess, true
}

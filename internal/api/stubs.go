package api

import (
	"encoding/json"
	"net/http"
)

// NotImplementedHandler returns 501 with a stable JSON body so frontend
// callers fail loudly during development. Used for v1-deferred endpoints
// (saved, shares, follows, social).
func NotImplementedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not implemented in v1"})
	})
}

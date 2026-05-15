package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotImplementedHandler(t *testing.T) {
	h := NotImplementedHandler()
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/saved", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotImplemented {
			t.Errorf("%s: status = %d, want 501", method, rr.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		if body["error"] != "not implemented in v1" {
			t.Errorf("%s: error = %q", method, body["error"])
		}
	}
}

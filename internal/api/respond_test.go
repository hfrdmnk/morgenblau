package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON_Valid(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(`{"url":"http://example.com"}`))
	var body struct {
		URL string `json:"url"`
	}
	if !decodeJSON(rr, r, &body) {
		t.Fatalf("decodeJSON returned false, body=%q", rr.Body.String())
	}
	if body.URL != "http://example.com" {
		t.Errorf("URL = %q", body.URL)
	}
}

func TestDecodeJSON_Malformed400(t *testing.T) {
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(`{not json`))
	var body map[string]any
	if decodeJSON(rr, r, &body) {
		t.Fatal("expected decodeJSON to fail on malformed JSON")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not an error envelope: %v", err)
	}
	if env.Code != codeInvalidRequest {
		t.Errorf("code = %q, want %q", env.Code, codeInvalidRequest)
	}
}

func TestDecodeJSON_OversizedBody413(t *testing.T) {
	rr := httptest.NewRecorder()
	oversized := `{"url":"` + strings.Repeat("a", 512) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(oversized))
	// Simulate the MaxBytesReader the auth middleware installs, with a tiny cap.
	r.Body = http.MaxBytesReader(rr, r.Body, 32)

	var body map[string]any
	if decodeJSON(rr, r, &body) {
		t.Fatal("expected decodeJSON to fail on oversized body")
	}
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rr.Code)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not an error envelope: %v", err)
	}
	if env.Code != codePayloadTooLarge {
		t.Errorf("code = %q, want %q", env.Code, codePayloadTooLarge)
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAboutHandler(t *testing.T) {
	rr := httptest.NewRecorder()
	AboutHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/about", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if body := rr.Body.String(); !strings.Contains(body, "<h1>Morgenblau</h1>") || !strings.Contains(body, "bot@morgen.blue") {
		t.Errorf("body missing expected about content: %q", body)
	}
}

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"morgenblau/internal/database"
)

func TestAlphaConfigurationFailsBeforeOpeningDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	t.Setenv("DB_PATH", path)
	t.Setenv("ALPHA_ENABLED", "true")
	t.Setenv("ALPHA_ALLOWED_DIDS", "")
	_, _, err := NewServer()
	if err == nil || !strings.Contains(err.Error(), "ALPHA_ALLOWED_DIDS") {
		t.Fatalf("error = %v, want missing allowlist", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid admission config opened the database")
	}
}

func TestHealthReturnsUnavailableWhenDatabaseIsClosed(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "app.db"))
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	srv := &Server{db: db}
	rr := httptest.NewRecorder()
	srv.healthHandler(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"status":"up"`) {
		t.Fatalf("healthy response: %d %s", rr.Code, rr.Body.String())
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	srv.healthHandler(rr, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), `"status":"down"`) {
		t.Fatalf("unhealthy response: %d %s", rr.Code, rr.Body.String())
	}
}

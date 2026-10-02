package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
	_ "modernc.org/sqlite"

	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/newsletter"
	"morgenblau/internal/session"
)

func TestNewsletterSaveRouteWritesOnlyPrivateStorage(t *testing.T) {
	reader, writer := openNewsletterRouteDB(t)
	service := newsletter.NewService(reader, writer, newsletter.Config{Domain: "news.example"})
	if _, err := service.CreateAddress(context.Background(), "did:plc:alice"); err != nil {
		t.Fatal(err)
	}
	const now = "2026-09-20T10:00:00Z"
	if _, err := writer.Exec(`INSERT INTO newsletter_sources (id, did, source_key, identity_kind, title, created_at, updated_at)
		VALUES ('source-1', 'did:plc:alice', 'manual:source-1', 'manual', 'Example Letters', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`INSERT INTO newsletter_messages (id, did, source_id, entry_slug, dedupe_key, received_at, body_html_blocked, created_at, updated_at)
		VALUES ('message-1', 'did:plc:alice', 'source-1', 'message-1', 'dedupe-1', ?, '<a href="https://letters.example.com/issues/1">Web version</a>', ?, ?)`, now, now, now); err != nil {
		t.Fatal(err)
	}

	did, _ := syntax.ParseDID("did:plc:alice")
	req := httptest.NewRequest(http.MethodPost, "/api/newsletter-saves", strings.NewReader(`{"messageId":"message-1"}`))
	req = req.WithContext(auth.WithSession(req.Context(), &session.Session{Data: &session.Data{AccountDID: did, SessionID: "sid-1"}}))
	rr := httptest.NewRecorder()
	(&Server{newsletters: service}).routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /api/newsletter-saves status = %d, body = %s; want the private save handler", rr.Code, rr.Body.String())
	}
	var publicSaves, privateSaves int
	if err := reader.QueryRow(`SELECT (SELECT COUNT(*) FROM user_saves), (SELECT COUNT(*) FROM newsletter_saves)`).Scan(&publicSaves, &privateSaves); err != nil {
		t.Fatal(err)
	}
	if publicSaves != 0 || privateSaves != 1 {
		t.Fatalf("user_saves = %d, newsletter_saves = %d; want 0 and 1", publicSaves, privateSaves)
	}
}

func openNewsletterRouteDB(t *testing.T) (*sql.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.db")
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("goose CLI is required for newsletter route tests; install github.com/pressly/goose/v3/cmd/goose@v3.27.1")
	}
	if output, err := exec.Command(goosePath, "-dir", filepath.Join("..", "database", "migrations"), "sqlite3", path, "up").CombinedOutput(); err != nil {
		t.Fatalf("goose migration: %v\n%s", err, output)
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)"
	writer, err := sql.Open("sqlite", dsn+"&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	writer.SetMaxOpenConns(1)
	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close(); writer.Close() })
	return reader, writer
}

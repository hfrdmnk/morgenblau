package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/database"
	"morgenblau/internal/newsletter"
)

func TestNewsletterSMTPToAuthenticatedEntryFlow(t *testing.T) {
	dbs := openNewsletterIntegrationDB(t)
	service := newsletter.NewService(dbs.Reader, dbs.Writer, newsletter.Config{Domain: "newsletter.localhost"})
	address, err := service.CreateAddress(context.Background(), "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}

	processorCtx, cancelProcessor := context.WithCancel(context.Background())
	processorDone := make(chan error, 1)
	go func() { processorDone <- service.RunProcessor(processorCtx) }()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	smtpServer := newsletter.NewSMTPServer(service, newsletter.SMTPConfig{
		Addr: listener.Addr().String(), Domain: "newsletter.localhost", MaxMessageBytes: 1 << 20, MaxConnections: 2,
	})
	smtpDone := make(chan error, 1)
	go func() { smtpDone <- smtpServer.Serve(newsletter.LimitSMTPListener(listener, 2)) }()
	t.Cleanup(func() {
		_ = smtpServer.Close()
		<-smtpDone
		cancelProcessor()
		<-processorDone
	})

	message := strings.ReplaceAll(`From: Sample Sender <sample@sender.example>
To: RECIPIENT
Subject: Local integration issue
Message-ID: <local-integration@sender.example>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="morgenblau-boundary"

--morgenblau-boundary
Content-Type: text/html; charset=utf-8

<p>Private issue</p><img src="cid:logo"><img src="https://remote.example.invalid/tracker.png">
--morgenblau-boundary
Content-Type: image/png
Content-Transfer-Encoding: base64
Content-ID: <logo>

iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=
--morgenblau-boundary--
`, "RECIPIENT", address)
	message = strings.ReplaceAll(message, "\n", "\r\n")
	if err := smtp.SendMail(listener.Addr().String(), nil, "sample@sender.example", []string{address}, []byte(message)); err != nil {
		t.Fatal(err)
	}

	var received newsletter.Message
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		groups, listErr := service.ListSources(context.Background(), "did:plc:alice")
		if listErr == nil && len(groups.Active) == 1 {
			messages, messagesErr := service.ListSourceMessages(context.Background(), "did:plc:alice", groups.Active[0].ID)
			if messagesErr == nil && len(messages) == 1 {
				received = messages[0]
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if received.ID == "" {
		t.Fatal("SMTP receipt was not processed into a newsletter message")
	}

	feedReader := &fakeEntryReader{getErr: errors.New("feed reader should not be called")}
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(feedReader, service))
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/entries/"+received.EntrySlug, nil), "did:plc:alice", "sid-1")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("Cache-Control = %q", rr.Header().Get("Cache-Control"))
	}
	var entry EntryWire
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Newsletter == nil || !entry.Newsletter.HasBlockedRemoteImages || entry.Newsletter.RemoteImagesAllowed {
		t.Fatalf("newsletter metadata = %+v", entry.Newsletter)
	}
	if entry.Body == nil || strings.Contains(*entry.Body, "https://remote.example.invalid") || !strings.Contains(*entry.Body, "/api/newsletter-assets/") {
		t.Fatalf("private body = %q", valueOrEmpty(entry.Body))
	}
}

func openNewsletterIntegrationDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "newsletter-integration.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbs.Close() })
	migration, err := os.ReadFile(filepath.Join("..", "database", "migrations", "20260920000000_newsletters.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(migration), "-- +goose Down")[0]
	up = strings.ReplaceAll(up, "-- +goose Up", "")
	up = strings.ReplaceAll(up, "-- +goose StatementBegin", "")
	up = strings.ReplaceAll(up, "-- +goose StatementEnd", "")
	if _, err := dbs.Writer.ExecContext(context.Background(), up); err != nil {
		t.Fatalf("migration: %v", err)
	}
	return dbs
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

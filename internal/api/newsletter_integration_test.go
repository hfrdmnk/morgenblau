package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/newsletter"
)

func TestNewsletterSMTPToAuthenticatedEntryFlow(t *testing.T) {
	dbs := openNewsletterIntegrationDB(t)
	service := newsletter.NewService(dbs.Reader, dbs.Writer, newsletter.Config{Domain: "newsletter.localhost"})
	address, err := service.CreateAddress(context.Background(), "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}

	received := deliverNewsletterOverSMTP(t, service, address, `From: Sample Sender <sample@sender.example>
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
`)

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

func TestNewsletterSaveStaysPrivateWhenMessageHasPublicWebURL(t *testing.T) {
	dbs := openNewsletterIntegrationDB(t)
	service := newsletter.NewService(dbs.Reader, dbs.Writer, newsletter.Config{Domain: "newsletter.localhost"})
	address, err := service.CreateAddress(context.Background(), "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	const webURL = "https://letters.example.com/issues/42"
	received := deliverNewsletterOverSMTP(t, service, address, `From: Example Letters <hello@letters.example.com>
To: RECIPIENT
Subject: Issue 42
Message-ID: <issue-42@letters.example.com>
List-ID: Example Letters <letters.example.com>
List-Archive: <https://letters.example.com/issues>
MIME-Version: 1.0
Content-Type: text/html; charset=utf-8

<p><a href="`+webURL+`">View this issue on the web</a></p><p>Private issue body</p>
`)

	pds := &fakePDS{}
	reader, writer := db.New(dbs.Reader), db.New(dbs.Writer)
	mux := http.NewServeMux()
	mux.Handle("GET /api/entries/{slug}", EntryHandler(reader, service))
	mux.Handle("POST /api/newsletter-saves", NewsletterSaveCreateHandler(service))
	mux.Handle("GET /api/saves", SavesListHandler(reader, service))
	mux.Handle("POST /api/saves", SavesCreateHandler(reader, writer, pds, &recordingRepair{}))
	serve := func(method, path, body string) *httptest.ResponseRecorder {
		req := withSession(httptest.NewRequest(method, path, strings.NewReader(body)), "did:plc:alice", "sid-1")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	rr := serve(http.MethodGet, "/api/entries/"+received.EntrySlug, "")
	var entry map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("entry status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if body, _ := entry["body"].(string); !strings.Contains(body, webURL) {
		t.Fatalf("entry body lost the public web link: %q", body)
	}
	if url, ok := entry["url"]; ok {
		t.Fatalf("newsletter entry exposes url %v, which the public save path would accept", url)
	}

	if rr := serve(http.MethodPost, "/api/newsletter-saves", `{"messageId":"`+received.ID+`"}`); rr.Code != http.StatusCreated {
		t.Fatalf("newsletter save status = %d, body = %s", rr.Code, rr.Body.String())
	}
	rr = serve(http.MethodGet, "/api/saves", "")
	var saves []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &saves); err != nil {
		t.Fatal(err)
	}
	if len(saves) != 1 || saves[0]["kind"] != "newsletter" || saves[0]["itemUrl"] != nil {
		t.Fatalf("saves = %+v, want one private newsletter save without an itemUrl", saves)
	}
	if pds.creates+pds.applyCalls+pds.puts != 0 {
		t.Fatalf("PDS writes = %d creates, %d applyWrites, %d puts; want none", pds.creates, pds.applyCalls, pds.puts)
	}
	var publicSaves, privateSaves int
	if err := dbs.Reader.QueryRow(`SELECT (SELECT COUNT(*) FROM user_saves), (SELECT COUNT(*) FROM newsletter_saves)`).Scan(&publicSaves, &privateSaves); err != nil {
		t.Fatal(err)
	}
	if publicSaves != 0 || privateSaves != 1 {
		t.Fatalf("user_saves = %d, newsletter_saves = %d; want 0 and 1", publicSaves, privateSaves)
	}
}

func deliverNewsletterOverSMTP(t *testing.T, service *newsletter.Service, address, message string) newsletter.Message {
	t.Helper()
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

	message = strings.ReplaceAll(strings.ReplaceAll(message, "RECIPIENT", address), "\n", "\r\n")
	if err := smtp.SendMail(listener.Addr().String(), nil, "sample@sender.example", []string{address}, []byte(message)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		groups, listErr := service.ListSources(context.Background(), "did:plc:alice")
		if listErr == nil && len(groups.Active) == 1 {
			messages, messagesErr := service.ListSourceMessages(context.Background(), "did:plc:alice", groups.Active[0].ID)
			if messagesErr == nil && len(messages) == 1 {
				return messages[0]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("SMTP receipt was not processed into a newsletter message")
	return newsletter.Message{}
}

func openNewsletterIntegrationDB(t *testing.T) *database.DB {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "newsletter-integration.db")
	t.Setenv("DB_PATH", databasePath)
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("goose CLI is required for newsletter integration tests; install github.com/pressly/goose/v3/cmd/goose@v3.27.1")
	}
	migrationDir, err := filepath.Abs(filepath.Join("..", "database", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(goosePath, "-dir", migrationDir, "sqlite3", databasePath, "up").CombinedOutput()
	if err != nil {
		t.Fatalf("goose migration: %v\n%s", err, output)
	}
	dbs, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbs.Close() })
	return dbs
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

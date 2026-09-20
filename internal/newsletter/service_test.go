package newsletter

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"morgenblau/internal/database/db"
)

func TestAddressIsStableAndOwnerScoped(t *testing.T) {
	service, _, _ := newTestService(t)
	ctx := context.Background()

	first, err := service.Address(ctx, "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Address(ctx, "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Address(ctx, "did:plc:bob")
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatalf("address changed: %q then %q", first, again)
	}
	if first == other || !strings.HasSuffix(first, "@news.example") {
		t.Fatalf("addresses = %q, %q", first, other)
	}
}

func TestStopRetainsSavedIssueAndUnsaveDeletesIt(t *testing.T) {
	service, reader, writer := newTestService(t)
	ctx := context.Background()
	sourceID := seedSource(t, writer, "did:plc:alice", "source-a", SourceActive)
	savedMessageID := seedMessage(t, writer, "did:plc:alice", sourceID, "saved", "dedupe-saved")
	seedMessage(t, writer, "did:plc:alice", sourceID, "unsaved", "dedupe-unsaved")
	save, err := service.SaveMessage(ctx, "did:plc:alice", savedMessageID)
	if err != nil {
		t.Fatal(err)
	}

	stopped, err := service.StopSource(ctx, "did:plc:alice", sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != SourceStopped {
		t.Fatalf("status = %q", stopped.Status)
	}
	if stopped.IssueCount != 1 || stopped.SavedCount != 1 {
		t.Fatalf("stopped source counts = %d issues, %d saves", stopped.IssueCount, stopped.SavedCount)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 1 {
		t.Fatalf("message count after stop = %d, want 1", got)
	}
	if err := service.DeleteSave(ctx, "did:plc:alice", save.ID); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 0 {
		t.Fatalf("message count after unsave = %d, want 0", got)
	}
}

func TestOwnerCannotReadOrMutateAnotherOwnersNewsletter(t *testing.T) {
	service, _, writer := newTestService(t)
	ctx := context.Background()
	sourceID := seedSource(t, writer, "did:plc:alice", "source-a", SourceActive)
	messageID := seedMessage(t, writer, "did:plc:alice", sourceID, "private", "dedupe-private")

	if _, err := service.GetSource(ctx, "did:plc:bob", sourceID); err != ErrNotFound {
		t.Fatalf("GetSource error = %v", err)
	}
	if _, err := service.AllowRemoteImages(ctx, "did:plc:bob", messageID); err != ErrNotFound {
		t.Fatalf("AllowRemoteImages error = %v", err)
	}
	if _, err := service.SaveMessage(ctx, "did:plc:bob", messageID); err != ErrNotFound {
		t.Fatalf("SaveMessage error = %v", err)
	}
}

func TestMoveMessageToNewSourceDoesNotChangeDeliveryDedupe(t *testing.T) {
	service, reader, writer := newTestService(t)
	ctx := context.Background()
	sourceID := seedSource(t, writer, "did:plc:alice", "source-a", SourceActive)
	messageID := seedMessage(t, writer, "did:plc:alice", sourceID, "move", "delivery-key")

	destination, err := service.MoveMessage(ctx, "did:plc:alice", messageID, MoveTarget{NewSourceTitle: "Filed manually"})
	if err != nil {
		t.Fatal(err)
	}
	if destination.ID == sourceID || destination.Title != "Filed manually" {
		t.Fatalf("destination = %+v", destination)
	}
	_, err = db.New(writer).CreateNewsletterMessage(ctx, db.CreateNewsletterMessageParams{
		ID: "retry", Did: "did:plc:alice", SourceID: sourceID, EntrySlug: "retry", DedupeKey: "delivery-key",
		SenderAddress: "sender@example.com", ReceivedAt: testNow.Format(time.RFC3339Nano), CreatedAt: testNow.Format(time.RFC3339Nano),
	})
	if err != sql.ErrNoRows {
		t.Fatalf("retry insert error = %v, want sql.ErrNoRows", err)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
}

func TestDigestBoundsIncludeFractionalMidnightAndExcludeNextDay(t *testing.T) {
	service, _, writer := newTestService(t)
	ctx := context.Background()
	sourceID := seedSource(t, writer, "did:plc:alice", "source-a", SourceActive)
	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	seedMessageAt(t, writer, "did:plc:alice", sourceID, "midnight", "digest-midnight", start)
	seedMessageAt(t, writer, "did:plc:alice", sourceID, "fraction", "digest-fraction", start.Add(500*time.Millisecond))
	seedMessageAt(t, writer, "did:plc:alice", sourceID, "next", "digest-next", start.Add(24*time.Hour))

	items, err := service.ListDigestMessages(ctx, "did:plc:alice", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "fraction" || items[1].ID != "midnight" {
		t.Fatalf("digest items = %+v", items)
	}
}

func TestRemoteImagePermissionIsRememberedPerMessage(t *testing.T) {
	service, _, writer := newTestService(t)
	ctx := context.Background()
	sourceID := seedSource(t, writer, "did:plc:alice", "source-a", SourceActive)
	messageID := seedMessage(t, writer, "did:plc:alice", sourceID, "images", "dedupe-images")

	message, err := service.AllowRemoteImages(ctx, "did:plc:alice", messageID)
	if err != nil {
		t.Fatal(err)
	}
	if !message.RemoteImagesAllowed || message.HasBlockedRemoteImages || message.BodyHTML != "<p>remote</p>" {
		t.Fatalf("message after consent = %+v", message)
	}
	again, err := service.GetMessageBySlug(ctx, "did:plc:alice", "images")
	if err != nil || !again.RemoteImagesAllowed || again.BodyHTML != "<p>remote</p>" {
		t.Fatalf("remembered message = %+v, %v", again, err)
	}
}

var testNow = time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

func newTestService(t *testing.T) (*Service, *sql.DB, *sql.DB) {
	return newTestServiceWithConfig(t, Config{Domain: "news.example"})
}

func newTestServiceWithConfig(t *testing.T, cfg Config) (*Service, *sql.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "newsletter.db")
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
	migration, err := os.ReadFile(filepath.Join("..", "database", "migrations", "20260920000000_newsletters.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(migration), "-- +goose Down")[0]
	up = strings.ReplaceAll(up, "-- +goose Up", "")
	up = strings.ReplaceAll(up, "-- +goose StatementBegin", "")
	up = strings.ReplaceAll(up, "-- +goose StatementEnd", "")
	if _, err := writer.Exec(up); err != nil {
		t.Fatalf("migration: %v", err)
	}
	service := NewService(reader, writer, cfg)
	service.now = func() time.Time { return testNow }
	return service, reader, writer
}

func seedSource(t *testing.T, writer *sql.DB, did, id string, status SourceStatus) string {
	t.Helper()
	now := testNow.Format(time.RFC3339Nano)
	_, err := writer.Exec(`INSERT INTO newsletter_sources
		(id, did, source_key, identity_kind, identity_value, title, sender_address, status, created_at, updated_at)
		VALUES (?, ?, ?, 'from', ?, ?, 'sender@example.com', ?, ?, ?)`, id, did, "from:"+id, id+"@example.com", id, status, now, now)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedMessage(t *testing.T, writer *sql.DB, did, sourceID, id, dedupe string) string {
	return seedMessageAt(t, writer, did, sourceID, id, dedupe, testNow)
}

func seedMessageAt(t *testing.T, writer *sql.DB, did, sourceID, id, dedupe string, receivedAt time.Time) string {
	t.Helper()
	now := formatTime(receivedAt)
	_, err := writer.Exec(`INSERT INTO newsletter_messages
		(id, did, source_id, entry_slug, dedupe_key, title, sender_address, received_at,
		 body_html_blocked, body_html_remote, body_text, has_blocked_remote_images, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'sender@example.com', ?, '<p>blocked</p>', '<p>remote</p>', 'body', 1, ?, ?)`, id, did, sourceID, id, dedupe, id, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func countRows(t *testing.T, reader *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := reader.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

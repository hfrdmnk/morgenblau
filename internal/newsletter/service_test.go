package newsletter

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"morgenblau/internal/database/db"
)

func TestAddressIsExplicitlyCreatedStableAndOwnerScoped(t *testing.T) {
	service, _, _ := newTestService(t)
	ctx := context.Background()

	if _, err := service.Address(ctx, "did:plc:alice"); err != ErrNotFound {
		t.Fatalf("Address before creation error = %v, want ErrNotFound", err)
	}

	first, err := service.CreateAddress(ctx, "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Address(ctx, "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateAddress(ctx, "did:plc:bob")
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

func TestCreateAddressConcurrentCallsReturnOneStableAddress(t *testing.T) {
	service, _, _ := newTestService(t)
	const callers = 16
	start := make(chan struct{})
	results := make(chan string, callers)
	errs := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	var done sync.WaitGroup
	for range callers {
		done.Add(1)
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			address, err := service.CreateAddress(context.Background(), "did:plc:alice")
			if err != nil {
				errs <- err
				return
			}
			results <- address
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent CreateAddress error = %v", err)
	}
	var first string
	for address := range results {
		if first == "" {
			first = address
		} else if address != first {
			t.Fatalf("concurrent addresses differ: %q and %q", first, address)
		}
	}
	if first == "" {
		t.Fatal("no address returned")
	}
}

func TestSaveMessageReturnsNotFoundWhenStopWinsAfterReadBeforeWrite(t *testing.T) {
	service, _, writer := newTestService(t)
	sourceID := seedSource(t, writer, "did:plc:alice", "source-a", SourceActive)
	messageID := seedMessage(t, writer, "did:plc:alice", sourceID, "race", "dedupe-race")

	stop, err := writer.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stop.Rollback()
	waitsBefore := writer.Stats().WaitCount
	saveResult := make(chan error, 1)
	go func() {
		_, err := service.SaveMessage(context.Background(), "did:plc:alice", messageID)
		saveResult <- err
	}()

	deadline := time.Now().Add(5 * time.Second)
	for writer.Stats().WaitCount == waitsBefore && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if writer.Stats().WaitCount == waitsBefore {
		t.Fatal("SaveMessage did not reach its write transaction")
	}
	if _, err := stop.Exec(`UPDATE newsletter_sources SET status = 'stopped' WHERE did = ? AND id = ?`, "did:plc:alice", sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := stop.Exec(`DELETE FROM newsletter_messages WHERE did = ? AND source_id = ? AND NOT EXISTS (
		SELECT 1 FROM newsletter_saves WHERE did = newsletter_messages.did AND message_id = newsletter_messages.id
	)`, "did:plc:alice", sourceID); err != nil {
		t.Fatal(err)
	}
	if err := stop.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-saveResult; !errors.Is(err, ErrNotFound) {
		t.Fatalf("SaveMessage error = %v, want ErrNotFound after stop committed", err)
	}
}

func TestGetSourceReadsOnlyRequestedSourceAndIncludesItsStats(t *testing.T) {
	service, reader, writer := newTestService(t)
	ctx := context.Background()
	targetID := seedSource(t, writer, "did:plc:alice", "target", SourceActive)
	otherID := seedSource(t, writer, "did:plc:alice", "other", SourceActive)
	seedMessageAt(t, writer, "did:plc:alice", targetID, "recent", "dedupe-recent", testNow.Add(-time.Hour))
	seedMessageAt(t, writer, "did:plc:alice", targetID, "older", "dedupe-older", testNow.AddDate(0, 0, -35))
	seedMessage(t, writer, "did:plc:alice", otherID, "unrelated", "dedupe-unrelated")
	if _, err := service.SaveMessage(ctx, "did:plc:alice", "recent"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`UPDATE newsletter_sources SET tags = 'not-json' WHERE did = ? AND id = ?`, "did:plc:alice", otherID); err != nil {
		t.Fatal(err)
	}
	source, err := service.GetSource(ctx, "did:plc:alice", targetID)
	if err != nil {
		t.Fatalf("GetSource error = %v", err)
	}
	if source.ID != targetID || source.IssueCount != 2 || source.SavedCount != 1 || source.Count7d != 1 || source.Count28d != 1 || source.Count56d != 2 || source.Count84d != 2 {
		t.Fatalf("GetSource details = %+v", source)
	}
	if got := countRows(t, reader, "newsletter_sources"); got != 2 {
		t.Fatalf("source count = %d, want 2", got)
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

func TestListDigestMessagesRequiresDateBounds(t *testing.T) {
	service, _, _ := newTestService(t)
	if _, err := service.ListDigestMessages(context.Background(), "did:plc:alice", time.Time{}, time.Time{}); err != ErrInvalid {
		t.Fatalf("ListDigestMessages without bounds error = %v, want ErrInvalid", err)
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

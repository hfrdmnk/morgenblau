package newsletter

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProcessReceiptCreatesPrivateSourceAndMessageThenDeletesRaw(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, err := service.Address(ctx, "did:plc:alice")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("From: Weekly <hello@example.com>\r\nSubject: Issue one\r\nMessage-ID: <one@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nHello Alice")
	if err := service.acceptReceipts(ctx, "bounce@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: address}}, raw); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 1 {
		t.Fatalf("receipt count before processing = %d", got)
	}
	processed, err := service.processNext(ctx)
	if err != nil || !processed {
		t.Fatalf("processNext = %v, %v", processed, err)
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 0 {
		t.Fatalf("receipt count after processing = %d", got)
	}
	if got := countRows(t, reader, "newsletter_sources"); got != 1 {
		t.Fatalf("source count = %d", got)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 1 {
		t.Fatalf("message count = %d", got)
	}
}

func TestStoppedSourceDiscardsFutureDeliveryWithoutRetainingRaw(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	first := []byte("From: Weekly <hello@example.com>\r\nSubject: First\r\nMessage-ID: <first@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nFirst")
	service.acceptReceipts(ctx, "bounce@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: address}}, first)
	if _, err := service.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	groups, err := service.ListSources(ctx, "did:plc:alice")
	if err != nil || len(groups.Active) != 1 {
		t.Fatalf("sources = %+v, %v", groups, err)
	}
	if _, err := service.StopSource(ctx, "did:plc:alice", groups.Active[0].ID); err != nil {
		t.Fatal(err)
	}
	second := []byte("From: Weekly <hello@example.com>\r\nSubject: Second\r\nMessage-ID: <second@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nSecond")
	service.acceptReceipts(ctx, "bounce@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: address}}, second)
	if _, err := service.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 0 {
		t.Fatalf("message count = %d, want 0", got)
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 0 {
		t.Fatalf("receipt count = %d, want 0", got)
	}
}

func TestEnableDiscardsBacklogAcceptedWhileSourceWasStopped(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	recipient := []deliveryRecipient{{DID: "did:plc:alice", Address: address}}
	first := []byte("From: Weekly <hello@example.com>\r\nSubject: First\r\nMessage-ID: <first@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nFirst")
	service.acceptReceipts(ctx, "bounce@example.com", recipient, first)
	service.processNext(ctx)
	groups, _ := service.ListSources(ctx, "did:plc:alice")
	sourceID := groups.Active[0].ID
	service.StopSource(ctx, "did:plc:alice", sourceID)

	service.now = func() time.Time { return testNow.Add(time.Minute) }
	backlog := []byte("From: Weekly <hello@example.com>\r\nSubject: Backlog\r\nMessage-ID: <backlog@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nBacklog")
	if err := service.acceptReceipts(ctx, "bounce@example.com", recipient, backlog); err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return testNow.Add(2 * time.Minute) }
	if _, err := service.EnableSource(ctx, "did:plc:alice", sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 0 {
		t.Fatalf("pre-resume backlog count = %d, want 0", got)
	}

	future := []byte("From: Weekly <hello@example.com>\r\nSubject: Future\r\nMessage-ID: <future@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nFuture")
	service.now = func() time.Time { return testNow.Add(3 * time.Minute) }
	service.acceptReceipts(ctx, "bounce@example.com", recipient, future)
	service.processNext(ctx)
	if got := countRows(t, reader, "newsletter_messages"); got != 1 {
		t.Fatalf("post-resume message count = %d, want 1", got)
	}
}

func TestDeliveryRetryAfterManualMoveDoesNotDuplicateMessage(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	raw := []byte("From: Weekly <hello@example.com>\r\nSubject: Issue\r\nMessage-ID: <retry@example.com>\r\nList-ID: Weekly <weekly.example>\r\nContent-Type: text/plain\r\n\r\nSame body")
	service.acceptReceipts(ctx, "bounce@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: address}}, raw)
	service.processNext(ctx)
	groups, _ := service.ListSources(ctx, "did:plc:alice")
	originalSource := groups.Active[0]
	messages, _ := service.ListSourceMessages(ctx, "did:plc:alice", groups.Active[0].ID)
	if _, err := service.MoveMessage(ctx, "did:plc:alice", messages[0].ID, MoveTarget{NewSourceTitle: "Filed"}); err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return testNow.Add(time.Hour) }
	service.acceptReceipts(ctx, "bounce@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: address}}, raw)
	if _, err := service.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, reader, "newsletter_messages"); got != 1 {
		t.Fatalf("message count after retry = %d, want 1", got)
	}
	afterRetry, err := service.GetSource(ctx, "did:plc:alice", originalSource.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRetry.LastReceivedAt == nil || originalSource.LastReceivedAt == nil || !afterRetry.LastReceivedAt.Equal(*originalSource.LastReceivedAt) {
		t.Fatalf("duplicate retry changed last received from %v to %v", originalSource.LastReceivedAt, afterRetry.LastReceivedAt)
	}
}

func TestInlineImageRetryAfterMoveDoesNotDuplicateMessage(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	recipient := []deliveryRecipient{{DID: "did:plc:alice", Address: address}}
	raw := []byte("From: Weekly <hello@example.com>\r\nSubject: Illustrated\r\nMessage-ID: <inline-retry@example.com>\r\nList-ID: Weekly <weekly.example>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=rel\r\n\r\n--rel\r\nContent-Type: text/html\r\n\r\n<p>Issue<img src=\"cid:logo\"></p>\r\n--rel\r\nContent-Type: image/png\r\nContent-ID: <logo>\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw0KGgo=\r\n--rel--\r\n")
	service.acceptReceipts(ctx, "bounce@example.com", recipient, raw)
	service.processNext(ctx)
	groups, _ := service.ListSources(ctx, "did:plc:alice")
	messages, _ := service.ListSourceMessages(ctx, "did:plc:alice", groups.Active[0].ID)
	if _, err := service.MoveMessage(ctx, "did:plc:alice", messages[0].ID, MoveTarget{NewSourceTitle: "Filed"}); err != nil {
		t.Fatal(err)
	}
	service.acceptReceipts(ctx, "bounce@example.com", recipient, raw)
	service.processNext(ctx)
	if got := countRows(t, reader, "newsletter_messages"); got != 1 {
		t.Fatalf("inline retry message count = %d, want 1", got)
	}
}

func TestReceiptQuotaRejectsAtomicallyAndRecoversAfterCleanup(t *testing.T) {
	service, reader, writer := newTestServiceWithConfig(t, Config{
		Domain: "news.example", OwnerStorageBytes: 2 * defaultReceiptReservationBytes,
		GlobalStorageBytes: 2*defaultReceiptReservationBytes - 1,
	})
	ctx := context.Background()
	alice, _ := service.Address(ctx, "did:plc:alice")
	bob, _ := service.Address(ctx, "did:plc:bob")
	raw := []byte("From: sender@example.com\r\nSubject: quota\r\n\r\nbody")
	if err := service.acceptReceipts(ctx, "sender@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: alice}}, raw); err != nil {
		t.Fatal(err)
	}
	err := service.acceptReceipts(ctx, "sender@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: alice}, {DID: "did:plc:bob", Address: bob}}, raw)
	if !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("quota error = %v", err)
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 1 {
		t.Fatalf("partial receipt count = %d, want 1", got)
	}
	if _, err := writer.Exec("DELETE FROM newsletter_receipts"); err != nil {
		t.Fatal(err)
	}
	if err := service.acceptReceipts(ctx, "sender@example.com", []deliveryRecipient{{DID: "did:plc:bob", Address: bob}}, raw); err != nil {
		t.Fatalf("accept after cleanup: %v", err)
	}
}

func TestNormalizedStorageCountsTowardQuotaAndStopFreesIt(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	recipient := []deliveryRecipient{{DID: "did:plc:alice", Address: address}}
	raw := []byte("From: sender@example.com\r\nSubject: stored\r\nContent-Type: text/plain\r\n\r\nstored body")
	service.acceptReceipts(ctx, "sender@example.com", recipient, raw)
	service.processNext(ctx)
	var storedBytes int64
	if err := reader.QueryRow("SELECT storage_bytes FROM newsletter_messages").Scan(&storedBytes); err != nil {
		t.Fatal(err)
	}
	if storedBytes <= int64(len("stored body")) {
		t.Fatalf("storage bytes = %d, want normalized bodies and metadata included", storedBytes)
	}
	service.ownerStorageBytes = storedBytes + defaultReceiptReservationBytes - 1
	if err := service.acceptReceipts(ctx, "sender@example.com", recipient, raw); !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("stored-message quota error = %v", err)
	}
	groups, _ := service.ListSources(ctx, "did:plc:alice")
	if _, err := service.StopSource(ctx, "did:plc:alice", groups.Active[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := service.acceptReceipts(ctx, "sender@example.com", recipient, raw); err != nil {
		t.Fatalf("accept after stored message cleanup: %v", err)
	}
}

func TestMalformedAcceptedMIMEBecomesReadableIssue(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	raw := []byte("From: broken@example.com\r\nSubject: Broken\r\nContent-Type: multipart/mixed; boundary=missing\r\n\r\nReadable fallback")
	service.acceptReceipts(ctx, "bounce@example.com", []deliveryRecipient{{DID: "did:plc:alice", Address: address}}, raw)
	if _, err := service.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := reader.QueryRow("SELECT body_text FROM newsletter_messages").Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Fatal("malformed MIME produced empty issue")
	}
}

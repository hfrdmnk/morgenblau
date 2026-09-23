package newsletter

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

const (
	defaultOwnerStorageBytes       = int64(256 << 20)
	defaultGlobalStorageBytes      = int64(768 << 20)
	defaultReceiptReservationBytes = int64(64 << 20)
)

type deliveryRecipient struct {
	DID     string
	Address string
}

func (s *Service) resolveRecipient(ctx context.Context, address string) (deliveryRecipient, error) {
	address = strings.TrimSpace(address)
	parts := strings.Split(address, "@")
	if len(parts) != 2 || strings.ToLower(parts[1]) != s.domain || parts[0] == "" {
		return deliveryRecipient{}, ErrNotFound
	}
	row, err := s.read.GetNewsletterAddressByLocalPart(ctx, strings.ToLower(parts[0]))
	if err != nil {
		return deliveryRecipient{}, publicDBError(err)
	}
	return deliveryRecipient{DID: row.Did, Address: row.LocalPart + "@" + s.domain}, nil
}

func (s *Service) acceptReceipts(ctx context.Context, envelopeFrom string, recipients []deliveryRecipient, raw []byte) error {
	if len(recipients) == 0 || len(raw) == 0 {
		return ErrInvalid
	}
	now := formatTime(s.now())
	seen := make(map[string]struct{}, len(recipients))
	unique := make([]deliveryRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		if _, ok := seen[recipient.DID]; ok {
			continue
		}
		seen[recipient.DID] = struct{}{}
		unique = append(unique, recipient)
	}
	reservation := int64(len(raw))
	if reservation < defaultReceiptReservationBytes {
		reservation = defaultReceiptReservationBytes
	}
	err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		globalUsed, err := q.GetNewsletterGlobalStorageBytes(ctx)
		if err != nil {
			return err
		}
		globalAdded := reservation * int64(len(unique))
		if exceedsStorageLimit(globalUsed, globalAdded, s.globalStorageBytes) {
			return ErrStorageQuota
		}
		for _, recipient := range unique {
			ownerUsed, err := q.GetNewsletterOwnerStorageBytes(ctx, recipient.DID)
			if err != nil {
				return err
			}
			if exceedsStorageLimit(ownerUsed, reservation, s.ownerStorageBytes) {
				return ErrStorageQuota
			}
		}
		for _, recipient := range unique {
			if err := q.CreateNewsletterReceipt(ctx, db.CreateNewsletterReceiptParams{
				ID: ulid.Make().String(), Did: recipient.DID, EnvelopeFrom: envelopeFrom,
				Recipient: recipient.Address, ReceivedAt: now, RawMime: raw, ReservedBytes: reservation, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

func (s *Service) RunProcessor(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		for {
			processed, err := s.processNext(ctx)
			if err != nil {
				break
			}
			if !processed {
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) processNext(ctx context.Context) (bool, error) {
	now := formatTime(s.now())
	receipt, err := s.read.GetNextNewsletterReceipt(ctx, &now)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parsed := parseMIME(receipt.RawMime, receipt.EnvelopeFrom)
	parsed = fitNormalizedMail(parsed)
	err = database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		source, sourceErr := q.GetNewsletterSourceByKey(ctx, db.GetNewsletterSourceByKeyParams{Did: receipt.Did, SourceKey: parsed.SourceKey})
		if errors.Is(sourceErr, sql.ErrNoRows) {
			identityValue := optionalString(parsed.IdentityValue)
			senderName := parsed.SenderName
			if createErr := q.CreateNewsletterSource(ctx, db.CreateNewsletterSourceParams{
				ID: ulid.Make().String(), Did: receipt.Did, SourceKey: parsed.SourceKey,
				IdentityKind: parsed.IdentityKind, IdentityValue: identityValue, Title: parsed.SourceTitle,
				SenderName: senderName, SenderAddress: parsed.SenderAddress, FirstReceivedAt: &receipt.ReceivedAt,
			}); createErr != nil {
				return createErr
			}
			source, sourceErr = q.GetNewsletterSourceByKey(ctx, db.GetNewsletterSourceByKeyParams{Did: receipt.Did, SourceKey: parsed.SourceKey})
		}
		if sourceErr != nil {
			return sourceErr
		}
		if source.Status == string(SourceStopped) || (source.AcceptAfter != nil && receipt.ReceivedAt < *source.AcceptAfter) {
			return q.DeleteNewsletterReceipt(ctx, receipt.ID)
		}
		messageID := ulid.Make().String()
		dedupeKey := deliveryDedupeKey(parsed)
		insertedID, createErr := q.CreateNewsletterMessage(ctx, db.CreateNewsletterMessageParams{
			ID: messageID, Did: receipt.Did, SourceID: source.ID, EntrySlug: strings.ToLower(messageID),
			DedupeKey: dedupeKey, MessageID: optionalString(parsed.MessageID), Title: optionalString(parsed.Title),
			SenderName: parsed.SenderName, SenderAddress: parsed.SenderAddress, SentAt: formatOptionalTime(parsed.SentAt),
			ReceivedAt: receipt.ReceivedAt, BodyHtmlBlocked: optionalString(parsed.BodyHTMLBlocked),
			BodyHtmlRemote: optionalString(parsed.BodyHTMLRemote), BodyText: optionalString(parsed.BodyText),
			HasBlockedRemoteImages: boolInt(parsed.HasBlockedRemoteImages), AttachmentSummary: optionalString(parsed.AttachmentSummary),
			StorageBytes: normalizedStorageBytes(parsed), CreatedAt: now,
		})
		if errors.Is(createErr, sql.ErrNoRows) {
			return q.DeleteNewsletterReceipt(ctx, receipt.ID)
		}
		if createErr != nil {
			return createErr
		}
		if err := q.TouchNewsletterSource(ctx, db.TouchNewsletterSourceParams{
			SenderName: parsed.SenderName, SenderAddress: parsed.SenderAddress,
			ReceivedAt: &receipt.ReceivedAt, Did: receipt.Did, ID: source.ID,
		}); err != nil {
			return err
		}
		for _, asset := range parsed.Assets {
			if err := q.CreateNewsletterInlineAsset(ctx, db.CreateNewsletterInlineAssetParams{
				Token: asset.Token, MessageID: insertedID, ContentID: asset.ContentID, MediaType: asset.MediaType,
				Data: asset.Data, ContentHash: asset.ContentHash, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return q.DeleteNewsletterReceipt(ctx, receipt.ID)
	})
	if err == nil {
		return true, nil
	}
	slog.Error("newsletter receipt processing failed", "receipt_id", receipt.ID, "err", err)
	next := formatTime(s.now().Add(receiptBackoff(receipt.Attempts + 1)))
	genericFailure := "processing failed"
	retryErr := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		return q.MarkNewsletterReceiptFailed(ctx, db.MarkNewsletterReceiptFailedParams{ID: receipt.ID, LastError: &genericFailure, NextAttemptAt: &next})
	})
	if retryErr != nil {
		slog.Error("newsletter receipt retry state update failed", "receipt_id", receipt.ID, "err", retryErr)
	}
	return false, err
}

func deliveryDedupeKey(message normalizedMail) string {
	canonical := strings.Join([]string{
		strings.TrimSpace(message.MessageID), strings.TrimSpace(message.Title), strings.TrimSpace(message.SenderAddress),
		formatOptionalTimeValue(message.SentAt), message.DedupeContentHash,
	}, "\n")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func exceedsStorageLimit(used, added, limit int64) bool {
	return used < 0 || added < 0 || added > limit || used > limit-added
}

func normalizedStorageBytes(message normalizedMail) int64 {
	total := int64(len(message.SourceKey) + len(message.IdentityKind) + len(message.IdentityValue) + len(message.SourceTitle) +
		len(message.Title) + len(message.SenderAddress) + len(message.MessageID) +
		len(message.BodyHTMLBlocked) + len(message.BodyHTMLRemote) + len(message.BodyText) + len(message.AttachmentSummary))
	if message.SenderName != nil {
		total += int64(len(*message.SenderName))
	}
	for _, asset := range message.Assets {
		total += int64(len(asset.Data) + len(asset.ContentID) + len(asset.MediaType) + len(asset.ContentHash) + len(asset.Token))
	}
	return total
}

func fitNormalizedMail(message normalizedMail) normalizedMail {
	if normalizedStorageBytes(message) <= defaultReceiptReservationBytes {
		return message
	}
	const fallbackTextBytes = 1 << 20
	text := message.BodyText
	if len(text) > fallbackTextBytes {
		text = strings.ToValidUTF8(text[:fallbackTextBytes], "")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "This email was too large to render fully."
	}
	body := plainTextHTML(text)
	message.BodyText = text
	message.BodyHTMLBlocked = body
	message.BodyHTMLRemote = body
	message.HasBlockedRemoteImages = false
	message.Assets = nil
	message.AttachmentSummary = ""
	return message
}

func receiptBackoff(attempt int64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * time.Minute
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}

func formatOptionalTimeValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

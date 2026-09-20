package newsletter

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

type Service struct {
	reader             *sql.DB
	writer             *sql.DB
	read               *db.Queries
	domain             string
	ownerStorageBytes  int64
	globalStorageBytes int64
	wake               chan struct{}
	now                func() time.Time
}

func NewService(reader, writer *sql.DB, cfg Config) *Service {
	if cfg.OwnerStorageBytes <= 0 {
		cfg.OwnerStorageBytes = defaultOwnerStorageBytes
	}
	if cfg.GlobalStorageBytes <= 0 {
		cfg.GlobalStorageBytes = defaultGlobalStorageBytes
	}
	return &Service{
		reader:             reader,
		writer:             writer,
		read:               db.New(reader),
		domain:             strings.ToLower(strings.TrimSpace(cfg.Domain)),
		ownerStorageBytes:  cfg.OwnerStorageBytes,
		globalStorageBytes: cfg.GlobalStorageBytes,
		wake:               make(chan struct{}, 1),
		now:                time.Now,
	}
}

func (s *Service) Address(ctx context.Context, did string) (string, error) {
	did = strings.TrimSpace(did)
	if did == "" {
		return "", ErrInvalid
	}
	if s.domain == "" {
		return "", ErrUnavailable
	}
	row, err := s.read.GetNewsletterAddress(ctx, did)
	if err == nil {
		return row.LocalPart + "@" + s.domain, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	localPart, err := randomLocalPart()
	if err != nil {
		return "", fmt.Errorf("generate newsletter address: %w", err)
	}
	now := formatTime(s.now())
	if err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		return q.CreateNewsletterAddress(ctx, db.CreateNewsletterAddressParams{Did: did, LocalPart: localPart, CreatedAt: now})
	}); err != nil {
		return "", err
	}
	row, err = s.read.GetNewsletterAddress(ctx, did)
	if err != nil {
		return "", err
	}
	return row.LocalPart + "@" + s.domain, nil
}

func (s *Service) ListSources(ctx context.Context, did string) (SourceGroups, error) {
	now := s.now().UTC()
	rows, err := s.read.ListNewsletterSources(ctx, db.ListNewsletterSourcesParams{
		Did:          did,
		ReceivedAt:   formatTime(now.AddDate(0, 0, -7)),
		ReceivedAt_2: formatTime(now),
		ReceivedAt_3: formatTime(now.AddDate(0, 0, -28)),
		ReceivedAt_4: formatTime(now.AddDate(0, 0, -56)),
		ReceivedAt_5: formatTime(now.AddDate(0, 0, -84)),
	})
	if err != nil {
		return SourceGroups{}, err
	}
	result := SourceGroups{Active: []Source{}, Stopped: []Source{}}
	for _, row := range rows {
		source, err := sourceFromListRow(row)
		if err != nil {
			return SourceGroups{}, err
		}
		if source.Status == SourceStopped {
			result.Stopped = append(result.Stopped, source)
		} else {
			result.Active = append(result.Active, source)
		}
	}
	return result, nil
}

func (s *Service) GetSource(ctx context.Context, did, id string) (Source, error) {
	groups, err := s.ListSources(ctx, did)
	if err != nil {
		return Source{}, err
	}
	for _, source := range append(groups.Active, groups.Stopped...) {
		if source.ID == id {
			return source, nil
		}
	}
	return Source{}, ErrNotFound
}

func (s *Service) PatchSource(ctx context.Context, did, id string, patch SourcePatch) (Source, error) {
	patch.Title = strings.TrimSpace(patch.Title)
	if patch.Title == "" {
		return Source{}, ErrInvalid
	}
	if _, err := s.GetSource(ctx, did, id); err != nil {
		return Source{}, err
	}
	tags, err := cleanTags(patch.Tags)
	if err != nil {
		return Source{}, err
	}
	encoded, _ := json.Marshal(tags)
	err = database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		return q.PatchNewsletterSource(ctx, db.PatchNewsletterSourceParams{
			Did: did, ID: id, Title: patch.Title, IsPrimary: boolInt(patch.Primary), Tags: string(encoded), UpdatedAt: formatTime(s.now()),
		})
	})
	if err != nil {
		return Source{}, err
	}
	return s.GetSource(ctx, did, id)
}

func (s *Service) StopSource(ctx context.Context, did, id string) (Source, error) {
	if _, err := s.GetSource(ctx, did, id); err != nil {
		return Source{}, err
	}
	err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		if err := q.SetNewsletterSourceStatus(ctx, db.SetNewsletterSourceStatusParams{Did: did, ID: id, Status: string(SourceStopped), UpdatedAt: formatTime(s.now())}); err != nil {
			return err
		}
		return q.DeleteUnsavedNewsletterMessages(ctx, db.DeleteUnsavedNewsletterMessagesParams{Did: did, SourceID: id})
	})
	if err != nil {
		return Source{}, err
	}
	return s.GetSource(ctx, did, id)
}

func (s *Service) EnableSource(ctx context.Context, did, id string) (Source, error) {
	if _, err := s.GetSource(ctx, did, id); err != nil {
		return Source{}, err
	}
	if err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		acceptAfter := formatTime(s.now())
		return q.EnableNewsletterSource(ctx, db.EnableNewsletterSourceParams{Did: did, ID: id, AcceptAfter: &acceptAfter})
	}); err != nil {
		return Source{}, err
	}
	return s.GetSource(ctx, did, id)
}

func (s *Service) ListSourceMessages(ctx context.Context, did, sourceID string) ([]Message, error) {
	if _, err := s.GetSource(ctx, did, sourceID); err != nil {
		return nil, err
	}
	rows, err := s.read.ListNewsletterMessagesForSource(ctx, db.ListNewsletterMessagesForSourceParams{Did: did, SourceID: sourceID})
	if err != nil {
		return nil, err
	}
	items := make([]Message, 0, len(rows))
	for _, row := range rows {
		message, err := messageFromSourceRow(row)
		if err != nil {
			return nil, err
		}
		items = append(items, message)
	}
	return items, nil
}

func (s *Service) GetMessageBySlug(ctx context.Context, did, slug string) (Message, error) {
	row, err := s.read.GetNewsletterMessageBySlug(ctx, db.GetNewsletterMessageBySlugParams{Did: did, EntrySlug: slug})
	if err != nil {
		return Message{}, publicDBError(err)
	}
	return messageFromSlugRow(row)
}

func (s *Service) AllowRemoteImages(ctx context.Context, did, messageID string) (Message, error) {
	if _, err := s.getMessage(ctx, did, messageID); err != nil {
		return Message{}, err
	}
	if err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		return q.AllowNewsletterRemoteImages(ctx, db.AllowNewsletterRemoteImagesParams{Did: did, ID: messageID, UpdatedAt: formatTime(s.now())})
	}); err != nil {
		return Message{}, err
	}
	return s.getMessage(ctx, did, messageID)
}

func (s *Service) MoveMessage(ctx context.Context, did, messageID string, target MoveTarget) (Source, error) {
	target.SourceID = strings.TrimSpace(target.SourceID)
	target.NewSourceTitle = strings.TrimSpace(target.NewSourceTitle)
	if (target.SourceID == "") == (target.NewSourceTitle == "") {
		return Source{}, ErrInvalid
	}
	if _, err := s.getMessage(ctx, did, messageID); err != nil {
		return Source{}, err
	}
	destinationID := target.SourceID
	if destinationID != "" {
		destination, err := s.GetSource(ctx, did, destinationID)
		if err != nil {
			return Source{}, err
		}
		if destination.Status != SourceActive {
			return Source{}, ErrInvalid
		}
	} else {
		destinationID = ulid.Make().String()
		now := formatTime(s.now())
		if err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
			if err := q.CreateManualNewsletterSource(ctx, db.CreateManualNewsletterSourceParams{
				ID: destinationID, Did: did, SourceKey: "manual:" + destinationID,
				Title: target.NewSourceTitle, CreatedAt: now,
			}); err != nil {
				return err
			}
			return q.MoveNewsletterMessage(ctx, db.MoveNewsletterMessageParams{Did: did, ID: messageID, SourceID: destinationID, UpdatedAt: now})
		}); err != nil {
			return Source{}, err
		}
		return s.GetSource(ctx, did, destinationID)
	}
	if err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		return q.MoveNewsletterMessage(ctx, db.MoveNewsletterMessageParams{Did: did, ID: messageID, SourceID: destinationID, UpdatedAt: formatTime(s.now())})
	}); err != nil {
		return Source{}, err
	}
	return s.GetSource(ctx, did, destinationID)
}

func (s *Service) SaveMessage(ctx context.Context, did, messageID string) (Save, error) {
	if _, err := s.getMessage(ctx, did, messageID); err != nil {
		return Save{}, err
	}
	id := ulid.Make().String()
	now := formatTime(s.now())
	if err := database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		return q.CreateNewsletterSave(ctx, db.CreateNewsletterSaveParams{ID: id, Did: did, MessageID: messageID, CreatedAt: now})
	}); err != nil {
		return Save{}, err
	}
	row, err := s.read.GetNewsletterSaveForMessage(ctx, db.GetNewsletterSaveForMessageParams{Did: did, MessageID: messageID})
	if err != nil {
		return Save{}, err
	}
	created, err := parseTime(row.CreatedAt)
	if err != nil {
		return Save{}, err
	}
	return Save{ID: row.ID, MessageID: row.MessageID, CreatedAt: created}, nil
}

func (s *Service) DeleteSave(ctx context.Context, did, saveID string) error {
	row, err := s.read.GetNewsletterSave(ctx, db.GetNewsletterSaveParams{Did: did, ID: saveID})
	if err != nil {
		return publicDBError(err)
	}
	return database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		if err := q.DeleteNewsletterSave(ctx, db.DeleteNewsletterSaveParams{Did: did, ID: saveID}); err != nil {
			return err
		}
		return q.DeleteStoppedUnsavedNewsletterMessage(ctx, db.DeleteStoppedUnsavedNewsletterMessageParams{Did: did, ID: row.MessageID})
	})
}

func (s *Service) ListSaves(ctx context.Context, did string) ([]SaveItem, error) {
	rows, err := s.read.ListNewsletterSaves(ctx, did)
	if err != nil {
		return nil, err
	}
	items := make([]SaveItem, 0, len(rows))
	for _, row := range rows {
		created, err := parseTime(row.CreatedAt)
		if err != nil {
			return nil, err
		}
		received, err := parseTime(row.ReceivedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, SaveItem{ID: row.ID, MessageID: row.MessageID, CreatedAt: created, Title: deref(row.Title), EntrySlug: row.EntrySlug, ReceivedAt: received, SourceTitle: row.SourceTitle})
	}
	return items, nil
}

func (s *Service) ListDigestMessages(ctx context.Context, did string, start, end time.Time) ([]Message, error) {
	if start.IsZero() != end.IsZero() || (!start.IsZero() && !start.Before(end)) {
		return nil, ErrInvalid
	}
	if start.IsZero() {
		rows, err := s.read.ListAllNewsletterMessagesForDigest(ctx, did)
		if err != nil {
			return nil, err
		}
		items := make([]Message, 0, len(rows))
		for _, row := range rows {
			message, err := messageFromAllDigestRow(row)
			if err != nil {
				return nil, err
			}
			items = append(items, message)
		}
		return items, nil
	}
	rows, err := s.read.ListNewsletterMessagesForDigest(ctx, db.ListNewsletterMessagesForDigestParams{Did: did, ReceivedAt: formatTime(start), ReceivedAt_2: formatTime(end)})
	if err != nil {
		return nil, err
	}
	items := make([]Message, 0, len(rows))
	for _, row := range rows {
		message, err := messageFromDigestRow(row)
		if err != nil {
			return nil, err
		}
		items = append(items, message)
	}
	return items, nil
}

func (s *Service) GetInlineAsset(ctx context.Context, did, token string) (InlineAsset, error) {
	row, err := s.read.GetNewsletterInlineAsset(ctx, db.GetNewsletterInlineAssetParams{Did: did, Token: token})
	if err != nil {
		return InlineAsset{}, publicDBError(err)
	}
	return InlineAsset{MediaType: row.MediaType, Data: row.Data, ETag: row.ContentHash}, nil
}

func randomLocalPart() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)), nil
}

func cleanTags(tags []string) ([]string, error) {
	seen := make(map[string]struct{}, len(tags))
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 80 {
			return nil, ErrInvalid
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, tag)
	}
	return cleaned, nil
}

func publicDBError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

const databaseTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func formatTime(value time.Time) string { return value.UTC().Format(databaseTimeLayout) }

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := parseTime(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

package newsletter

import (
	"context"
	"encoding/json"

	"morgenblau/internal/database/db"
)

func sourceFromRow(row db.NewsletterSource) (Source, error) {
	var tags []string
	if err := json.Unmarshal([]byte(row.Tags), &tags); err != nil {
		return Source{}, err
	}
	first, err := parseOptionalTime(row.FirstReceivedAt)
	if err != nil {
		return Source{}, err
	}
	last, err := parseOptionalTime(row.LastReceivedAt)
	if err != nil {
		return Source{}, err
	}
	return Source{
		ID: row.ID, Title: row.Title, SenderName: row.SenderName, SenderAddress: row.SenderAddress,
		Status: SourceStatus(row.Status), Primary: row.IsPrimary != 0, Tags: tags,
		FirstReceivedAt: first, LastReceivedAt: last,
	}, nil
}

func sourceFromListRow(row db.ListNewsletterSourcesRow) (Source, error) {
	source, err := sourceFromRow(db.NewsletterSource{
		ID: row.ID, Did: row.Did, SourceKey: row.SourceKey, IdentityKind: row.IdentityKind,
		IdentityValue: row.IdentityValue, Title: row.Title, SenderName: row.SenderName,
		SenderAddress: row.SenderAddress, Status: row.Status, IsPrimary: row.IsPrimary,
		Tags: row.Tags, FirstReceivedAt: row.FirstReceivedAt, LastReceivedAt: row.LastReceivedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		return Source{}, err
	}
	source.IssueCount = row.IssueCount
	source.SavedCount = row.SavedCount
	source.Count7d = row.Count7d
	source.Count28d = row.Count28d
	source.Count56d = row.Count56d
	source.Count84d = row.Count84d
	return source, nil
}

type messageFields struct {
	id                     string
	entrySlug              string
	sourceID               string
	sourceTitle            string
	sourcePrimary          int64
	title                  *string
	senderName             *string
	senderAddress          string
	sentAt                 *string
	receivedAt             string
	bodyHTMLBlocked        *string
	bodyHTMLRemote         *string
	bodyText               *string
	hasBlockedRemoteImages int64
	remoteImagesAllowed    int64
	saveID                 *string
	savedAt                *string
}

func messageFromFields(row messageFields) (Message, error) {
	received, err := parseTime(row.receivedAt)
	if err != nil {
		return Message{}, err
	}
	sent, err := parseOptionalTime(row.sentAt)
	if err != nil {
		return Message{}, err
	}
	saved, err := parseOptionalTime(row.savedAt)
	if err != nil {
		return Message{}, err
	}
	body := deref(row.bodyHTMLBlocked)
	if row.remoteImagesAllowed != 0 {
		body = deref(row.bodyHTMLRemote)
	}
	return Message{
		ID: row.id, EntrySlug: row.entrySlug, SourceID: row.sourceID, SourceTitle: row.sourceTitle, SourcePrimary: row.sourcePrimary != 0,
		Title: deref(row.title), SenderName: row.senderName, SenderAddress: row.senderAddress,
		SentAt: sent, ReceivedAt: received, BodyHTML: body, BodyText: deref(row.bodyText),
		HasBlockedRemoteImages: row.hasBlockedRemoteImages != 0 && row.remoteImagesAllowed == 0,
		RemoteImagesAllowed:    row.remoteImagesAllowed != 0, SaveID: row.saveID, SavedAt: saved,
	}, nil
}

func (s *Service) getMessage(ctx context.Context, did, id string) (Message, error) {
	row, err := s.read.GetNewsletterMessage(ctx, db.GetNewsletterMessageParams{Did: did, ID: id})
	if err != nil {
		return Message{}, publicDBError(err)
	}
	return messageFromGetRow(row)
}

func messageFromGetRow(row db.GetNewsletterMessageRow) (Message, error) {
	return messageFromFields(messageFields{id: row.ID, entrySlug: row.EntrySlug, sourceID: row.SourceID, sourceTitle: row.SourceTitle, sourcePrimary: row.SourcePrimary, title: row.Title, senderName: row.SenderName, senderAddress: row.SenderAddress, sentAt: row.SentAt, receivedAt: row.ReceivedAt, bodyHTMLBlocked: row.BodyHtmlBlocked, bodyHTMLRemote: row.BodyHtmlRemote, bodyText: row.BodyText, hasBlockedRemoteImages: row.HasBlockedRemoteImages, remoteImagesAllowed: row.RemoteImagesAllowed, saveID: row.SaveID, savedAt: row.SavedAt})
}

func messageFromSlugRow(row db.GetNewsletterMessageBySlugRow) (Message, error) {
	return messageFromFields(messageFields{id: row.ID, entrySlug: row.EntrySlug, sourceID: row.SourceID, sourceTitle: row.SourceTitle, sourcePrimary: row.SourcePrimary, title: row.Title, senderName: row.SenderName, senderAddress: row.SenderAddress, sentAt: row.SentAt, receivedAt: row.ReceivedAt, bodyHTMLBlocked: row.BodyHtmlBlocked, bodyHTMLRemote: row.BodyHtmlRemote, bodyText: row.BodyText, hasBlockedRemoteImages: row.HasBlockedRemoteImages, remoteImagesAllowed: row.RemoteImagesAllowed, saveID: row.SaveID, savedAt: row.SavedAt})
}

func messageFromSourceRow(row db.ListNewsletterMessagesForSourceRow) (Message, error) {
	return messageFromFields(messageFields{id: row.ID, entrySlug: row.EntrySlug, sourceID: row.SourceID, sourceTitle: row.SourceTitle, sourcePrimary: row.SourcePrimary, title: row.Title, senderName: row.SenderName, senderAddress: row.SenderAddress, sentAt: row.SentAt, receivedAt: row.ReceivedAt, bodyHTMLBlocked: row.BodyHtmlBlocked, bodyHTMLRemote: row.BodyHtmlRemote, bodyText: row.BodyText, hasBlockedRemoteImages: row.HasBlockedRemoteImages, remoteImagesAllowed: row.RemoteImagesAllowed, saveID: row.SaveID, savedAt: row.SavedAt})
}

func messageFromDigestRow(row db.ListNewsletterMessagesForDigestRow) (Message, error) {
	return messageFromFields(messageFields{id: row.ID, entrySlug: row.EntrySlug, sourceID: row.SourceID, sourceTitle: row.SourceTitle, sourcePrimary: row.SourcePrimary, title: row.Title, senderName: row.SenderName, senderAddress: row.SenderAddress, sentAt: row.SentAt, receivedAt: row.ReceivedAt, bodyHTMLBlocked: row.BodyHtmlBlocked, bodyHTMLRemote: row.BodyHtmlRemote, bodyText: row.BodyText, hasBlockedRemoteImages: row.HasBlockedRemoteImages, remoteImagesAllowed: row.RemoteImagesAllowed, saveID: row.SaveID, savedAt: row.SavedAt})
}

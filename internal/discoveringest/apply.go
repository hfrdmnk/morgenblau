package discoveringest

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bluesky-social/jetstream"

	"morgenblau/internal/database/db"
)

const statusDeleted = "deleted"

// MirrorStore is the complete write surface for one atomic Jetstream batch.
type MirrorStore interface {
	UpsertTapRecord(ctx context.Context, arg db.UpsertTapRecordParams) error
	DeleteTapRecord(ctx context.Context, arg db.DeleteTapRecordParams) error
	DeleteTapRecordsForRepo(ctx context.Context, did string) error
	UpsertTapRepoHandle(ctx context.Context, arg db.UpsertTapRepoHandleParams) error
	UpsertTapRepoAccount(ctx context.Context, arg db.UpsertTapRepoAccountParams) error
	MarkTapRepoDirty(ctx context.Context, arg db.MarkTapRepoDirtyParams) error
	TapRepoIsMirrored(ctx context.Context, did string) (bool, error)
	UpsertDiscoverIngestCursor(ctx context.Context, arg db.UpsertDiscoverIngestCursorParams) error
}

func applyEvent(ctx context.Context, store MirrorStore, event jetstream.Event, now func() time.Time) error {
	switch event.Kind {
	case jetstream.KindCommit:
		return applyCommit(ctx, store, event, now)
	case jetstream.KindIdentity:
		return applyIdentity(ctx, store, event, now)
	case jetstream.KindAccount:
		return applyAccount(ctx, store, event, now)
	case jetstream.KindSync:
		return foldSyncMarker(ctx, store, event)
	default:
		return nil
	}
}

func applyCommit(ctx context.Context, store MirrorStore, event jetstream.Event, now func() time.Time) error {
	commit := event.Commit
	if commit == nil || event.DID == "" || commit.Rkey == "" || !tracked(commit.Collection) {
		return nil
	}
	switch commit.Operation {
	case jetstream.OpDelete:
		if err := store.DeleteTapRecord(ctx, db.DeleteTapRecordParams{Did: event.DID, Collection: commit.Collection, Rkey: commit.Rkey}); err != nil {
			return err
		}
	case jetstream.OpCreate, jetstream.OpUpdate:
		if commit.Record != nil {
			record, err := json.Marshal(commit.Record)
			if err != nil {
				return err
			}
			if err := store.UpsertTapRecord(ctx, db.UpsertTapRecordParams{
				Did: event.DID, Collection: commit.Collection, Rkey: commit.Rkey,
				Cid: commit.CID, Record: string(record), IndexedAt: eventStamp(event, now),
			}); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	return store.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: event.DID, MarkedSeq: int64(event.Seq)})
}

func applyIdentity(ctx context.Context, store MirrorStore, event jetstream.Event, now func() time.Time) error {
	if event.Identity == nil || event.DID == "" {
		return nil
	}
	mirrored, err := store.TapRepoIsMirrored(ctx, event.DID)
	if err != nil || !mirrored {
		return err
	}
	if err := store.UpsertTapRepoHandle(ctx, db.UpsertTapRepoHandleParams{
		Did: event.DID, Handle: event.Identity.Handle, UpdatedAt: eventStamp(event, now),
	}); err != nil {
		return err
	}
	return store.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: event.DID, MarkedSeq: int64(event.Seq)})
}

func applyAccount(ctx context.Context, store MirrorStore, event jetstream.Event, now func() time.Time) error {
	if event.Account == nil || event.DID == "" {
		return nil
	}
	mirrored, err := store.TapRepoIsMirrored(ctx, event.DID)
	if err != nil || !mirrored {
		return err
	}
	var active int64
	if event.Account.Active {
		active = 1
	}
	if err := store.UpsertTapRepoAccount(ctx, db.UpsertTapRepoAccountParams{
		Did: event.DID, IsActive: active, Status: event.Account.Status, UpdatedAt: eventStamp(event, now),
	}); err != nil {
		return err
	}
	if !event.Account.Active && event.Account.Status == statusDeleted {
		if err := store.DeleteTapRecordsForRepo(ctx, event.DID); err != nil {
			return err
		}
	}
	return store.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: event.DID, MarkedSeq: int64(event.Seq)})
}

func foldSyncMarker(ctx context.Context, store MirrorStore, event jetstream.Event) error {
	if event.Sync == nil || event.DID == "" {
		return nil
	}
	mirrored, err := store.TapRepoIsMirrored(ctx, event.DID)
	if err != nil || !mirrored {
		return err
	}
	if err := store.DeleteTapRecordsForRepo(ctx, event.DID); err != nil {
		return err
	}
	return store.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: event.DID, MarkedSeq: int64(event.Seq)})
}

func eventStamp(event jetstream.Event, now func() time.Time) string {
	if event.TimeUS > 0 {
		return time.UnixMicro(event.TimeUS).UTC().Format(time.RFC3339Nano)
	}
	return now().UTC().Format(time.RFC3339Nano)
}

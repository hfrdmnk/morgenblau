package sync

import (
	"context"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
)

// reconcileSaves has no Tier-2 join and no fetch to trigger; saves are leaf bookmarks.
func (e *Engine) reconcileSaves(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	// Taken before the listing so any row created while the round-trip is in flight is newer than the snapshot.
	snapshotAt := e.now().UTC()

	// Network first: the writer connection must never be held across a PDS round-trip.
	remote, err := e.lister.ListSaves(ctx, sess)
	if err != nil {
		return err
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	desired := make([]desiredRow, 0, len(remote))
	for _, r := range remote {
		params := db.UpsertUserSaveParams{
			Did:       didStr,
			Rkey:      r.Rkey,
			AtUri:     r.URI,
			ItemUrl:   r.ItemURL,
			FeedUrl:   nilIfEmpty(r.FeedURL),
			CreatedAt: orNow(r.CreatedAt, now),
			UpdatedAt: now,
		}
		desired = append(desired, desiredRow{
			rkey:  r.Rkey,
			write: func(ctx context.Context, q SyncStore) error { return q.UpsertUserSave(ctx, params) },
		})
	}

	return reconcileCollection(ctx, e.runTx, reconcilePass[db.ListUserSavesForSyncRow]{
		collection: "saves",
		snapshotAt: snapshotAt,
		snapshot: func(ctx context.Context, q SyncStore) ([]db.ListUserSavesForSyncRow, error) {
			return q.ListUserSavesForSync(ctx, didStr)
		},
		rkeyOf:      func(row db.ListUserSavesForSyncRow) string { return row.Rkey },
		createdAtOf: func(row db.ListUserSavesForSyncRow) string { return row.CreatedAt },
		desired:     desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserSave(ctx, db.DeleteUserSaveParams{Did: didStr, Rkey: rkey})
		},
	})
}

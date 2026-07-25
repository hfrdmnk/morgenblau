package sync

import (
	"context"
	"log/slog"
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

	remoteByRkey := make(map[string]PDSSave, len(remote))
	for _, r := range remote {
		remoteByRkey[r.Rkey] = r
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// One tx: snapshot read, upserts, deletes; a read failure rolls back, per-statement write errors are tolerated.
	return e.runTx(ctx, func(q SyncStore) error {
		snapshot, err := q.ListUserSavesForSync(ctx, didStr)
		if err != nil {
			return err
		}
		localByRkey := make(map[string]db.ListUserSavesForSyncRow, len(snapshot))
		for _, row := range snapshot {
			localByRkey[row.Rkey] = row
		}

		for rkey, r := range remoteByRkey {
			createdAt := r.CreatedAt
			if createdAt == "" {
				createdAt = now
			}
			if err := q.UpsertUserSave(ctx, db.UpsertUserSaveParams{
				Did:       didStr,
				Rkey:      rkey,
				AtUri:     r.URI,
				ItemUrl:   r.ItemURL,
				FeedUrl:   nilIfEmpty(r.FeedURL),
				CreatedAt: createdAt,
				UpdatedAt: now,
			}); err != nil {
				slog.Warn("reconcile: saves upsert failed", "rkey", rkey, "err", err)
			}
		}

		for rkey, row := range localByRkey {
			if _, stillThere := remoteByRkey[rkey]; stillThere {
				continue
			}
			if createdAfterSnapshot(row.CreatedAt, snapshotAt) {
				slog.Debug("reconcile: saves delete skipped, row newer than the PDS snapshot", "rkey", rkey, "createdAt", row.CreatedAt)
				continue
			}
			if err := q.DeleteUserSave(ctx, db.DeleteUserSaveParams{
				Did:  didStr,
				Rkey: rkey,
			}); err != nil {
				slog.Warn("reconcile: saves delete failed", "rkey", rkey, "err", err)
			}
		}
		return nil
	})
}

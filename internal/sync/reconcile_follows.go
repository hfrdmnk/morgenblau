package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
)

// reconcileFollows mirrors reconcileSaves: follows are leaf edges, no Tier-2 join and no fetch to trigger.
// A subject can have more than one PDS record (two devices following before either synced); canonicalByKey
// collapses those to the smallest rkey so at most one upsert runs, keeping user_follows_did_subject_did_idx satisfied.
func (e *Engine) reconcileFollows(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	// Taken before the listing so any row created while the round-trip is in flight is newer than the snapshot.
	snapshotAt := e.now().UTC()

	remote, err := e.lister.ListFollows(ctx, sess)
	if err != nil {
		return err
	}

	didStr := did.String()
	validRemote := make([]PDSFollow, 0, len(remote))
	for _, r := range remote {
		if r.SubjectDID != didStr {
			validRemote = append(validRemote, r)
			continue
		}
		if e.pds == nil {
			continue
		}
		if err := e.pds.DeleteRecord(ctx, sess, syntax.NSID(followCollection), r.Rkey); err != nil {
			slog.Warn("reconcile: self-follow PDS delete failed", "rkey", r.Rkey, "err", err)
		}
	}

	canonicalBySubject := canonicalByKey(validRemote,
		func(r PDSFollow) string { return r.SubjectDID },
		func(r PDSFollow) string { return r.Rkey })
	desired := make(map[string]PDSFollow, len(canonicalBySubject))
	for _, r := range canonicalBySubject {
		desired[r.Rkey] = r
	}

	now := e.now().UTC().Format(time.RFC3339)

	return e.runTx(ctx, func(q SyncStore) error {
		snapshot, err := q.ListUserFollowsForSync(ctx, didStr)
		if err != nil {
			return err
		}

		// Deletes first: if the canonical rkey for a subject changes, the stale row still holds (did, subject_did) and would collide with the unique index on upsert.
		for _, row := range snapshot {
			if _, keep := desired[row.Rkey]; keep {
				continue
			}
			if createdAfterSnapshot(row.CreatedAt, snapshotAt) {
				slog.Debug("reconcile: follows delete skipped, row newer than the PDS snapshot", "rkey", row.Rkey, "createdAt", row.CreatedAt)
				continue
			}
			if err := q.DeleteUserFollow(ctx, db.DeleteUserFollowParams{
				Did:  didStr,
				Rkey: row.Rkey,
			}); err != nil {
				slog.Warn("reconcile: follows delete failed", "rkey", row.Rkey, "err", err)
			}
		}

		for rkey, r := range desired {
			createdAt := r.CreatedAt
			if createdAt == "" {
				createdAt = now
			}
			if err := q.UpsertUserFollow(ctx, db.UpsertUserFollowParams{
				Did:        didStr,
				Rkey:       rkey,
				AtUri:      r.URI,
				SubjectDid: r.SubjectDID,
				CreatedAt:  createdAt,
				UpdatedAt:  now,
			}); err != nil {
				slog.Warn("reconcile: follows upsert failed", "rkey", rkey, "err", err)
			}
		}
		return nil
	})
}

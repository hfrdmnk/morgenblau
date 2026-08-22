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
	validRemote := e.tombstoneSelfFollows(ctx, sess, remote, didStr)

	canonicalBySubject := canonicalByKey(validRemote,
		func(r PDSFollow) string { return r.SubjectDID },
		func(r PDSFollow) string { return r.Rkey })

	now := e.now().UTC().Format(time.RFC3339)

	desired := make([]desiredRow, 0, len(canonicalBySubject))
	for _, r := range canonicalBySubject {
		params := db.UpsertUserFollowParams{
			Did:        didStr,
			Rkey:       r.Rkey,
			AtUri:      r.URI,
			SubjectDid: r.SubjectDID,
			CreatedAt:  orNow(r.CreatedAt, now),
			UpdatedAt:  now,
		}
		desired = append(desired, desiredRow{
			rkey:  r.Rkey,
			write: func(ctx context.Context, q SyncStore) error { return q.UpsertUserFollow(ctx, params) },
		})
	}

	return reconcileCollection(ctx, e.runTx, reconcilePass[db.ListUserFollowsForSyncRow]{
		collection: "follows",
		snapshotAt: snapshotAt,
		snapshot: func(ctx context.Context, q SyncStore) ([]db.ListUserFollowsForSyncRow, error) {
			return q.ListUserFollowsForSync(ctx, didStr)
		},
		rkeyOf:      func(row db.ListUserFollowsForSyncRow) string { return row.Rkey },
		createdAtOf: func(row db.ListUserFollowsForSyncRow) string { return row.CreatedAt },
		desired:     desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserFollow(ctx, db.DeleteUserFollowParams{Did: didStr, Rkey: rkey})
		},
		// A changed canonical rkey leaves the stale row holding (did, subject_did), which the unique index would trip on the new upsert.
		deleteFirst: true,
	})
}

// tombstoneSelfFollows drops self-follows from the desired set and deletes their PDS records; a nil pds only drops them locally.
func (e *Engine) tombstoneSelfFollows(ctx context.Context, sess *oauth.ClientSession, remote []PDSFollow, didStr string) []PDSFollow {
	valid := make([]PDSFollow, 0, len(remote))
	for _, r := range remote {
		if r.SubjectDID != didStr {
			valid = append(valid, r)
			continue
		}
		if e.pds == nil {
			continue
		}
		if err := e.pds.DeleteRecord(ctx, sess, syntax.NSID(followCollection), r.Rkey); err != nil {
			slog.Warn("reconcile: self-follow PDS delete failed", "rkey", r.Rkey, "err", err)
		}
	}
	return valid
}

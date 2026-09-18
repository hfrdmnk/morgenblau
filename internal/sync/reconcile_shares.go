package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
)

// reconcileShares diffs shares across two record types (SPEC <sync-architecture>). The local rkey
// for a standardfeed row is the recommend's rkey; the sidecar is metadata, never its own row.
func (e *Engine) reconcileShares(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	// Taken before the listings so any row created while the round-trips are in flight is newer than the snapshot.
	snapshotAt := e.now().UTC()

	// Both lists fetched before any mutation and before the tx opens, so the writer connection is never held across a PDS round-trip.
	shares, err := e.lister.ListShares(ctx, sess)
	if err != nil {
		return err
	}
	recommends, err := e.lister.ListRecommends(ctx, sess)
	if err != nil {
		return err
	}

	var (
		rssShares []PDSShare
		sidecars  []PDSShare
	)
	for _, s := range shares {
		if s.Document == "" {
			rssShares = append(rssShares, s)
			continue
		}
		sidecars = append(sidecars, s)
	}
	// Newest sidecar wins so the user's latest comment survives a sync/PATCH race duplicate; losers are deleted below.
	sidecarByDoc := newestSidecarByKey(sidecars,
		func(s PDSShare) string { return s.Document },
		func(s PDSShare) string { return s.Rkey },
		func(kept, dropped PDSShare) {
			slog.Warn("reconcile: duplicate share sidecar for document", "document", kept.Document, "kept", kept.Rkey, "dropped", dropped.Rkey)
		})

	canonicalByDoc := canonicalByKey(recommends,
		func(r PDSRecommend) string { return r.Document },
		func(r PDSRecommend) string { return r.Rkey })

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	desired := make([]desiredRow, 0, len(canonicalByDoc)+len(rssShares))
	for _, rec := range canonicalByDoc {
		doc := rec.Document
		params := db.UpsertUserShareParams{
			Did:       didStr,
			Rkey:      rec.Rkey,
			AtUri:     rec.URI,
			Kind:      "standardfeed",
			Document:  &doc,
			CreatedAt: orNow(rec.CreatedAt, now),
			UpdatedAt: now,
		}
		if sc, ok := sidecarByDoc[doc]; ok {
			rk := sc.Rkey
			params.ItemUrl = nilIfEmpty(sc.ItemURL)
			params.Comment = nilIfEmpty(sc.Comment)
			params.FeedUrl = nilIfEmpty(sc.FeedURL)
			params.SidecarRkey = &rk
		}
		desired = append(desired, desiredRow{
			rkey: rec.Rkey,
			write: func(ctx context.Context, q SyncStore) error {
				p := params
				// No sidecar itemUrl (bare recommend): backfill from the cached entry so the display fallback survives the entry's later deletion, matching what the API stores at share time.
				if p.ItemUrl == nil {
					if url, err := q.GetFeedEntryURLByGuid(ctx, doc); err == nil {
						p.ItemUrl = nilIfEmpty(url)
					}
				}
				return q.UpsertUserShare(ctx, p)
			},
		})
	}
	for _, s := range rssShares {
		itemURL := s.ItemURL
		params := db.UpsertUserShareParams{
			Did:       didStr,
			Rkey:      s.Rkey,
			AtUri:     s.URI,
			Kind:      "rss",
			ItemUrl:   &itemURL,
			Comment:   nilIfEmpty(s.Comment),
			FeedUrl:   nilIfEmpty(s.FeedURL),
			CreatedAt: orNow(s.CreatedAt, now),
			UpdatedAt: now,
		}
		desired = append(desired, desiredRow{
			rkey:  s.Rkey,
			write: func(ctx context.Context, q SyncStore) error { return q.UpsertUserShare(ctx, params) },
		})
	}

	if err := reconcileCollection(ctx, e.runTx, reconcilePass[db.ListUserSharesForSyncRow]{
		collection: "shares",
		snapshotAt: snapshotAt,
		snapshot: func(ctx context.Context, q SyncStore) ([]db.ListUserSharesForSyncRow, error) {
			return q.ListUserSharesForSync(ctx, didStr)
		},
		rkeyOf: func(row db.ListUserSharesForSyncRow) string { return row.Rkey },
		// Keeping an in-flight row can leave it holding (did, document) against a rekeyed canonical; that upsert fails and the next pass converges, which beats dropping a fresh share.
		createdAtOf: func(row db.ListUserSharesForSyncRow) string { return row.CreatedAt },
		desired:     desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserShare(ctx, db.DeleteUserShareParams{Did: didStr, Rkey: rkey})
		},
		// A changed canonical rkey leaves the stale row holding (did, document), which the partial unique index would trip on the new upsert.
		deleteFirst: true,
	}); err != nil {
		slog.Warn("reconcile: shares tx failed", "did", didStr, "err", err)
	}

	// Deletes orphaned and non-canonical sidecars; covered by the blue.morgen grant, no standard-scope check needed. Post-commit since it's a network write.
	sidecarCleanup(ctx, e.pds, sess, syntax.NSID(shareCollection), sidecars,
		func(s PDSShare) string { return s.Document },
		func(s PDSShare) string { return s.Rkey },
		canonicalByDoc, sidecarByDoc,
		func(rkey string, err error) {
			slog.Warn("reconcile: share sidecar cleanup failed", "rkey", rkey, "err", err)
		})
	return nil
}

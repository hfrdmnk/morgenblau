package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/tags"
)

func (e *Engine) reconcileTier1(
	ctx context.Context,
	did syntax.DID,
	sess *oauth.ClientSession,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	onAdded func(feedURL string),
) error {
	// Both lists are fetched before any mutation, so a failed listing can't leave deletes running against a partial snapshot.
	remote, err := e.lister.ListSubscriptions(ctx, sess)
	if err != nil {
		return err
	}
	standard, err := e.lister.ListStandardSubscriptions(ctx, sess)
	if err != nil {
		return err
	}
	e.reconcileRSS(ctx, did, snapshot, remote, onAdded)
	e.reconcileStandardfeed(ctx, did, sess, snapshot, remote, standard, onAdded)
	return nil
}

func (e *Engine) reconcileRSS(
	ctx context.Context,
	did syntax.DID,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	remote []PDSSubscription,
	onAdded func(feedURL string),
) {
	// This pass only touches kind=rss rows; standardfeed rows belong to reconcileStandardfeed and must never be deleted here.
	localByRkey := make(map[string]db.ListUserSubscriptionsForSyncRow, len(snapshot))
	for _, row := range snapshot {
		if row.Kind == "standardfeed" {
			continue
		}
		localByRkey[row.Rkey] = row
	}
	remoteByRkey := make(map[string]PDSSubscription, len(remote))
	for _, r := range remote {
		if r.Kind != "rss" {
			continue
		}
		remoteByRkey[r.Rkey] = r
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// Per-statement errors are logged and tolerated so one bad row doesn't lose its siblings; only a Begin/Commit failure rolls back the whole batch.
	if err := e.runTx(ctx, func(q SyncStore) error {
		for rkey, r := range remoteByRkey {
			_, existed := localByRkey[rkey]
			// Tier-2 upsert first: the FK from feed_entries.feed_url requires it, and onAdded (the Phase-2 fetch trigger) is gated on its success.
			if err := q.UpsertFeed(ctx, db.UpsertFeedParams{
				FeedUrl:   r.FeedURL,
				SiteUrl:   nilIfEmpty(r.SiteURL),
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				slog.Warn("reconcile: Tier-2 upsert failed", "feedUrl", r.FeedURL, "err", err)
				continue
			}
			if !existed {
				onAdded(r.FeedURL)
			}
			if err := q.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
				Did:       didStr,
				Rkey:      rkey,
				AtUri:     r.URI,
				FeedUrl:   r.FeedURL,
				Title:     nilIfEmpty(r.Title),
				IsPrimary: boolToInt64(r.Primary),
				Tags:      tags.Marshal(r.Tags),
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				slog.Warn("reconcile: Tier-1 upsert failed", "err", err)
			}
		}

		for rkey := range localByRkey {
			if _, stillThere := remoteByRkey[rkey]; stillThere {
				continue
			}
			if err := q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{
				Did:  didStr,
				Rkey: rkey,
			}); err != nil {
				slog.Warn("reconcile: Tier-1 delete failed", "err", err)
			}
		}
		return nil
	}); err != nil {
		slog.Warn("reconcile: rss tx failed", "did", didStr, "err", err)
	}
}

// reconcileStandardfeed applies the publication-source model (SPEC <sync-architecture>).
func (e *Engine) reconcileStandardfeed(
	ctx context.Context,
	did syntax.DID,
	sess *oauth.ClientSession,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	morgen []PDSSubscription,
	standard []PDSStandardSubscription,
	onAdded func(feedURL string),
) {
	localStd := make(map[string]db.ListUserSubscriptionsForSyncRow)
	for _, row := range snapshot {
		if row.Kind == "standardfeed" {
			localStd[row.Rkey] = row
		}
	}

	var sidecars []PDSSubscription
	for _, s := range morgen {
		if s.Kind == "standardfeed" {
			sidecars = append(sidecars, s)
		}
	}
	// Newest sidecar wins so the user's latest edit survives a sync/PATCH race duplicate; losers are deleted below.
	sidecarByPub := newestSidecarByKey(sidecars,
		func(s PDSSubscription) string { return s.Publication },
		func(s PDSSubscription) string { return s.Rkey },
		func(kept, dropped PDSSubscription) {
			slog.Warn("reconcile: duplicate sidecar for publication", "publication", kept.Publication, "kept", kept.Rkey, "dropped", dropped.Rkey)
		})

	canonicalByPub := canonicalByKey(standard,
		func(s PDSStandardSubscription) string { return s.Publication },
		func(s PDSStandardSubscription) string { return s.Rkey })
	desired := make(map[string]PDSStandardSubscription, len(canonicalByPub))
	for _, canon := range canonicalByPub {
		desired[canon.Rkey] = canon
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// One tx: deletes then upserts; per-statement errors are tolerated. Sidecar cleanup stays post-commit since it's a network write.
	if err := e.runTx(ctx, func(q SyncStore) error {
		// Deletes first: a canonical-rkey change (duplicate collapse, delete+recreate elsewhere) would trip UNIQUE(did, feed_url) if the new upsert ran before the stale row was gone.
		for rkey := range localStd {
			if _, keep := desired[rkey]; keep {
				continue
			}
			if err := q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{
				Did:  didStr,
				Rkey: rkey,
			}); err != nil {
				slog.Warn("reconcile: standardfeed Tier-1 delete failed", "err", err)
			}
		}

		// Tier-2 upsert precedes Tier-1 for the FK; onAdded gates on Tier-2 success so name/site/icon resolution happens in the dispatched Phase-2 fetch, never here.
		for rkey, canon := range desired {
			if err := q.UpsertFeed(ctx, db.UpsertFeedParams{
				FeedUrl:   canon.Publication,
				Kind:      "standardfeed",
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				slog.Warn("reconcile: standardfeed Tier-2 upsert failed", "publication", canon.Publication, "err", err)
				continue
			}
			if _, existed := localStd[rkey]; !existed {
				onAdded(canon.Publication)
			}
			var (
				title       *string
				primary     int64
				tagsJSON    *string
				sidecarRkey *string
			)
			if sc, ok := sidecarByPub[canon.Publication]; ok {
				title = nilIfEmpty(sc.Title)
				primary = boolToInt64(sc.Primary)
				tagsJSON = tags.Marshal(sc.Tags)
				rk := sc.Rkey
				sidecarRkey = &rk
			}
			if err := q.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
				Did:         didStr,
				Rkey:        rkey,
				AtUri:       canon.URI,
				FeedUrl:     canon.Publication,
				Kind:        "standardfeed",
				SidecarRkey: sidecarRkey,
				Title:       title,
				IsPrimary:   primary,
				Tags:        tagsJSON,
				CreatedAt:   now,
				UpdatedAt:   now,
			}); err != nil {
				slog.Warn("reconcile: standardfeed Tier-1 upsert failed", "err", err)
			}
		}
		return nil
	}); err != nil {
		slog.Warn("reconcile: standardfeed tx failed", "did", didStr, "err", err)
	}

	// Deletes orphaned and non-canonical sidecars; covered by the blue.morgen grant, no standard-scope check needed.
	sidecarCleanup(ctx, e.pds, sess, syntax.NSID(subscriptionCollection), sidecars,
		func(s PDSSubscription) string { return s.Publication },
		func(s PDSSubscription) string { return s.Rkey },
		canonicalByPub, sidecarByPub,
		func(rkey string, err error) {
			slog.Warn("reconcile: sidecar cleanup failed", "rkey", rkey, "err", err)
		})
}

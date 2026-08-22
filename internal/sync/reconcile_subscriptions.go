package sync

import (
	"context"
	"fmt"
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
	local := filterSubscriptions(snapshot, isRSS)
	localByRkey := rkeySet(local)

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	var desired []desiredRow
	for _, r := range remote {
		if r.Kind != "rss" {
			continue
		}
		_, existed := localByRkey[r.Rkey]
		feed := db.UpsertFeedParams{
			FeedUrl:   r.FeedURL,
			SiteUrl:   nilIfEmpty(r.SiteURL),
			CreatedAt: now,
			UpdatedAt: now,
		}
		sub := db.UpsertUserSubscriptionParams{
			Did:       didStr,
			Rkey:      r.Rkey,
			AtUri:     r.URI,
			FeedUrl:   r.FeedURL,
			Title:     nilIfEmpty(r.Title),
			IsPrimary: boolToInt64(r.Primary),
			Tags:      tags.Marshal(r.Tags),
			CreatedAt: now,
			UpdatedAt: now,
		}
		desired = append(desired, desiredRow{
			rkey:  r.Rkey,
			write: tier2ThenTier1(feed, sub, existed, onAdded),
		})
	}

	if err := reconcileCollection(ctx, e.runTx, reconcilePass[db.ListUserSubscriptionsForSyncRow]{
		collection: "subscriptions.rss",
		snapshot: func(context.Context, SyncStore) ([]db.ListUserSubscriptionsForSyncRow, error) {
			return local, nil
		},
		rkeyOf:  func(row db.ListUserSubscriptionsForSyncRow) string { return row.Rkey },
		desired: desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey})
		},
		// A subscription delete+recreated on the PDS keeps its feed URL, so the stale row must vacate before the new rkey upserts or UNIQUE(did, feed_url) rejects it.
		deleteFirst: true,
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
	local := filterSubscriptions(snapshot, isStandardfeed)
	localByRkey := rkeySet(local)

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

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	desired := make([]desiredRow, 0, len(canonicalByPub))
	for _, canon := range canonicalByPub {
		_, existed := localByRkey[canon.Rkey]
		feed := db.UpsertFeedParams{
			FeedUrl:   canon.Publication,
			Kind:      "standardfeed",
			CreatedAt: now,
			UpdatedAt: now,
		}
		sub := db.UpsertUserSubscriptionParams{
			Did:       didStr,
			Rkey:      canon.Rkey,
			AtUri:     canon.URI,
			FeedUrl:   canon.Publication,
			Kind:      "standardfeed",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if sc, ok := sidecarByPub[canon.Publication]; ok {
			rk := sc.Rkey
			sub.Title = nilIfEmpty(sc.Title)
			sub.IsPrimary = boolToInt64(sc.Primary)
			sub.Tags = tags.Marshal(sc.Tags)
			sub.SidecarRkey = &rk
		}
		desired = append(desired, desiredRow{
			rkey:  canon.Rkey,
			write: tier2ThenTier1(feed, sub, existed, onAdded),
		})
	}

	if err := reconcileCollection(ctx, e.runTx, reconcilePass[db.ListUserSubscriptionsForSyncRow]{
		collection: "subscriptions.standardfeed",
		snapshot: func(context.Context, SyncStore) ([]db.ListUserSubscriptionsForSyncRow, error) {
			return local, nil
		},
		rkeyOf:  func(row db.ListUserSubscriptionsForSyncRow) string { return row.Rkey },
		desired: desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey})
		},
		// A canonical-rkey change (duplicate collapse, delete+recreate elsewhere) would trip UNIQUE(did, feed_url) if the new upsert ran before the stale row was gone.
		deleteFirst: true,
	}); err != nil {
		slog.Warn("reconcile: standardfeed tx failed", "did", didStr, "err", err)
	}

	// Deletes orphaned and non-canonical sidecars; covered by the blue.morgen grant, no standard-scope check needed. Post-commit since it's a network write.
	sidecarCleanup(ctx, e.pds, sess, syntax.NSID(subscriptionCollection), sidecars,
		func(s PDSSubscription) string { return s.Publication },
		func(s PDSSubscription) string { return s.Rkey },
		canonicalByPub, sidecarByPub,
		func(rkey string, err error) {
			slog.Warn("reconcile: sidecar cleanup failed", "rkey", rkey, "err", err)
		})
}

// tier2ThenTier1 upserts the catalog row before the subscription: the FK from feed_entries.feed_url requires it,
// and onAdded (the Phase-2 fetch trigger, which resolves name/site/icon) fires only for a source this pass newly added.
func tier2ThenTier1(feed db.UpsertFeedParams, sub db.UpsertUserSubscriptionParams, existed bool, onAdded func(feedURL string)) func(context.Context, SyncStore) error {
	return func(ctx context.Context, q SyncStore) error {
		if err := q.UpsertFeed(ctx, feed); err != nil {
			return fmt.Errorf("tier-2 upsert %s: %w", feed.FeedUrl, err)
		}
		if !existed {
			onAdded(feed.FeedUrl)
		}
		return q.UpsertUserSubscription(ctx, sub)
	}
}

func rkeySet(rows []db.ListUserSubscriptionsForSyncRow) map[string]struct{} {
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		set[row.Rkey] = struct{}{}
	}
	return set
}

// filterSubscriptions splits the caller's Tier-1 snapshot, read once before the PDS listing on purpose: that ordering is these passes' in-flight-write guard, so never re-read it inside the tx.
func filterSubscriptions(snapshot []db.ListUserSubscriptionsForSyncRow, keep func(db.ListUserSubscriptionsForSyncRow) bool) []db.ListUserSubscriptionsForSyncRow {
	out := make([]db.ListUserSubscriptionsForSyncRow, 0, len(snapshot))
	for _, row := range snapshot {
		if keep(row) {
			out = append(out, row)
		}
	}
	return out
}

func isStandardfeed(row db.ListUserSubscriptionsForSyncRow) bool { return row.Kind == "standardfeed" }

// A row whose kind predates the column reads as rss, so the rss pass claims everything the standardfeed pass does not.
func isRSS(row db.ListUserSubscriptionsForSyncRow) bool { return !isStandardfeed(row) }

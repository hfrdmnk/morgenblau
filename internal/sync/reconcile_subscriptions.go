package sync

import (
	"context"
	"errors"
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
	snapshot []db.UserSubscription,
	snapshotAt time.Time,
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
	rssErr := e.reconcileRSS(ctx, did, snapshot, snapshotAt, remote, onAdded)
	standardErr := e.reconcileStandardfeed(ctx, did, sess, snapshot, snapshotAt, remote, standard, onAdded)
	return errors.Join(rssErr, standardErr)
}

func (e *Engine) reconcileRSS(
	ctx context.Context,
	did syntax.DID,
	snapshot []db.UserSubscription,
	snapshotAt time.Time,
	remote []PDSSubscription,
	onAdded func(feedURL string),
) error {
	// This pass only touches kind=rss rows; standardfeed rows belong to reconcileStandardfeed and must never be deleted here.
	local := filterSubscriptions(snapshot, isRSS)
	localByRkey := rkeySet(local)

	var rss []PDSSubscription
	for _, r := range remote {
		if r.Kind == "rss" {
			rss = append(rss, r)
		}
	}
	canonicalByFeed := canonicalByKey(rss,
		func(r PDSSubscription) string { return r.FeedURL },
		func(r PDSSubscription) string { return r.Rkey })

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	desired := make([]desiredRow, 0, len(canonicalByFeed))
	for _, r := range canonicalByFeed {
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

	return reconcileCollection(ctx, e.runTx, reconcilePass[db.UserSubscription]{
		collection:           "subscriptions.rss",
		snapshotAt:           snapshotAt,
		baseline:             local,
		guardLocalChanges:    true,
		updatedAtOf:          func(row db.UserSubscription) string { return row.UpdatedAt },
		changedSinceSnapshot: subscriptionChangedSinceSnapshot,
		snapshot: func(ctx context.Context, q SyncStore) ([]db.UserSubscription, error) {
			rows, err := q.ListUserSubscriptionsForSync(ctx, didStr)
			return filterSubscriptions(rows, isRSS), err
		},
		rkeyOf:  func(row db.UserSubscription) string { return row.Rkey },
		desired: desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey})
		},
		// A subscription delete+recreated on the PDS keeps its feed URL, so the stale row must vacate before the new rkey upserts or UNIQUE(did, feed_url) rejects it.
		deleteFirst: true,
	})
}

// reconcileStandardfeed applies the publication-source model (SPEC <sync-architecture>).
func (e *Engine) reconcileStandardfeed(
	ctx context.Context,
	did syntax.DID,
	sess *oauth.ClientSession,
	snapshot []db.UserSubscription,
	snapshotAt time.Time,
	morgen []PDSSubscription,
	standard []PDSStandardSubscription,
	onAdded func(feedURL string),
) error {
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

	err := reconcileCollection(ctx, e.runTx, reconcilePass[db.UserSubscription]{
		collection:           "subscriptions.standardfeed",
		snapshotAt:           snapshotAt,
		baseline:             local,
		guardLocalChanges:    true,
		updatedAtOf:          func(row db.UserSubscription) string { return row.UpdatedAt },
		changedSinceSnapshot: subscriptionChangedSinceSnapshot,
		snapshot: func(ctx context.Context, q SyncStore) ([]db.UserSubscription, error) {
			rows, err := q.ListUserSubscriptionsForSync(ctx, didStr)
			return filterSubscriptions(rows, isStandardfeed), err
		},
		rkeyOf:  func(row db.UserSubscription) string { return row.Rkey },
		desired: desired,
		deleteRow: func(ctx context.Context, q SyncStore, rkey string) error {
			return q.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{Did: didStr, Rkey: rkey})
		},
		// A canonical-rkey change (duplicate collapse, delete+recreate elsewhere) would trip UNIQUE(did, feed_url) if the new upsert ran before the stale row was gone.
		deleteFirst: true,
	})
	if err != nil {
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
	return err
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

func rkeySet(rows []db.UserSubscription) map[string]struct{} {
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		set[row.Rkey] = struct{}{}
	}
	return set
}

func subscriptionChangedSinceSnapshot(current, baseline db.UserSubscription) bool {
	return current.AtUri != baseline.AtUri || current.FeedUrl != baseline.FeedUrl || current.Kind != baseline.Kind ||
		current.IsPrimary != baseline.IsPrimary || current.CreatedAt != baseline.CreatedAt || current.UpdatedAt != baseline.UpdatedAt ||
		optionalStringChanged(current.SidecarRkey, baseline.SidecarRkey) || optionalStringChanged(current.Title, baseline.Title) ||
		optionalStringChanged(current.Tags, baseline.Tags)
}

func optionalStringChanged(current, baseline *string) bool {
	if current == nil || baseline == nil {
		return current != baseline
	}
	return *current != *baseline
}

// filterSubscriptions keeps the RSS and Standardfeed passes separate when comparing pre-list and in-transaction rows.
func filterSubscriptions(snapshot []db.UserSubscription, keep func(db.UserSubscription) bool) []db.UserSubscription {
	out := make([]db.UserSubscription, 0, len(snapshot))
	for _, row := range snapshot {
		if keep(row) {
			out = append(out, row)
		}
	}
	return out
}

func isStandardfeed(row db.UserSubscription) bool { return row.Kind == "standardfeed" }

// A row whose kind predates the column reads as rss, so the rss pass claims everything the standardfeed pass does not.
func isRSS(row db.UserSubscription) bool { return !isStandardfeed(row) }

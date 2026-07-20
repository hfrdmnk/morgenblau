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

	desired := make(map[string]struct{}, len(canonicalByDoc)+len(rssShares))
	for _, rec := range canonicalByDoc {
		desired[rec.Rkey] = struct{}{}
	}
	for _, s := range rssShares {
		desired[s.Rkey] = struct{}{}
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// One tx: snapshot read, deletes, upserts; a read failure rolls back, per-statement write errors are tolerated. Sidecar cleanup stays post-commit since it's a network write.
	if err := e.runTx(ctx, func(q SyncStore) error {
		snapshot, err := q.ListUserSharesForSync(ctx, didStr)
		if err != nil {
			return err
		}

		// Deletes first: a changed canonical rkey leaves the stale row holding (did, document), which would trip the partial unique index on the new upsert.
		for _, row := range snapshot {
			if _, keep := desired[row.Rkey]; keep {
				continue
			}
			if err := q.DeleteUserShare(ctx, db.DeleteUserShareParams{
				Did:  didStr,
				Rkey: row.Rkey,
			}); err != nil {
				slog.Warn("reconcile: share delete failed", "rkey", row.Rkey, "err", err)
			}
		}

		for _, rec := range canonicalByDoc {
			doc := rec.Document
			createdAt := rec.CreatedAt
			if createdAt == "" {
				createdAt = now
			}
			var (
				itemURL     *string
				comment     *string
				feedURL     *string
				sidecarRkey *string
			)
			if sc, ok := sidecarByDoc[doc]; ok {
				itemURL = nilIfEmpty(sc.ItemURL)
				comment = nilIfEmpty(sc.Comment)
				feedURL = nilIfEmpty(sc.FeedURL)
				rk := sc.Rkey
				sidecarRkey = &rk
			}
			// No sidecar itemUrl (bare recommend): backfill from the cached entry so the display fallback survives the entry's later deletion, matching what the API stores at share time.
			if itemURL == nil {
				if url, err := q.GetFeedEntryURLByGuid(ctx, doc); err == nil {
					itemURL = nilIfEmpty(url)
				}
			}
			if err := q.UpsertUserShare(ctx, db.UpsertUserShareParams{
				Did:         didStr,
				Rkey:        rec.Rkey,
				AtUri:       rec.URI,
				Kind:        "standardfeed",
				ItemUrl:     itemURL,
				Document:    &doc,
				Comment:     comment,
				FeedUrl:     feedURL,
				SidecarRkey: sidecarRkey,
				CreatedAt:   createdAt,
				UpdatedAt:   now,
			}); err != nil {
				slog.Warn("reconcile: share upsert failed", "rkey", rec.Rkey, "err", err)
			}
		}

		for _, s := range rssShares {
			createdAt := s.CreatedAt
			if createdAt == "" {
				createdAt = now
			}
			itemURL := s.ItemURL
			if err := q.UpsertUserShare(ctx, db.UpsertUserShareParams{
				Did:       didStr,
				Rkey:      s.Rkey,
				AtUri:     s.URI,
				Kind:      "rss",
				ItemUrl:   &itemURL,
				Comment:   nilIfEmpty(s.Comment),
				FeedUrl:   nilIfEmpty(s.FeedURL),
				CreatedAt: createdAt,
				UpdatedAt: now,
			}); err != nil {
				slog.Warn("reconcile: rss share upsert failed", "rkey", s.Rkey, "err", err)
			}
		}
		return nil
	}); err != nil {
		slog.Warn("reconcile: shares tx failed", "did", didStr, "err", err)
	}

	// Deletes orphaned and non-canonical sidecars; covered by the blue.morgen grant, no standard-scope check needed.
	sidecarCleanup(ctx, e.pds, sess, syntax.NSID(shareCollection), sidecars,
		func(s PDSShare) string { return s.Document },
		func(s PDSShare) string { return s.Rkey },
		canonicalByDoc, sidecarByDoc,
		func(rkey string, err error) {
			slog.Warn("reconcile: share sidecar cleanup failed", "rkey", rkey, "err", err)
		})
	return nil
}

package database

import (
	"context"
	"database/sql"
	"time"

	"morgenblau/internal/database/db"
)

// FeedSweepGrace is how long a feed nobody follows, or a web source nobody follows or was offered, stays in the shared cache.
const FeedSweepGrace = 30 * 24 * time.Hour

type SweepResult struct {
	Feeds, WebSources int64
}

// SweepUnfollowedFeeds deletes feeds, and the web sources behind them, once nobody has followed them for FeedSweepGrace.
func SweepUnfollowedFeeds(ctx context.Context, writer *sql.DB, now time.Time) (SweepResult, error) {
	stamp := now.UTC().Format(time.RFC3339)
	cutoff := now.Add(-FeedSweepGrace).UTC().Format(time.RFC3339)
	var res SweepResult
	err := WithTx(ctx, writer, func(q *db.Queries) error {
		if err := q.ClaimFollowedFeeds(ctx); err != nil {
			return err
		}
		if err := q.MarkOrphanedFeeds(ctx, &stamp); err != nil {
			return err
		}
		n, err := q.DeleteOrphanedFeeds(ctx, &cutoff)
		if err != nil {
			return err
		}
		res.Feeds = n
		res.WebSources, err = q.DeleteUnfollowedWebSources(ctx, cutoff)
		return err
	})
	return res, err
}

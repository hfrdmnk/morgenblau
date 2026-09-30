package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"morgenblau/internal/database/db"
)

func subscriptionImportSnapshot(ctx context.Context, q *db.Queries, did, feedURL string) (*db.UserSubscription, error) {
	row, err := q.GetUserSubscriptionByFeedURL(ctx, db.GetUserSubscriptionByFeedURLParams{Did: did, FeedUrl: feedURL})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (d *DB) SnapshotImportedSubscription(ctx context.Context, did, feedURL string) (*db.UserSubscription, error) {
	return subscriptionImportSnapshot(ctx, db.New(d.Reader), did, feedURL)
}

func (d *DB) MirrorImportedSubscription(ctx context.Context, baseline *db.UserSubscription, feed db.UpsertFeedParams, row db.UpsertUserSubscriptionParams) error {
	return WithTx(ctx, d.Writer, func(q *db.Queries) error {
		current, err := subscriptionImportSnapshot(ctx, q, row.Did, row.FeedUrl)
		if err != nil {
			return err
		}
		// Field comparison also catches writes sharing a timestamp with the baseline.
		if !reflect.DeepEqual(current, baseline) {
			return fmt.Errorf("subscription changed during PDS import")
		}
		if err := q.UpsertFeed(ctx, feed); err != nil {
			return err
		}
		return q.UpsertUserSubscription(ctx, row)
	})
}

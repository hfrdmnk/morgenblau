package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// txRunner is Engine.runTx's shape, taken as a parameter so the reconcile core is exercisable without an Engine.
type txRunner func(ctx context.Context, fn func(SyncStore) error) error

// desiredRow is one record the PDS says must exist locally, paired with the writes that make it so.
type desiredRow struct {
	rkey  string
	write func(ctx context.Context, q SyncStore) error
}

// reconcilePass is one collection's contribution to reconcileCollection: how to read the local
// side, what the PDS says should survive, and how to write each side.
type reconcilePass[L any] struct {
	collection string

	// snapshotAt must be taken before the PDS listing, so a row written during the round-trip reads as newer.
	snapshotAt time.Time

	// snapshot reads the local side inside the tx; its failure rolls the whole pass back rather than deleting against a partial view.
	snapshot func(ctx context.Context, q SyncStore) ([]L, error)

	rkeyOf func(L) string

	// createdAtOf feeds the in-flight guard; nil where the snapshot query carries no created_at column.
	createdAtOf func(L) string

	desired []desiredRow

	deleteRow func(ctx context.Context, q SyncStore, rkey string) error

	// deleteFirst is required wherever a unique index other than rkey means a rekeyed record must vacate before its replacement upserts.
	deleteFirst bool
}

// reconcileCollection applies one pass's diff in a single transaction: local rows the PDS no
// longer lists are deleted, everything the PDS lists is upserted.
func reconcileCollection[L any](ctx context.Context, runTx txRunner, p reconcilePass[L]) error {
	keep := make(map[string]struct{}, len(p.desired))
	for _, d := range p.desired {
		keep[d.rkey] = struct{}{}
	}

	return runTx(ctx, func(q SyncStore) error {
		local, err := p.snapshot(ctx, q)
		if err != nil {
			return err
		}

		deleteStale := func(fatal bool) error {
			for _, row := range local {
				rkey := p.rkeyOf(row)
				if _, alive := keep[rkey]; alive {
					continue
				}
				if p.createdAtOf != nil && createdAfterSnapshot(p.createdAtOf(row), p.snapshotAt) {
					slog.Debug("reconcile: delete skipped, row newer than the PDS snapshot", "collection", p.collection, "rkey", rkey, "createdAt", p.createdAtOf(row))
					continue
				}
				if err := p.deleteRow(ctx, q, rkey); err != nil {
					if fatal {
						return fmt.Errorf("reconcile %s delete %q: %w", p.collection, rkey, err)
					}
					slog.Warn("reconcile: delete failed", "collection", p.collection, "rkey", rkey, "err", err)
				}
			}
			return nil
		}
		upsertDesired := func(fatal bool) error {
			for _, d := range p.desired {
				if err := d.write(ctx, q); err != nil {
					if fatal {
						return fmt.Errorf("reconcile %s upsert %q: %w", p.collection, d.rkey, err)
					}
					slog.Warn("reconcile: upsert failed", "collection", p.collection, "rkey", d.rkey, "err", err)
				}
			}
			return nil
		}

		if p.deleteFirst {
			if err := deleteStale(true); err != nil {
				return err
			}
			return upsertDesired(true)
		}
		_ = upsertDesired(false)
		_ = deleteStale(false)
		return nil
	})
}

// orNow backfills a record whose optional createdAt is absent, so the local row never carries an empty timestamp.
func orNow(createdAt, now string) string {
	if createdAt == "" {
		return now
	}
	return createdAt
}

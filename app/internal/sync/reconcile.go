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

	// baseline is the local side read before the PDS listing; rows that differ from it in the tx were written during the listing and win.
	baseline []L

	rkeyOf func(L) string

	updatedAtOf func(L) string

	changedSinceSnapshot func(current, baseline L) bool

	desired []desiredRow

	deleteRow func(ctx context.Context, q SyncStore, rkey string) error

	// deleteFirst is required wherever a unique index other than rkey means a rekeyed record must vacate before its replacement upserts.
	deleteFirst bool
}

// committed proves one reconcile pass's transaction committed; only reconcileCollection mints it, so finishSync cannot be satisfied by a swallowed error.
type committed[L any] struct{}

// reconcileCollection applies one pass's diff in a single transaction: local rows the PDS no
// longer lists are deleted, everything the PDS lists is upserted.
func reconcileCollection[L any](ctx context.Context, runTx txRunner, p reconcilePass[L]) (*committed[L], error) {
	keep := make(map[string]struct{}, len(p.desired))
	for _, d := range p.desired {
		keep[d.rkey] = struct{}{}
	}

	err := runTx(ctx, func(q SyncStore) error {
		local, err := p.snapshot(ctx, q)
		if err != nil {
			return err
		}
		changedDuringListing := make(map[string]struct{})
		deletedDuringListing := make(map[string]struct{})
		baselineByRkey := make(map[string]L, len(p.baseline))
		for _, row := range p.baseline {
			baselineByRkey[p.rkeyOf(row)] = row
		}
		currentByRkey := make(map[string]L, len(local))
		for _, row := range local {
			rkey := p.rkeyOf(row)
			currentByRkey[rkey] = row
			baseline, existed := baselineByRkey[rkey]
			if !existed || (p.changedSinceSnapshot != nil && p.changedSinceSnapshot(row, baseline)) ||
				(p.updatedAtOf != nil && updatedAfterSnapshot(p.updatedAtOf(row), p.snapshotAt)) {
				changedDuringListing[rkey] = struct{}{}
			}
		}
		for rkey := range baselineByRkey {
			if _, exists := currentByRkey[rkey]; !exists {
				deletedDuringListing[rkey] = struct{}{}
			}
		}

		deleteStale := func() error {
			for _, row := range local {
				rkey := p.rkeyOf(row)
				if _, alive := keep[rkey]; alive {
					continue
				}
				if _, fresh := changedDuringListing[rkey]; fresh {
					slog.Debug("reconcile: delete skipped, local row changed during the PDS listing", "collection", p.collection, "rkey", rkey)
					continue
				}
				if err := p.deleteRow(ctx, q, rkey); err != nil {
					return fmt.Errorf("reconcile %s delete %q: %w", p.collection, rkey, err)
				}
			}
			return nil
		}
		upsertDesired := func() error {
			for _, d := range p.desired {
				if _, deleted := deletedDuringListing[d.rkey]; deleted {
					slog.Debug("reconcile: upsert skipped, local row was deleted during the PDS listing", "collection", p.collection, "rkey", d.rkey)
					continue
				}
				if _, fresh := changedDuringListing[d.rkey]; fresh {
					slog.Debug("reconcile: upsert skipped, local row changed during the PDS listing", "collection", p.collection, "rkey", d.rkey)
					continue
				}
				if err := d.write(ctx, q); err != nil {
					return fmt.Errorf("reconcile %s upsert %q: %w", p.collection, d.rkey, err)
				}
			}
			return nil
		}

		if p.deleteFirst {
			if err := deleteStale(); err != nil {
				return err
			}
			return upsertDesired()
		}
		if err := upsertDesired(); err != nil {
			return err
		}
		return deleteStale()
	})
	if err != nil {
		return nil, err
	}
	return &committed[L]{}, nil
}

// orNow backfills a record whose optional createdAt is absent, so the local row never carries an empty timestamp.
func orNow(createdAt, now string) string {
	if createdAt == "" {
		return now
	}
	return createdAt
}

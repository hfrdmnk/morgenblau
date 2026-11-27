package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type txRunner func(ctx context.Context, fn func(SyncStore) error) error

type desiredRow struct {
	rkey  string
	write func(ctx context.Context, q SyncStore) error
}

type reconcilePass[L any] struct {
	collection           string
	snapshotAt           time.Time
	snapshot             func(ctx context.Context, q SyncStore) ([]L, error)
	baseline             []L
	rkeyOf               func(L) string
	updatedAtOf          func(L) string
	changedSinceSnapshot func(current, baseline L) bool
	desired              []desiredRow
	deleteRow            func(ctx context.Context, q SyncStore, rkey string) error
}

type committed[L any] struct{}

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
	})
	if err != nil {
		return nil, err
	}
	return &committed[L]{}, nil
}

func orNow(createdAt, now string) string {
	if createdAt == "" {
		return now
	}
	return createdAt
}

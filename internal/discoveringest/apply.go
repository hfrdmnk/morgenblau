package discoveringest

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"morgenblau/internal/database/db"
)

// MirrorStore is the ingest write surface over the record mirror and the stream position; *db.Queries satisfies it.
type MirrorStore interface {
	UpsertTapRecord(ctx context.Context, arg db.UpsertTapRecordParams) error
	DeleteTapRecord(ctx context.Context, arg db.DeleteTapRecordParams) error
	DeleteTapRecordsForRepo(ctx context.Context, did string) error
	UpsertTapRepoHandle(ctx context.Context, arg db.UpsertTapRepoHandleParams) error
	UpsertTapRepoAccount(ctx context.Context, arg db.UpsertTapRepoAccountParams) error
	MarkTapRepoDirty(ctx context.Context, arg db.MarkTapRepoDirtyParams) error
	TapRepoIsMirrored(ctx context.Context, did string) (bool, error)
	UpsertDiscoverIngestCursor(ctx context.Context, arg db.UpsertDiscoverIngestCursorParams) error
}

// MirrorRecord is one record a re-crawl produced, ready for the mirror.
type MirrorRecord struct {
	Collection string
	Rkey       string
	CID        string
	Record     string
}

// applyEvent routes one decoded event, live or archived, onto its mirror write. All network I/O happens before any transaction opens.
func (c *Consumer) applyEvent(ctx context.Context, ev event) error {
	switch ev.Kind {
	case kindCommit:
		op := *ev.Commit
		if op.DID == "" || op.Rkey == "" || !knownAction(op.Action) || !tracked(op.Collection) {
			return nil
		}
		return c.applyCommit(ctx, op)
	case kindIdentity:
		if ev.Identity.DID == "" {
			return nil
		}
		return c.applyIdentity(ctx, *ev.Identity)
	case kindAccount:
		if ev.Account.DID == "" {
			return nil
		}
		return c.applyAccount(ctx, *ev.Account)
	case kindSync:
		if ev.Sync.DID == "" {
			return nil
		}
		return c.applySync(ctx, *ev.Sync)
	default:
		return nil
	}
}

// applyCommit writes one record change and its dirty mark in a single transaction, so the rebuild worker never sees a mirror change without the mark that schedules it.
func (c *Consumer) applyCommit(ctx context.Context, op commitOp) error {
	var payload string
	if op.Action != actionDelete && len(op.Record) > 0 && !bytes.Equal(op.Record, []byte("null")) {
		compacted, err := compactJSON(op.Record)
		if err != nil {
			return err
		}
		payload = compacted
	}
	stamp := c.stamp()

	return c.runTx(ctx, func(s MirrorStore) error {
		switch {
		case op.Action == actionDelete:
			if err := s.DeleteTapRecord(ctx, db.DeleteTapRecordParams{
				Did:        op.DID,
				Collection: op.Collection,
				Rkey:       op.Rkey,
			}); err != nil {
				return err
			}
		case payload != "":
			if err := s.UpsertTapRecord(ctx, db.UpsertTapRecordParams{
				Did:        op.DID,
				Collection: op.Collection,
				Rkey:       op.Rkey,
				Cid:        op.CID,
				Record:     payload,
				IndexedAt:  stamp,
			}); err != nil {
				return err
			}
		}
		// A body-less create means the record would not decode; the repo is still marked so the rebuild reconciles from whatever is mirrored.
		return s.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: op.DID, MarkedAt: stamp})
	})
}

// applyIdentity records a handle change and nothing else: hosting status arrives on its own event kind.
func (c *Consumer) applyIdentity(ctx context.Context, ch identityChange) error {
	stamp := c.stamp()
	return c.runTx(ctx, func(s MirrorStore) error {
		mirrored, err := s.TapRepoIsMirrored(ctx, ch.DID)
		if err != nil || !mirrored {
			return err
		}
		if err := s.UpsertTapRepoHandle(ctx, db.UpsertTapRepoHandleParams{
			Did:       ch.DID,
			Handle:    ch.Handle,
			UpdatedAt: stamp,
		}); err != nil {
			return err
		}
		return s.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: ch.DID, MarkedAt: stamp})
	})
}

// applyAccount records hosting status. Deletion purges the repo's records; every other inactive status retains them, so a reactivation restores the repo's signals without a re-crawl.
func (c *Consumer) applyAccount(ctx context.Context, ch accountChange) error {
	stamp := c.stamp()
	var isActive int64
	if ch.Active {
		isActive = 1
	}
	return c.runTx(ctx, func(s MirrorStore) error {
		mirrored, err := s.TapRepoIsMirrored(ctx, ch.DID)
		if err != nil || !mirrored {
			return err
		}
		if err := s.UpsertTapRepoAccount(ctx, db.UpsertTapRepoAccountParams{
			Did:       ch.DID,
			IsActive:  isActive,
			Status:    ch.Status,
			UpdatedAt: stamp,
		}); err != nil {
			return err
		}
		if !ch.Active && ch.Status == statusDeleted {
			if err := s.DeleteTapRecordsForRepo(ctx, ch.DID); err != nil {
				return err
			}
		}
		return s.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: ch.DID, MarkedAt: stamp})
	})
}

// applySync repairs a diverged repo: the mirror cannot be reconciled from a stream of ops, so the whole repo is re-read from its PDS and replaced.
func (c *Consumer) applySync(ctx context.Context, m syncMarker) error {
	mirrored, err := c.isMirrored(ctx, m.DID)
	if err != nil || !mirrored {
		return err
	}
	records, err := c.records.FetchRepoRecords(ctx, m.DID)
	if err != nil {
		return err
	}
	stamp := c.stamp()

	return c.runTx(ctx, func(s MirrorStore) error {
		if err := s.DeleteTapRecordsForRepo(ctx, m.DID); err != nil {
			return err
		}
		for _, r := range records {
			if !tracked(r.Collection) {
				continue
			}
			if err := s.UpsertTapRecord(ctx, db.UpsertTapRecordParams{
				Did:        m.DID,
				Collection: r.Collection,
				Rkey:       r.Rkey,
				Cid:        r.CID,
				Record:     r.Record,
				IndexedAt:  stamp,
			}); err != nil {
				return err
			}
		}
		return s.MarkTapRepoDirty(ctx, db.MarkTapRepoDirtyParams{Did: m.DID, MarkedAt: stamp})
	})
}

// isMirrored runs the membership probe on its own so a divergence re-crawl is skipped before its network cost is paid.
func (c *Consumer) isMirrored(ctx context.Context, did string) (bool, error) {
	var mirrored bool
	err := c.runTx(ctx, func(s MirrorStore) error {
		var err error
		mirrored, err = s.TapRepoIsMirrored(ctx, did)
		return err
	})
	return mirrored, err
}

// persistPosition writes the whole stream position, including the bootstrap columns, in one statement.
func (c *Consumer) persistPosition(ctx context.Context, pos position) error {
	return c.runTx(ctx, func(s MirrorStore) error {
		return s.UpsertDiscoverIngestCursor(ctx, pos.params(c.stamp()))
	})
}

func (c *Consumer) stamp() string {
	return c.now().UTC().Format(time.RFC3339)
}

// compactJSON strips transport whitespace without reordering keys, keeping the mirrored bytes byte-comparable to what the PDS serves.
func compactJSON(raw json.RawMessage) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", err
	}
	return buf.String(), nil
}

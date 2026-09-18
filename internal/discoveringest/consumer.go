package discoveringest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

// Config is the operator-configured Jetstream instance. APIKey is optional and scoped by the official client to archive requests.
type Config struct {
	URL    string
	APIKey string
}

// CursorReader reads the persisted stream position; wire it to the reader pool.
type CursorReader interface {
	GetDiscoverIngestCursor(ctx context.Context) (db.GetDiscoverIngestCursorRow, error)
}

// Consumer owns one official Jetstream client and folds its batches into the mirror.
type Consumer struct {
	cfg     Config
	cursors CursorReader
	factory sourceFactory
	runTx   func(ctx context.Context, fn func(MirrorStore) error) error
	now     func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	source eventSource
}

// NewConsumer keeps the transport dependency private to discoveringest.
func NewConsumer(cfg Config, cursors CursorReader) *Consumer {
	return newConsumer(cfg, cursors, newOfficialSource)
}

func newConsumer(cfg Config, cursors CursorReader, factory sourceFactory) *Consumer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Consumer{
		cfg: cfg, cursors: cursors, factory: factory, now: time.Now,
		ctx: ctx, cancel: cancel,
		runTx: func(context.Context, func(MirrorStore) error) error {
			return errors.New("discoveringest: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits each delivered batch in one SQLite write transaction.
func (c *Consumer) WithTxRunner(w *sql.DB) *Consumer {
	c.runTx = func(ctx context.Context, fn func(MirrorStore) error) error {
		return database.WithTx(ctx, w, func(q *db.Queries) error { return fn(q) })
	}
	return c
}

// Start launches the event loop. It is not safe to call more than once.
func (c *Consumer) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.run()
	}()
}

// Shutdown cancels and closes the active source, then waits for the event loop to drain.
func (c *Consumer) Shutdown(ctx context.Context) error {
	c.cancel()
	c.mu.Lock()
	source := c.source
	c.mu.Unlock()
	if source != nil {
		_ = source.Close()
	}
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Consumer) run() {
	req, err := c.request(c.ctx)
	if err != nil {
		if c.ctx.Err() == nil {
			slog.Warn("discoveringest: stream position read failed", "err", err)
		}
		return
	}
	if c.factory == nil {
		slog.Error("discoveringest: event source factory is not configured")
		return
	}
	source, err := c.factory(req)
	if err != nil {
		if c.ctx.Err() == nil {
			slog.Warn("discoveringest: subscribe failed", "err", err)
		}
		return
	}
	c.mu.Lock()
	c.source = source
	c.mu.Unlock()
	defer func() {
		_ = source.Close()
		c.mu.Lock()
		if c.source == source {
			c.source = nil
		}
		c.mu.Unlock()
	}()

	for batch, streamErr := range source.Events(c.ctx) {
		if streamErr != nil {
			if c.ctx.Err() == nil {
				slog.Warn("discoveringest: stream error", "err", streamErr)
			}
			continue
		}
		if err := c.foldBatch(c.ctx, batch); err != nil {
			if c.ctx.Err() == nil {
				slog.Warn("discoveringest: batch fold failed; cursor left unchanged", "cursor", batch.cursor, "err", err)
			}
			return
		}
	}
}

func (c *Consumer) request(ctx context.Context) (sourceRequest, error) {
	req := sourceRequest{Host: c.cfg.URL, Collections: append([]string(nil), Collections...), APIKey: c.cfg.APIKey}
	row, err := c.cursors.GetDiscoverIngestCursor(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		zero := uint64(0)
		req.AfterSeq = &zero
		return req, nil
	}
	if err != nil {
		return sourceRequest{}, err
	}
	if row.Seq < 0 {
		return sourceRequest{}, fmt.Errorf("discoveringest: stored cursor is negative: %d", row.Seq)
	}
	live := uint64(row.Seq)
	req.LiveCursor = &live
	return req, nil
}

func (c *Consumer) foldBatch(ctx context.Context, batch sourceBatch) error {
	if batch.cursor > math.MaxInt64 {
		return fmt.Errorf("discoveringest: batch cursor %d exceeds SQLite INTEGER range", batch.cursor)
	}
	for i := range batch.events {
		if batch.events[i].Seq > math.MaxInt64 {
			return fmt.Errorf("discoveringest: event cursor %d exceeds SQLite INTEGER range", batch.events[i].Seq)
		}
	}
	return c.runTx(ctx, func(store MirrorStore) error {
		for i := range batch.events {
			if err := applyEvent(ctx, store, batch.events[i], c.now); err != nil {
				return fmt.Errorf("fold event %d: %w", i, err)
			}
		}
		return store.UpsertDiscoverIngestCursor(ctx, db.UpsertDiscoverIngestCursorParams{
			Seq:       int64(batch.cursor),
			UpdatedAt: c.now().UTC().Format(time.RFC3339),
		})
	})
}

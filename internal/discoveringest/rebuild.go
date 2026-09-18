package discoveringest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/standardfeed"
)

// Resolver supplies identity metadata needed when an authored publication has no mirrored handle.
type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

const (
	// defaultRebuildInterval paces the drain. The stream marks repos dirty continuously, so this is a coalescing window rather than a freshness budget.
	defaultRebuildInterval = 30 * time.Second
	// dirtyBatchLimit bounds one tick's work; the rest of the backlog waits for the next tick.
	dirtyBatchLimit = 100
)

// MirrorReader is the rebuild's read surface over the mirror; wire it to the reader pool.
type MirrorReader interface {
	ListTapDirtyRepos(ctx context.Context, limit int64) ([]db.TapDirtyRepo, error)
	ListTapRecordsForRepo(ctx context.Context, did string) ([]db.TapRecord, error)
	GetTapRepoState(ctx context.Context, did string) (db.TapRepoState, error)
}

// RecordDecoder turns mirrored rows into the shapes the reduction consumes; *discovercrawl.Client satisfies it.
// Both methods may reach the network (publication resolution, well-known probes), so neither may run inside a transaction.
type RecordDecoder interface {
	DecodeSubscriptions(ctx context.Context, byCollection map[string][]discovercrawl.RecordEntry) []discovercrawl.Subscription
	DecodeAuthoredPublications(ctx context.Context, byCollection map[string][]discovercrawl.RecordEntry, did syntax.DID, handle syntax.Handle) ([]discovercrawl.AuthoredPublication, error)
}

// RebuildWorker turns dirty mirrors into discover's trending aggregates.
type RebuildWorker struct {
	reader        MirrorReader
	decoder       RecordDecoder
	resolver      Resolver
	entries       EntryResolver
	runTx         func(ctx context.Context, fn func(rebuildWriter) error) error
	invalidateAll func()
	interval      time.Duration
	batchSize     int64
	now           func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRebuildWorker builds the worker. Callers must chain WithTxRunner; without one every repo fails and stays dirty (logged).
func NewRebuildWorker(reader MirrorReader, decoder RecordDecoder, resolver Resolver, entries EntryResolver) *RebuildWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &RebuildWorker{
		reader:    reader,
		decoder:   decoder,
		resolver:  resolver,
		entries:   entries,
		interval:  defaultRebuildInterval,
		batchSize: dirtyBatchLimit,
		now:       time.Now,
		ctx:       ctx,
		cancel:    cancel,
		runTx: func(ctx context.Context, fn func(rebuildWriter) error) error {
			return errors.New("discoveringest: no transaction runner configured (call WithTxRunner)")
		},
	}
}

// WithTxRunner commits each repo's aggregate replace in one transaction on the writer pool.
func (w *RebuildWorker) WithTxRunner(writer *sql.DB) *RebuildWorker {
	w.runTx = func(ctx context.Context, fn func(rebuildWriter) error) error {
		return database.WithTx(ctx, writer, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return w
}

// WithInvalidator registers the discover cache drop that runs after a productive drain; nil-safe.
func (w *RebuildWorker) WithInvalidator(fn func()) *RebuildWorker {
	w.invalidateAll = fn
	return w
}

// Start launches the drain ticker. Not safe to call more than once.
func (w *RebuildWorker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				w.drain(w.ctx)
			}
		}
	}()
}

// Shutdown cancels the worker and waits for the in-flight drain, or for ctx to expire.
func (w *RebuildWorker) Shutdown(ctx context.Context) error {
	w.cancel()
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// drain rebuilds one batch of dirty repos. A repo that fails keeps its mark and is retried next tick, so one bad repo never blocks the rest.
func (w *RebuildWorker) drain(ctx context.Context) {
	repos, err := w.reader.ListTapDirtyRepos(ctx, w.batchSize)
	if err != nil {
		slog.Warn("discoveringest: dirty repo read failed", "err", err)
		return
	}
	rebuilt := 0
	for _, repo := range repos {
		if ctx.Err() != nil {
			return
		}
		if err := w.rebuildRepo(ctx, repo); err != nil {
			slog.Warn("discoveringest: repo rebuild failed, leaving it dirty", "did", repo.Did, "err", err)
			continue
		}
		rebuilt++
	}
	if rebuilt == 0 {
		return
	}
	w.refreshTrendingCounts(ctx)
	if w.invalidateAll != nil {
		w.invalidateAll()
	}
	slog.Debug("discoveringest: rebuild drain complete", "repos", rebuilt)
}

// rebuildRepo decodes the repo's whole mirror and replaces its aggregate rows. All decoding, and therefore all network I/O, happens before the transaction opens.
func (w *RebuildWorker) rebuildRepo(ctx context.Context, repo db.TapDirtyRepo) error {
	did, err := syntax.ParseDID(repo.Did)
	if err != nil {
		return err
	}

	state, err := w.reader.GetTapRepoState(ctx, repo.Did)
	hasState := true
	if errors.Is(err, sql.ErrNoRows) {
		hasState = false
	} else if err != nil {
		return err
	}

	var signals map[string]repoSource
	var follows []discovercrawl.ReaderNetworkFollow
	if !hasState || state.IsActive != 0 {
		rows, err := w.reader.ListTapRecordsForRepo(ctx, repo.Did)
		if err != nil {
			return err
		}
		byCollection := partitionByCollection(repo.Did, rows)

		var pubs []discovercrawl.AuthoredPublication
		if len(byCollection[standardfeed.CollectionPublication]) > 0 {
			handle, err := w.repoHandle(ctx, did, state, hasState)
			if err != nil {
				return err
			}
			pubs, err = w.decoder.DecodeAuthoredPublications(ctx, byCollection, did, handle)
			if err != nil {
				return err
			}
		}

		signals = reduceRepoSignals(
			ctx,
			w.decoder.DecodeSubscriptions(ctx, byCollection),
			pubs,
			discovercrawl.MergeShares(byCollection),
			discovercrawl.DecodeSaves(byCollection),
			w.entries,
		)
		follows = discovercrawl.DecodeFollows(repo.Did, byCollection)
	}

	fetchedAt := w.now().UTC().Format(time.RFC3339)
	return w.runTx(ctx, func(x rebuildWriter) error {
		if err := replaceRepoSignals(ctx, x, repo.Did, signals, fetchedAt); err != nil {
			return err
		}
		if err := replaceRepoFollows(ctx, x, repo.Did, follows, fetchedAt); err != nil {
			return err
		}
		return x.DeleteTapDirtyRepo(ctx, db.DeleteTapDirtyRepoParams{Did: repo.Did, MarkedSeq: repo.MarkedSeq})
	})
}

func (w *RebuildWorker) repoHandle(ctx context.Context, did syntax.DID, state db.TapRepoState, hasState bool) (syntax.Handle, error) {
	if hasState && state.Handle != "" {
		if handle, err := syntax.ParseHandle(state.Handle); err == nil {
			return handle, nil
		}
	}
	ident, err := w.resolver.LookupDID(ctx, did)
	if err != nil {
		return "", err
	}
	return ident.Handle, nil
}

// refreshTrendingCounts rebuilds both quality-bar count tables in one transaction; every trending read joins them, so skipping it leaves the surfaces empty however many signals were written.
func (w *RebuildWorker) refreshTrendingCounts(ctx context.Context) {
	if err := w.runTx(ctx, func(x rebuildWriter) error {
		if err := x.DeleteDiscoverTrendingSourceCounts(ctx); err != nil {
			return err
		}
		if err := x.RebuildDiscoverTrendingSourceCounts(ctx); err != nil {
			return err
		}
		if err := x.DeleteDiscoverTrendingFollowCounts(ctx); err != nil {
			return err
		}
		return x.RebuildDiscoverTrendingFollowCounts(ctx)
	}); err != nil {
		// Prior counts survive the failed transaction, so trending stays on the previous bar rather than going dark.
		slog.Warn("discoveringest: trending counts refresh failed", "err", err)
	}
}

// partitionByCollection bridges mirror rows to the crawl's record shape. An unparseable row is dropped rather than failing the repo: it would fail identically on every retry.
func partitionByCollection(did string, rows []db.TapRecord) map[string][]discovercrawl.RecordEntry {
	out := make(map[string][]discovercrawl.RecordEntry)
	for _, row := range rows {
		var value map[string]any
		if err := json.Unmarshal([]byte(row.Record), &value); err != nil {
			slog.Warn("discoveringest: skipping unparseable mirror row", "did", did, "collection", row.Collection, "rkey", row.Rkey, "err", err)
			continue
		}
		out[row.Collection] = append(out[row.Collection], discovercrawl.NewRecordEntry(did, row.Collection, row.Rkey, row.Cid, value))
	}
	return out
}

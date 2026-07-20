package sharemeta

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
)

const (
	DefaultTTL              = 24 * time.Hour
	batchBudget             = 10 * time.Second
	resolveConcurrencyLimit = 8
)

var metadataBackoff = backoff.Policy{Steps: backoff.Exponential(5*time.Minute, 2, 24*time.Hour)}

type EntryReader interface {
	GetFeedEntryShareMetadataByDocument(ctx context.Context, guid string) (db.GetFeedEntryShareMetadataByDocumentRow, error)
	GetFeedEntryShareMetadataByItemURL(ctx context.Context, url string) (db.GetFeedEntryShareMetadataByItemURLRow, error)
}

type CacheReader interface {
	GetShareMetadataCache(ctx context.Context, targetKey string) (db.ShareMetadataCache, error)
}

type CacheWriter interface {
	UpsertShareMetadataSuccess(ctx context.Context, arg db.UpsertShareMetadataSuccessParams) error
	RecordShareMetadataFailure(ctx context.Context, arg db.RecordShareMetadataFailureParams) error
}

type MetadataFetcher interface {
	Fetch(ctx context.Context, target Target) (Metadata, error)
}

type Resolver struct {
	entries EntryReader
	cache   CacheReader
	fetcher MetadataFetcher
	ttl     time.Duration
	now     func() time.Time
	runTx   func(ctx context.Context, fn func(CacheWriter) error) error

	group singleflight.Group
	sem   chan struct{}
}

func NewResolver(entries EntryReader, cache CacheReader, fetcher MetadataFetcher, ttl time.Duration) *Resolver {
	return &Resolver{
		entries: entries,
		cache:   cache,
		fetcher: fetcher,
		ttl:     ttl,
		now:     time.Now,
		sem:     make(chan struct{}, resolveConcurrencyLimit),
		runTx: func(context.Context, func(CacheWriter) error) error {
			return errors.New("sharemeta: no transaction runner configured")
		},
	}
}

func (r *Resolver) WithTxRunner(writer *sql.DB) *Resolver {
	r.runTx = func(ctx context.Context, fn func(CacheWriter) error) error {
		return database.WithTx(ctx, writer, func(q *db.Queries) error {
			return fn(q)
		})
	}
	return r
}

func (r *Resolver) ResolveMany(ctx context.Context, targets []Target) []Metadata {
	results := make([]Metadata, len(targets))
	if len(targets) == 0 {
		return results
	}
	fetchCtx, cancel := context.WithTimeout(ctx, batchBudget)
	defer cancel()

	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(len(targets), resolveConcurrencyLimit) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = r.resolveOne(ctx, fetchCtx, targets[i])
			}
		}()
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func (r *Resolver) resolveOne(writeCtx, fetchCtx context.Context, target Target) Metadata {
	entry, haveEntry := r.entryMetadata(fetchCtx, target)
	if haveEntry && entry.Title != "" {
		return entry
	}

	key := targetKey(target)
	if key == "" {
		return entry
	}
	state, haveState := r.cacheState(fetchCtx, key)
	cached := metadataFromState(state)
	now := r.now()
	if haveState && state.FetchedAt != nil {
		if fetchedAt, err := time.Parse(time.RFC3339, *state.FetchedAt); err == nil && now.Sub(fetchedAt) < r.ttl {
			return mergeMetadata(cached, entry)
		}
	}
	if haveState && state.NextRetryAt != nil {
		if nextRetry, err := time.Parse(time.RFC3339, *state.NextRetryAt); err == nil && now.Before(nextRetry) {
			return mergeMetadata(cached, entry)
		}
	}

	value, _, _ := r.group.Do(key, func() (any, error) {
		if fetchCtx.Err() != nil {
			return cached, nil
		}
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		case <-fetchCtx.Done():
			return cached, nil
		}

		resolved, err := r.fetcher.Fetch(fetchCtx, target)
		if fetchCtx.Err() != nil {
			return cached, nil
		}
		if err != nil || resolved.Title == "" {
			r.recordFailure(writeCtx, key, state, haveState)
			return cached, nil
		}
		r.recordSuccess(writeCtx, key, resolved)
		return resolved, nil
	})
	return mergeMetadata(value.(Metadata), entry)
}

func (r *Resolver) entryMetadata(ctx context.Context, target Target) (Metadata, bool) {
	document := strings.TrimSpace(target.Document)
	if document != "" {
		row, err := r.entries.GetFeedEntryShareMetadataByDocument(ctx, document)
		if err == nil {
			return Metadata{
				Title:     deref(row.Title),
				TargetURL: row.Url,
				EntrySlug: row.EntrySlug,
			}, true
		}
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("sharemeta: document entry lookup failed", "document", document, "err", err)
		}
	}

	itemURL := strings.TrimSpace(target.ItemURL)
	if itemURL == "" {
		return Metadata{}, false
	}
	row, err := r.entries.GetFeedEntryShareMetadataByItemURL(ctx, itemURL)
	if err == nil {
		return Metadata{
			Title:     deref(row.Title),
			TargetURL: row.Url,
			EntrySlug: row.EntrySlug,
		}, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("sharemeta: item URL entry lookup failed", "itemUrl", itemURL, "err", err)
	}
	return Metadata{}, false
}

func (r *Resolver) cacheState(ctx context.Context, key string) (db.ShareMetadataCache, bool) {
	state, err := r.cache.GetShareMetadataCache(ctx, key)
	if err == nil {
		return state, true
	}
	if !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("sharemeta: cache read failed", "key", key, "err", err)
	}
	return db.ShareMetadataCache{}, false
}

func (r *Resolver) recordSuccess(ctx context.Context, key string, metadata Metadata) {
	fetchedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.runTx(ctx, func(writer CacheWriter) error {
		return writer.UpsertShareMetadataSuccess(ctx, db.UpsertShareMetadataSuccessParams{
			TargetKey: key,
			Title:     nilIfBlank(metadata.Title),
			TargetUrl: nilIfBlank(metadata.TargetURL),
			FetchedAt: &fetchedAt,
		})
	}); err != nil {
		slog.Warn("sharemeta: cache write failed", "key", key, "err", err)
	}
}

func (r *Resolver) recordFailure(ctx context.Context, key string, prior db.ShareMetadataCache, havePrior bool) {
	var failures int64 = 1
	if havePrior {
		failures = prior.FailureCount + 1
	}
	nextRetryAt := r.now().Add(metadataBackoff.Delay(int(failures))).UTC().Format(time.RFC3339)
	if err := r.runTx(ctx, func(writer CacheWriter) error {
		return writer.RecordShareMetadataFailure(ctx, db.RecordShareMetadataFailureParams{
			TargetKey:    key,
			FailureCount: failures,
			NextRetryAt:  &nextRetryAt,
		})
	}); err != nil {
		slog.Warn("sharemeta: failure record write failed", "key", key, "err", err)
	}
}

func targetKey(target Target) string {
	if document := strings.TrimSpace(target.Document); document != "" {
		return document
	}
	return strings.TrimSpace(target.ItemURL)
}

func metadataFromState(state db.ShareMetadataCache) Metadata {
	return Metadata{Title: deref(state.Title), TargetURL: deref(state.TargetUrl)}
}

func mergeMetadata(primary, fallback Metadata) Metadata {
	if primary.Title == "" {
		primary.Title = fallback.Title
	}
	if primary.TargetURL == "" {
		primary.TargetURL = fallback.TargetURL
	}
	if primary.EntrySlug == "" {
		primary.EntrySlug = fallback.EntrySlug
	}
	return primary
}

func nilIfBlank(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

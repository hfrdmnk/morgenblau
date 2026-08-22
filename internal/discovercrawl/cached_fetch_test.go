package discovercrawl

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// storeCall records one invocation of cachedFetch.store, for tests that assert it was (or wasn't) reached.
type storeCall struct {
	didStr    string
	results   []string
	fetchedAt string
}

func TestCachedFetch_ServesWithinTTL(t *testing.T) {
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	crawlCalls := 0
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return now },
		getState: func(context.Context, string) (string, error) {
			return now.Add(-30 * time.Minute).UTC().Format(time.RFC3339), nil
		},
		listCached: func(context.Context, string) ([]string, error) {
			return []string{"cached"}, nil
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			crawlCalls++
			return []string{"fresh"}, nil
		},
		store: func(context.Context, string, []string, string) error {
			t.Fatal("store must not run on a cache hit")
			return nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	got, err := cf.fetch(context.Background(), did)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if crawlCalls != 0 {
		t.Errorf("crawl calls = %d, want 0 (TTL hit must not re-crawl)", crawlCalls)
	}
	if len(got) != 1 || got[0] != "cached" {
		t.Fatalf("got = %v, want the cached rows", got)
	}
}

func TestCachedFetch_RecrawlsWhenStale(t *testing.T) {
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	crawlCalls := 0
	var stored storeCall
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return now },
		getState: func(context.Context, string) (string, error) {
			return now.Add(-2 * time.Hour).UTC().Format(time.RFC3339), nil
		},
		listCached: func(context.Context, string) ([]string, error) {
			t.Fatal("listCached must not run once the state is stale")
			return nil, nil
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			crawlCalls++
			return []string{"fresh"}, nil
		},
		store: func(_ context.Context, didStr string, results []string, fetchedAt string) error {
			stored = storeCall{didStr, results, fetchedAt}
			return nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	got, err := cf.fetch(context.Background(), did)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if crawlCalls != 1 {
		t.Errorf("crawl calls = %d, want 1 (stale state must re-crawl)", crawlCalls)
	}
	if len(got) != 1 || got[0] != "fresh" {
		t.Fatalf("got = %v, want the freshly crawled rows", got)
	}
	if stored.didStr != followedDID || len(stored.results) != 1 || stored.results[0] != "fresh" {
		t.Errorf("store call = %+v, want the fresh results written through", stored)
	}
}

func TestCachedFetch_NeverCrawled_Crawls(t *testing.T) {
	crawlCalls := 0
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return mustParseTime(t, "2026-07-09T12:00:00Z") },
		getState: func(context.Context, string) (string, error) {
			return "", sql.ErrNoRows
		},
		listCached: func(context.Context, string) ([]string, error) {
			t.Fatal("listCached must not run when never crawled")
			return nil, nil
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			crawlCalls++
			return []string{"fresh"}, nil
		},
		store: func(context.Context, string, []string, string) error {
			return nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	got, err := cf.fetch(context.Background(), did)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if crawlCalls != 1 {
		t.Errorf("crawl calls = %d, want 1", crawlCalls)
	}
	if len(got) != 1 || got[0] != "fresh" {
		t.Fatalf("got = %v, want the freshly crawled rows", got)
	}
}

func TestCachedFetch_StateReadErrorPropagatesWithoutCrawling(t *testing.T) {
	crawlCalls := 0
	wantErr := errors.New("db unavailable")
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return mustParseTime(t, "2026-07-09T12:00:00Z") },
		getState: func(context.Context, string) (string, error) {
			return "", wantErr
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			crawlCalls++
			return nil, nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	_, err := cf.fetch(context.Background(), did)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if crawlCalls != 0 {
		t.Errorf("crawl calls = %d, want 0 (a state read failure must not trigger a crawl)", crawlCalls)
	}
}

func TestCachedFetch_FreshRowReadErrorPropagatesWithoutCrawling(t *testing.T) {
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	crawlCalls := 0
	wantErr := errors.New("row read failed")
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return now },
		getState: func(context.Context, string) (string, error) {
			return now.Add(-30 * time.Minute).UTC().Format(time.RFC3339), nil
		},
		listCached: func(context.Context, string) ([]string, error) {
			return nil, wantErr
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			crawlCalls++
			return nil, nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	_, err := cf.fetch(context.Background(), did)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if crawlCalls != 0 {
		t.Errorf("crawl calls = %d, want 0", crawlCalls)
	}
}

func TestCachedFetch_CrawlErrorWithCachedRowsPresent(t *testing.T) {
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	wantErr := errors.New("pds unreachable")
	storeCalls := 0
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return now },
		getState: func(context.Context, string) (string, error) {
			// A prior crawl completed, but it's stale now.
			return now.Add(-2 * time.Hour).UTC().Format(time.RFC3339), nil
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			return nil, wantErr
		},
		store: func(context.Context, string, []string, string) error {
			storeCalls++
			return nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	_, err := cf.fetch(context.Background(), did)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if storeCalls != 0 {
		t.Errorf("store calls = %d, want 0 (a failed re-crawl must leave the stale cache untouched)", storeCalls)
	}
}

func TestCachedFetch_CrawlErrorWithNothingCached(t *testing.T) {
	wantErr := errors.New("pds unreachable")
	storeCalls := 0
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return mustParseTime(t, "2026-07-09T12:00:00Z") },
		getState: func(context.Context, string) (string, error) {
			return "", sql.ErrNoRows
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			return nil, wantErr
		},
		store: func(context.Context, string, []string, string) error {
			storeCalls++
			return nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	_, err := cf.fetch(context.Background(), did)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if storeCalls != 0 {
		t.Errorf("store calls = %d, want 0", storeCalls)
	}
}

func TestCachedFetch_CacheWriteFailureDegradesWithoutFailingTheServe(t *testing.T) {
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return mustParseTime(t, "2026-07-09T12:00:00Z") },
		getState: func(context.Context, string) (string, error) {
			return "", sql.ErrNoRows
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			return []string{"fresh"}, nil
		},
		store: func(context.Context, string, []string, string) error {
			return errors.New("write conflict")
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	got, err := cf.fetch(context.Background(), did)
	if err != nil {
		t.Fatalf("fetch: %v, want the crawl result served despite the write failure", err)
	}
	if len(got) != 1 || got[0] != "fresh" {
		t.Fatalf("got = %v, want the freshly crawled rows", got)
	}
}

func TestCachedFetch_NowInjection_DrivesFreshnessAndTheStampWritten(t *testing.T) {
	injected := mustParseTime(t, "2026-01-01T00:00:00Z") // far from wall-clock "now", so a real-time leak would fail this.
	var stampWritten string
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return injected },
		getState: func(context.Context, string) (string, error) {
			return "", sql.ErrNoRows
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			return []string{"fresh"}, nil
		},
		store: func(_ context.Context, _ string, _ []string, fetchedAt string) error {
			stampWritten = fetchedAt
			return nil
		},
	}
	did, _ := syntax.ParseDID(followedDID)

	if _, err := cf.fetch(context.Background(), did); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if want := injected.UTC().Format(time.RFC3339); stampWritten != want {
		t.Errorf("stamp written = %q, want %q (must come from the injected clock, not time.Now)", stampWritten, want)
	}
}

func TestCachedFetch_PostProcess_AppliesToBothCachedAndFreshResults(t *testing.T) {
	now := mustParseTime(t, "2026-07-09T12:00:00Z")
	dropFresh := func(rows []string) []string {
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			if r != "drop-me" {
				out = append(out, r)
			}
		}
		return out
	}

	t.Run("cached path", func(t *testing.T) {
		cf := cachedFetch[string]{
			ttl: time.Hour,
			now: func() time.Time { return now },
			getState: func(context.Context, string) (string, error) {
				return now.Add(-30 * time.Minute).UTC().Format(time.RFC3339), nil
			},
			listCached: func(context.Context, string) ([]string, error) {
				return []string{"keep-me", "drop-me"}, nil
			},
			postProcess: dropFresh,
		}
		did, _ := syntax.ParseDID(followedDID)
		got, err := cf.fetch(context.Background(), did)
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if len(got) != 1 || got[0] != "keep-me" {
			t.Fatalf("got = %v, want postProcess applied to the cached rows", got)
		}
	})

	t.Run("freshly crawled path", func(t *testing.T) {
		cf := cachedFetch[string]{
			ttl: time.Hour,
			now: func() time.Time { return now },
			getState: func(context.Context, string) (string, error) {
				return "", sql.ErrNoRows
			},
			crawl: func(context.Context, syntax.DID) ([]string, error) {
				return []string{"keep-me", "drop-me"}, nil
			},
			store:       func(context.Context, string, []string, string) error { return nil },
			postProcess: dropFresh,
		}
		did, _ := syntax.ParseDID(followedDID)
		got, err := cf.fetch(context.Background(), did)
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if len(got) != 1 || got[0] != "keep-me" {
			t.Fatalf("got = %v, want postProcess applied to the freshly crawled rows", got)
		}
	})
}

func TestCachedFetch_NilPostProcess_IsIdentity(t *testing.T) {
	cf := cachedFetch[string]{
		ttl: time.Hour,
		now: func() time.Time { return mustParseTime(t, "2026-07-09T12:00:00Z") },
		getState: func(context.Context, string) (string, error) {
			return "", sql.ErrNoRows
		},
		crawl: func(context.Context, syntax.DID) ([]string, error) {
			return []string{"a", "b"}, nil
		},
		store: func(context.Context, string, []string, string) error { return nil },
	}
	did, _ := syntax.ParseDID(followedDID)

	got, err := cf.fetch(context.Background(), did)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got = %v, want the results unchanged", got)
	}
}

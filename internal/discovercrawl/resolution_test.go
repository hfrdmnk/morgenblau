package discovercrawl

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/leafletfeed"
	"morgenblau/internal/standardfeed"
)

const resolutionCacheSchema = `
CREATE TABLE discover_publication_resolutions (
    publication_uri TEXT PRIMARY KEY,
    canonical_key   TEXT,
    kind            TEXT,
    title           TEXT,
    site_url        TEXT,
    icon_url        TEXT,
    failure_count   INTEGER NOT NULL DEFAULT 0,
    fetched_at      TEXT NOT NULL,
    next_retry_at   TEXT NOT NULL
);
CREATE INDEX discover_publication_resolutions_canonical_key_idx ON discover_publication_resolutions (canonical_key);
`

func openResolutionCacheTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), resolutionCacheSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return dbs
}

// singleHostResolver satisfies standardfeed.Resolver, resolving any authority to the same PDS endpoint.
type singleHostResolver struct {
	endpoint string
}

func (r singleHostResolver) Lookup(_ context.Context, atid syntax.AtIdentifier) (*identity.Identity, error) {
	did, err := syntax.ParseDID(atid.String())
	if err != nil {
		return nil, err
	}
	return &identity.Identity{
		DID: did,
		Services: map[string]identity.ServiceEndpoint{
			"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: r.endpoint},
		},
	}, nil
}

func TestIsRecordNotFound_RealXRPCNotFoundResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"RecordNotFound","message":"could not locate record"}`))
	}))
	defer srv.Close()

	stdClient := standardfeed.NewClient(singleHostResolver{endpoint: srv.URL}, srv.Client())
	_, err := stdClient.GetPublication(context.Background(), leafletPubSiblingURI)
	if err == nil {
		t.Fatal("expected error")
	}
	if !isRecordNotFound(err) {
		t.Errorf("isRecordNotFound(%v) = false, want true", err)
	}
}

func TestIsRecordNotFound_500IsNotRecordNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	stdClient := standardfeed.NewClient(singleHostResolver{endpoint: srv.URL}, srv.Client())
	_, err := stdClient.GetPublication(context.Background(), leafletPubSiblingURI)
	if err == nil {
		t.Fatal("expected error")
	}
	if isRecordNotFound(err) {
		t.Errorf("isRecordNotFound(%v) = true, want false for a 500", err)
	}
}

func TestResolveCached_SuccessThenServedFromDBWithFreshL1(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	got1, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{})
	if !ok {
		t.Fatal("first resolve: expected ok")
	}
	if got1.Key != pubURI {
		t.Errorf("got1 = %+v", got1)
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", row.FailureCount)
	}
	wantNextRetry := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if row.NextRetryAt != wantNextRetry {
		t.Errorf("NextRetryAt = %q, want %q", row.NextRetryAt, wantNextRetry)
	}

	// Fresh L1 map simulates a second, independent Crawl.
	got2, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{})
	if !ok {
		t.Fatal("second resolve: expected ok")
	}
	if got2.Key != pubURI {
		t.Errorf("got2 = %+v", got2)
	}
	if standard.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (served from DB cache)", standard.calls)
	}
}

func TestResolveCached_TransientFailureBacksOff(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{errByURI: map[string]error{pubURI: &atclient.APIError{StatusCode: 500}}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected failure")
	}
	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.CanonicalKey != nil {
		t.Errorf("CanonicalKey = %v, want nil", row.CanonicalKey)
	}
	if row.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", row.FailureCount)
	}
	wantNext := now.Add(time.Hour).UTC().Format(time.RFC3339)
	if row.NextRetryAt != wantNext {
		t.Errorf("NextRetryAt = %q, want %q", row.NextRetryAt, wantNext)
	}

	// Before expiry: no new attempt.
	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected still failing")
	}
	if standard.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (within backoff window)", standard.calls)
	}

	// Past expiry: retries, failure_count increments, delay doubles.
	c.now = func() time.Time { return now.Add(time.Hour).Add(time.Minute) }
	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected still failing")
	}
	if standard.calls != 2 {
		t.Errorf("resolver calls = %d, want 2", standard.calls)
	}
	row, err = db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", row.FailureCount)
	}
	wantNext2 := c.now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	if row.NextRetryAt != wantNext2 {
		t.Errorf("NextRetryAt = %q, want %q", row.NextRetryAt, wantNext2)
	}
}

func TestResolutionBackoff_Table(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{1, time.Hour},
		{2, 2 * time.Hour},
		{9, 168 * time.Hour},
		{64, 168 * time.Hour},
	}
	for _, tc := range cases {
		if got := resolutionBackoff.Delay(tc.failures); got != tc.want {
			t.Errorf("Delay(%d) = %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestResolveCached_StaleWhileErrorServesPriorPayload(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); !ok {
		t.Fatal("seed resolve: expected ok")
	}

	// Expire the row and make the resolver start failing.
	c.now = func() time.Time { return now.Add(25 * time.Hour) }
	standard.byURI = nil
	standard.errByURI = map[string]error{pubURI: &atclient.APIError{StatusCode: 500}}

	got, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{})
	if !ok {
		t.Fatal("expected stale payload served despite the failed refresh")
	}
	if got.Key != pubURI || got.Title != "Zine" {
		t.Errorf("got = %+v, want the stale payload preserved", got)
	}
	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", row.FailureCount)
	}
	if row.CanonicalKey == nil || *row.CanonicalKey != pubURI {
		t.Errorf("CanonicalKey = %v, want preserved %q", row.CanonicalKey, pubURI)
	}
}

func TestResolveCached_DeterministicSkipCachedFor24h(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	standard := &fakePublicationResolver{errByURI: map[string]error{leafletPubSiblingURI: recordNotFoundErr()}}
	leaflet := &fakeLeafletResolver{byURI: map[string]*leafletfeed.Publication{
		leafletPubURI: {URI: leafletPubURI, Name: "Zine", FeedURL: "https://zine.example/rss", ShowInDiscover: false},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard, leaflet: leaflet}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected skip")
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), leafletPubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", row.FailureCount)
	}
	if row.CanonicalKey != nil {
		t.Errorf("CanonicalKey = %v, want nil", row.CanonicalKey)
	}
	wantNext := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if row.NextRetryAt != wantNext {
		t.Errorf("NextRetryAt = %q, want %q", row.NextRetryAt, wantNext)
	}
}

func TestResolvePublication_StandardOptedOutIsDeterministicSkip(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: false},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected skip")
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.CanonicalKey != nil {
		t.Errorf("CanonicalKey = %v, want nil", row.CanonicalKey)
	}
	if row.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", row.FailureCount)
	}
	wantNext := now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if row.NextRetryAt != wantNext {
		t.Errorf("NextRetryAt = %q, want %q", row.NextRetryAt, wantNext)
	}
}

func TestResolveCached_CancelledContextWritesNoRow(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{errByURI: map[string]error{pubURI: context.Canceled}}
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := c.resolvePublication(ctx, pubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected failure")
	}

	_, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected no row written on cancellation, got err = %v", err)
	}
}

func TestResolveCached_UnknownCollectionNeverWritesRow(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	uri := "at://did:plc:x/com.example.zine/1"
	c := (&Client{}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))

	if _, ok := c.resolvePublication(context.Background(), uri, "", map[string]Subscription{}); ok {
		t.Fatal("expected skip")
	}

	_, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), uri)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected no row ever written for an unknown collection, got err = %v", err)
	}
}

func TestResolveCached_StandardSuccessPersistsIconURL(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	iconURL := "https://pds.example/xrpc/com.atproto.sync.getBlob?did=did:plc:pub&cid=bafkreiabc"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", IconURL: iconURL, ShowInDiscover: true},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); !ok {
		t.Fatal("expected ok")
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.IconUrl == nil || *row.IconUrl != iconURL {
		t.Errorf("IconUrl = %v, want %q", row.IconUrl, iconURL)
	}
}

func TestResolveCached_LeafletSiblingSuccessPersistsIconURL(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	iconURL := "https://pds.example/xrpc/com.atproto.sync.getBlob?did=did:plc:pub&cid=bafkreisibling"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		leafletPubSiblingURI: {URI: leafletPubSiblingURI, Name: "Zine", URL: "https://zine.example", IconURL: iconURL, ShowInDiscover: true},
	}}
	leaflet := &fakeLeafletResolver{}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard, leaflet: leaflet}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), leafletPubURI, "", map[string]Subscription{}); !ok {
		t.Fatal("expected ok")
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), leafletPubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.IconUrl == nil || *row.IconUrl != iconURL {
		t.Errorf("IconUrl = %v, want %q", row.IconUrl, iconURL)
	}
}

func TestResolveCached_StaleWhileErrorPreservesIconURL(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	iconURL := "https://pds.example/xrpc/com.atproto.sync.getBlob?did=did:plc:pub&cid=bafkreiabc"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", IconURL: iconURL, ShowInDiscover: true},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); !ok {
		t.Fatal("seed resolve: expected ok")
	}

	// Expire the row and make the resolver start failing.
	c.now = func() time.Time { return now.Add(25 * time.Hour) }
	standard.byURI = nil
	standard.errByURI = map[string]error{pubURI: &atclient.APIError{StatusCode: 500}}

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); !ok {
		t.Fatal("expected stale payload served despite the failed refresh")
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.IconUrl == nil || *row.IconUrl != iconURL {
		t.Errorf("IconUrl = %v, want preserved %q", row.IconUrl, iconURL)
	}
}

func TestResolveCached_DeterministicSkipLeavesIconURLNull(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &fakePublicationResolver{byURI: map[string]*standardfeed.Publication{
		pubURI: {URI: pubURI, Name: "Zine", URL: "https://zine.example", IconURL: "https://pds.example/xrpc/com.atproto.sync.getBlob?did=did:plc:pub&cid=bafkreiabc", ShowInDiscover: false},
	}}
	now := mustParseTime(t, "2026-07-11T12:00:00Z")
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))
	c.now = func() time.Time { return now }

	if _, ok := c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{}); ok {
		t.Fatal("expected skip")
	}

	row, err := db.New(dbs.Reader).GetDiscoverPublicationResolution(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolution: %v", err)
	}
	if row.IconUrl != nil {
		t.Errorf("IconUrl = %v, want nil", row.IconUrl)
	}
}

// TestGetDiscoverPublicationResolutionByCanonicalKey covers the handle-form/DID-form fan-in: two PK rows sharing one canonical_key.
func TestGetDiscoverPublicationResolutionByCanonicalKey(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	ctx := context.Background()
	canonicalKey := "at://did:plc:pub/site.standard.publication/3p"
	now := mustParseTime(t, "2026-07-11T12:00:00Z").UTC().Format(time.RFC3339)
	writer := db.New(dbs.Writer)
	for _, uri := range []string{"at://pub.example/site.standard.publication/3p", canonicalKey} {
		if err := writer.UpsertDiscoverPublicationResolution(ctx, db.UpsertDiscoverPublicationResolutionParams{
			PublicationUri: uri,
			CanonicalKey:   &canonicalKey,
			FailureCount:   0,
			FetchedAt:      now,
			NextRetryAt:    now,
		}); err != nil {
			t.Fatalf("Upsert(%s): %v", uri, err)
		}
	}

	got, err := db.New(dbs.Reader).GetDiscoverPublicationResolutionByCanonicalKey(ctx, &canonicalKey)
	if err != nil {
		t.Fatalf("GetDiscoverPublicationResolutionByCanonicalKey: %v", err)
	}
	if got.CanonicalKey == nil || *got.CanonicalKey != canonicalKey {
		t.Errorf("CanonicalKey = %v, want %q", got.CanonicalKey, canonicalKey)
	}

	miss := "at://did:plc:missing/site.standard.publication/9z"
	if _, err := db.New(dbs.Reader).GetDiscoverPublicationResolutionByCanonicalKey(ctx, &miss); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows on miss, got %v", err)
	}
}

// blockingPublicationResolver sleeps briefly so concurrent resolves of the same uri pile up behind singleflight.
type blockingPublicationResolver struct {
	calls int32
	pub   *standardfeed.Publication
}

func (f *blockingPublicationResolver) GetPublication(_ context.Context, _ string) (*standardfeed.Publication, error) {
	atomic.AddInt32(&f.calls, 1)
	time.Sleep(50 * time.Millisecond)
	return f.pub, nil
}

func TestResolveCached_SingleflightCollapsesConcurrentResolves(t *testing.T) {
	dbs := openResolutionCacheTestDB(t)
	pubURI := "at://did:plc:pub/site.standard.publication/3p"
	standard := &blockingPublicationResolver{pub: &standardfeed.Publication{URI: pubURI, Name: "Zine", URL: "https://zine.example", ShowInDiscover: true}}
	c := (&Client{standard: standard}).WithResolutionCache(db.New(dbs.Reader), db.New(dbs.Writer))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.resolvePublication(context.Background(), pubURI, "", map[string]Subscription{})
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&standard.calls); got != 1 {
		t.Errorf("resolver calls = %d, want 1 (singleflight collapsed)", got)
	}
}

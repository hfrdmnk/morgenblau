package tapingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/discoverbatch"
)

// The seeder is what discoverbatch.Runner ticks in production; a signature drift here would only surface in internal/server.
var _ discoverbatch.Runnable = (*Seeder)(nil)

const seederSchema = `
CREATE TABLE tap_seeder_state (
    did       TEXT PRIMARY KEY,
    seeded_at TEXT NOT NULL
);
`

// seedDIDs mints n distinct placeholder repo DIDs. The seeder never parses them, so the suffix only has to be unique.
func seedDIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("did:plc:seed%016d", i)
	}
	return out
}

func fakeSeedRelay(t *testing.T, dids []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repos := make([]map[string]any, 0, len(dids))
		for _, d := range dids {
			repos = append(repos, map[string]any{"did": d})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"repos": repos}); err != nil {
			t.Errorf("encode relay response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeTap records every /repos/add body and can fail a chosen request, standing in for the sidecar.
type fakeTap struct {
	mu      sync.Mutex
	posts   [][]string
	methods []string
	paths   []string
	types   []string
	failAt  int
	srv     *httptest.Server
}

func newFakeTap(t *testing.T) *fakeTap {
	t.Helper()
	f := &fakeTap{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DIDs []string `json:"dids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode /repos/add body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.posts = append(f.posts, body.DIDs)
		f.methods = append(f.methods, r.Method)
		f.paths = append(f.paths, r.URL.Path)
		f.types = append(f.types, r.Header.Get("Content-Type"))
		n, failAt := len(f.posts), f.failAt
		f.mu.Unlock()
		if failAt == n {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTap) setFailAt(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAt = n
}

func (f *fakeTap) chunkSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.posts))
	for i, p := range f.posts {
		out[i] = len(p)
	}
	return out
}

func (f *fakeTap) post(i int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts[i]
}

// fakeSeedStore is both the reader and the writer half of the seeded-DID state.
type fakeSeedStore struct {
	mu        sync.Mutex
	seeded    []string
	listErr   error
	insertErr error
}

func (f *fakeSeedStore) ListTapSeededDids(ctx context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]string(nil), f.seeded...), nil
}

func (f *fakeSeedStore) InsertTapSeededDid(ctx context.Context, arg db.InsertTapSeededDidParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.seeded = append(f.seeded, arg.Did)
	return nil
}

func (f *fakeSeedStore) marked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seeded...)
}

func withFakeSeedTx(s *Seeder, store *fakeSeedStore) *Seeder {
	s.runTx = func(ctx context.Context, fn func(SeedWriter) error) error { return fn(store) }
	return s
}

func newTestSeeder(t *testing.T, relayDIDs []string, tap *fakeTap, store *fakeSeedStore) *Seeder {
	t.Helper()
	relay := fakeSeedRelay(t, relayDIDs)
	s := NewSeeder(tap.srv.URL, tap.srv.Client(), relay.URL, relay.Client(), store)
	// One collection keeps the fake relay's bookkeeping honest; the real list is exercised in discoverbatch.
	s.collections = []string{"collection-a"}
	return withFakeSeedTx(s, store)
}

func TestSeeder_PostsNewDidsToTapInChunksOf500(t *testing.T) {
	dids := seedDIDs(1200)
	tap := newFakeTap(t)
	store := &fakeSeedStore{}
	s := newTestSeeder(t, dids, tap, store)

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 1200 {
		t.Errorf("Run returned %d, want 1200 newly registered repos", n)
	}
	if got, want := tap.chunkSizes(), []int{500, 500, 200}; !equalInts(got, want) {
		t.Fatalf("chunk sizes = %v, want %v", got, want)
	}
	if got := tap.post(0)[0]; got != dids[0] {
		t.Errorf("first chunk starts at %q, want %q (enumeration order must survive chunking)", got, dids[0])
	}
	if got := tap.post(2)[199]; got != dids[1199] {
		t.Errorf("last chunk ends at %q, want %q", got, dids[1199])
	}
	if got := len(store.marked()); got != 1200 {
		t.Errorf("marked %d dids, want 1200", got)
	}
}

func TestSeeder_PostsToReposAddAsJSON(t *testing.T) {
	tap := newFakeTap(t)
	store := &fakeSeedStore{}
	s := newTestSeeder(t, seedDIDs(1), tap, store)

	if _, err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tap.mu.Lock()
	defer tap.mu.Unlock()
	if len(tap.methods) != 1 {
		t.Fatalf("requests = %d, want 1", len(tap.methods))
	}
	if tap.methods[0] != http.MethodPost {
		t.Errorf("method = %q, want POST", tap.methods[0])
	}
	if tap.paths[0] != "/repos/add" {
		t.Errorf("path = %q, want /repos/add", tap.paths[0])
	}
	if tap.types[0] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", tap.types[0])
	}
}

func TestSeeder_SkipsDidsAlreadySeeded(t *testing.T) {
	dids := seedDIDs(3)
	tap := newFakeTap(t)
	store := &fakeSeedStore{seeded: []string{dids[0], dids[1]}}
	s := newTestSeeder(t, dids, tap, store)

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 1 {
		t.Errorf("Run returned %d, want 1 (only the unseeded repo is new)", n)
	}
	if got, want := tap.chunkSizes(), []int{1}; !equalInts(got, want) {
		t.Fatalf("chunk sizes = %v, want %v", got, want)
	}
	if got := tap.post(0)[0]; got != dids[2] {
		t.Errorf("posted %q, want the only unseeded did %q", got, dids[2])
	}
}

func TestSeeder_NothingNewPostsNothing(t *testing.T) {
	dids := seedDIDs(2)
	tap := newFakeTap(t)
	store := &fakeSeedStore{seeded: dids}
	s := newTestSeeder(t, dids, tap, store)

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 0 {
		t.Errorf("Run returned %d, want 0", n)
	}
	if got := tap.chunkSizes(); len(got) != 0 {
		t.Fatalf("posted %v, want no requests when tap already tracks every repo", got)
	}
}

func TestSeeder_MarksSeededOnlyAfterATapSuccess(t *testing.T) {
	tap := newFakeTap(t)
	tap.setFailAt(1)
	store := &fakeSeedStore{}
	s := newTestSeeder(t, seedDIDs(3), tap, store)

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v, want nil (a tap outage degrades the run, it does not fail it)", err)
	}
	if n != 0 {
		t.Errorf("Run returned %d, want 0 registered repos", n)
	}
	if got := store.marked(); len(got) != 0 {
		t.Fatalf("marked %v, want nothing: a non-2xx must never record a repo as seeded", got)
	}
}

func TestSeeder_FailedChunkAbandonsTheRestOfTheRun(t *testing.T) {
	dids := seedDIDs(1200)
	tap := newFakeTap(t)
	tap.setFailAt(2)
	store := &fakeSeedStore{}
	s := newTestSeeder(t, dids, tap, store)

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v, want nil", err)
	}
	if n != 500 {
		t.Errorf("Run returned %d, want 500 (only the first chunk landed)", n)
	}
	if got, want := tap.chunkSizes(), []int{500, 500}; !equalInts(got, want) {
		t.Fatalf("chunk sizes = %v, want %v: the third chunk must not be attempted", got, want)
	}
	if got := len(store.marked()); got != 500 {
		t.Errorf("marked %d dids, want 500", got)
	}
}

func TestSeeder_RerunAfterAFailedChunkRepostsOnlyUnmarkedDids(t *testing.T) {
	dids := seedDIDs(1200)
	tap := newFakeTap(t)
	tap.setFailAt(2)
	store := &fakeSeedStore{}
	s := newTestSeeder(t, dids, tap, store)

	if _, err := s.Run(context.Background()); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	tap.setFailAt(0)
	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if n != 700 {
		t.Errorf("second Run returned %d, want 700", n)
	}
	if got, want := tap.chunkSizes(), []int{500, 500, 500, 200}; !equalInts(got, want) {
		t.Fatalf("chunk sizes = %v, want %v", got, want)
	}
	if got := tap.post(2)[0]; got != dids[500] {
		t.Errorf("rerun resumed at %q, want %q: marked repos must not be re-posted", got, dids[500])
	}
	if got := len(store.marked()); got != 1200 {
		t.Errorf("marked %d dids, want 1200 after the rerun", got)
	}
}

func TestSeeder_RelayFailurePropagatesAndRegistersNothing(t *testing.T) {
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer relay.Close()

	tap := newFakeTap(t)
	store := &fakeSeedStore{}
	s := NewSeeder(tap.srv.URL, tap.srv.Client(), relay.URL, relay.Client(), store)
	s.collections = []string{"collection-a"}
	withFakeSeedTx(s, store)

	if _, err := s.Run(context.Background()); err == nil {
		t.Fatal("expected the relay failure to propagate so the run is not stamped as successful")
	}
	if got := tap.chunkSizes(); len(got) != 0 {
		t.Errorf("posted %v, want nothing when enumeration failed", got)
	}
}

func TestSeeder_SeededStateReadFailurePropagates(t *testing.T) {
	tap := newFakeTap(t)
	store := &fakeSeedStore{listErr: fmt.Errorf("db unavailable")}
	s := newTestSeeder(t, seedDIDs(2), tap, store)

	if _, err := s.Run(context.Background()); err == nil {
		t.Fatal("expected an unreadable seeded-state to abort the run rather than re-post the whole network")
	}
	if got := tap.chunkSizes(); len(got) != 0 {
		t.Errorf("posted %v, want nothing", got)
	}
}

func TestSeeder_MarkFailureStopsTheRun(t *testing.T) {
	tap := newFakeTap(t)
	store := &fakeSeedStore{insertErr: fmt.Errorf("disk full")}
	s := newTestSeeder(t, seedDIDs(1200), tap, store)

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v, want nil", err)
	}
	if n != 0 {
		t.Errorf("Run returned %d, want 0", n)
	}
	if got, want := tap.chunkSizes(), []int{500}; !equalInts(got, want) {
		t.Fatalf("chunk sizes = %v, want %v: a mark failure must stop the run", got, want)
	}
}

func TestNewSeeder_NormalizesBareRelayHostAndDerivesTheAddURL(t *testing.T) {
	s := NewSeeder("http://localhost:2480/", nil, "relay.example", nil, nil)
	if s.relayEndpoint != "https://relay.example" {
		t.Errorf("relayEndpoint = %q, want https://relay.example", s.relayEndpoint)
	}
	if s.addURL != "http://localhost:2480/repos/add" {
		t.Errorf("addURL = %q, want http://localhost:2480/repos/add", s.addURL)
	}
	if len(s.collections) != len(discoverbatch.EnumerationCollections)+len(discoverbatch.FollowEnumerationCollections) {
		t.Errorf("collections = %v, want both enumeration lists", s.collections)
	}
}

func TestSeeder_WithTxRunner_MarksSeededInRealSQLite(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	dbs, err := database.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { dbs.Close() })
	if _, err := dbs.Writer.ExecContext(context.Background(), seederSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}

	dids := seedDIDs(3)
	tap := newFakeTap(t)
	relay := fakeSeedRelay(t, dids)
	s := NewSeeder(tap.srv.URL, tap.srv.Client(), relay.URL, relay.Client(), db.New(dbs.Reader)).WithTxRunner(dbs.Writer)
	s.collections = []string{"collection-a"}

	if _, err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stored, err := db.New(dbs.Reader).ListTapSeededDids(context.Background())
	if err != nil {
		t.Fatalf("ListTapSeededDids: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("stored = %v, want 3 rows", stored)
	}

	n, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if n != 0 {
		t.Errorf("second Run returned %d, want 0: persisted marks must survive into the next run", n)
	}
	if got := tap.chunkSizes(); len(got) != 1 {
		t.Errorf("requests = %v, want only the first run's chunk", got)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

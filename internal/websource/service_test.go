package websource

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"morgenblau/internal/database"
)

const (
	testBase = "https://app.example.com"
	testDID  = "did:plc:reader"
)

var testNow = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

type pollCall struct {
	recipe json.RawMessage
	prev   PollState
}

type fakeGenerator struct {
	mu          sync.Mutex
	generated   Generated
	setupErr    error
	setups      []string
	anchors     []string
	allowModel  []bool
	setupCtxs   []context.Context
	outcomes    []PollOutcome
	pollErr     error
	polls       []pollCall
	beforeSetup func(ctx context.Context)
	duringPoll  func()
}

func (f *fakeGenerator) Setup(ctx context.Context, listingURL, anchorURL string, allowModel bool) (Generated, error) {
	if f.beforeSetup != nil {
		f.beforeSetup(ctx)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setups = append(f.setups, listingURL)
	f.anchors = append(f.anchors, anchorURL)
	f.allowModel = append(f.allowModel, allowModel)
	f.setupCtxs = append(f.setupCtxs, ctx)
	return f.generated, f.setupErr
}

func (f *fakeGenerator) setupCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.setups)
}

func (f *fakeGenerator) Poll(_ context.Context, recipe json.RawMessage, prev PollState) (PollOutcome, error) {
	if f.duringPoll != nil {
		f.duringPoll()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls = append(f.polls, pollCall{recipe: recipe, prev: prev})
	if len(f.outcomes) == 0 {
		if f.pollErr != nil {
			return PollOutcome{}, f.pollErr
		}
		return PollOutcome{Verdict: VerdictOK}, nil
	}
	out := f.outcomes[0]
	if len(f.outcomes) > 1 {
		f.outcomes = f.outcomes[1:]
	}
	return out, f.pollErr
}

type testEnv struct {
	service *Service
	gen     *fakeGenerator
	reader  *sql.DB
	writer  *sql.DB
	now     time.Time
}

func (e *testEnv) advance(d time.Duration) { e.now = e.now.Add(d) }

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "websource.db")
	applyMigrations(t, path)
	dsn := "file:" + path + "?_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)"
	writer, err := sql.Open("sqlite", dsn+"&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	writer.SetMaxOpenConns(1)
	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close(); writer.Close() })
	env := &testEnv{gen: &fakeGenerator{generated: exampleGenerated()}, reader: reader, writer: writer, now: testNow}
	env.service = NewService(reader, writer, env.gen, Config{PublicBaseURL: testBase, PollInterval: 30 * time.Minute, ModelDailyUSD: 1})
	env.service.now = func() time.Time { return env.now }
	return env
}

func applyMigrations(t *testing.T, databasePath string) {
	t.Helper()
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("goose CLI is required for web source storage tests; install github.com/pressly/goose/v3/cmd/goose@v3.27.1")
	}
	migrationDir, err := filepath.Abs(filepath.Join("..", "database", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(goosePath, "-dir", migrationDir, "sqlite3", databasePath, "up").CombinedOutput(); err != nil {
		t.Fatalf("goose migration: %v\n%s", err, output)
	}
}

func day(d int) time.Time { return time.Date(2026, 10, d, 8, 0, 0, 0, time.UTC) }

func exampleGenerated() Generated {
	return Generated{
		Recipe:  json.RawMessage(`{"version":1,"mode":"streams"}`),
		Title:   "Example Publication",
		SiteURL: "https://news.example.com/",
		Health:  HealthOK,
		Items: []Item{
			{URL: "https://news.example.com/posts/third", Title: "Third post", Date: day(8), Excerpt: "The third one."},
			{URL: "https://news.example.com/posts/second", Title: "Second post", Excerpt: "No date on this one."},
			{URL: "https://news.example.com/posts/first", Title: "First post", Date: day(6)},
			{URL: "https://news.example.com/posts/third", Title: "Third post again"},
		},
		Fingerprint: Fingerprint{URLTemplate: "news.example.com/posts/*", DateFrac: 0.67, TitleShape: 1},
		Cost:        Cost{Stages: []string{"heuristic"}},
	}
}

type storedItem struct {
	url, title, excerpt, firstSeen string
	published                      *string
	history                        bool
}

func storedItems(t *testing.T, env *testEnv, id string) map[string]storedItem {
	t.Helper()
	rows, err := env.reader.Query(`SELECT url, title, excerpt, first_seen_at, published_at, history FROM web_source_items WHERE source_id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]storedItem{}
	for rows.Next() {
		var it storedItem
		if err := rows.Scan(&it.url, &it.title, &it.excerpt, &it.firstSeen, &it.published, &it.history); err != nil {
			t.Fatal(err)
		}
		out[it.url] = it
	}
	return out
}

func TestCreateStoresSetupItemsAsHistory(t *testing.T) {
	env := newTestEnv(t)
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-z2-7]{26}$`).MatchString(preview.Source.ID) {
		t.Errorf("id = %q, want 128 random bits as lowercase base32", preview.Source.ID)
	}
	if want := testBase + "/feed/" + preview.Source.ID; preview.Source.FeedURL != want {
		t.Errorf("feed URL = %q, want %q", preview.Source.FeedURL, want)
	}
	if preview.Source.Title != "Example Publication" || preview.Source.SiteURL != "https://news.example.com/" || preview.Source.Health != HealthOK {
		t.Errorf("source = %+v", preview.Source)
	}
	if preview.ItemCount != 3 {
		t.Errorf("item count = %d, want 3 after deduping by URL", preview.ItemCount)
	}
	var urls []string
	for _, it := range preview.Items {
		urls = append(urls, it.URL)
	}
	want := []string{"https://news.example.com/posts/third", "https://news.example.com/posts/first", "https://news.example.com/posts/second"}
	if !slices.Equal(urls, want) {
		t.Errorf("preview order = %v, want dated newest first, then undated history", urls)
	}

	items := storedItems(t, env, preview.Source.ID)
	for url, it := range items {
		if !it.history {
			t.Errorf("%s: setup item not stored as history", url)
		}
		if it.firstSeen != "2026-10-10T09:00:00Z" {
			t.Errorf("%s: first seen = %q", url, it.firstSeen)
		}
	}
	if got := items["https://news.example.com/posts/second"].published; got != nil {
		t.Errorf("undated item stored with date %q", *got)
	}
	if got := items["https://news.example.com/posts/third"]; got.published == nil || *got.published != "2026-10-08T08:00:00Z" || got.title != "Third post" {
		t.Errorf("dated item = %+v", got)
	}

	var did, cost string
	var nextPoll *string
	if err := env.reader.QueryRow(`SELECT created_by_did, setup_cost, next_poll_at FROM web_sources WHERE id = ?`, preview.Source.ID).Scan(&did, &cost, &nextPoll); err != nil {
		t.Fatal(err)
	}
	if did != testDID || cost != `{"stages":["heuristic"],"judgeCalls":0,"inputTokens":0,"outputTokens":0,"usd":0}` {
		t.Errorf("did = %q, cost = %s", did, cost)
	}
	if nextPoll == nil || *nextPoll != "2026-10-10T09:30:00Z" {
		t.Errorf("next poll = %v, want one interval after setup", nextPoll)
	}
	var polls int
	if err := env.reader.QueryRow(`SELECT COUNT(*) FROM web_source_polls WHERE source_id = ?`, preview.Source.ID).Scan(&polls); err != nil {
		t.Fatal(err)
	}
	if polls != 1 {
		t.Errorf("good polls = %d, want the setup recorded as the first", polls)
	}
}

func TestCreateCapsExcerpts(t *testing.T) {
	env := newTestEnv(t)
	long := ""
	for range 100 {
		long += "word "
	}
	env.gen.generated.Items = []Item{{URL: "https://news.example.com/posts/long", Title: "Long", Excerpt: long}}
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	got := []rune(storedItems(t, env, preview.Source.ID)["https://news.example.com/posts/long"].excerpt)
	if len(got) > maxExcerptRunes+1 || got[len(got)-1] != '…' {
		t.Errorf("excerpt = %q, want at most %d runes ending in an ellipsis", string(got), maxExcerptRunes)
	}
}

func TestCreateReusesTheSourceForTheSameSite(t *testing.T) {
	env := newTestEnv(t)
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/blog/", "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := env.service.Create(context.Background(), "did:plc:other", "HTTPS://WWW.News.Example.com/blog#latest", "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Source.ID != first.Source.ID {
		t.Errorf("second create made %q, want the existing %q", again.Source.ID, first.Source.ID)
	}
	if len(env.gen.setups) != 1 {
		t.Errorf("setups = %d, want reuse without regenerating", len(env.gen.setups))
	}
	if again.ItemCount != 3 || len(again.Items) != 3 {
		t.Errorf("reused preview = %+v", again)
	}
}

func TestCreateKeepsAReusedSourceFromTheSweepWhileTheReaderDecides(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	first, err := env.service.Create(ctx, testDID, "https://news.example.com/blog/", "")
	if err != nil {
		t.Fatal(err)
	}
	env.advance(database.FeedSweepGrace + 24*time.Hour)
	if _, err := env.service.Create(ctx, "did:plc:other", "https://news.example.com/blog/", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SweepUnfollowedFeeds(ctx, env.writer, env.now); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.read.GetWebSource(ctx, first.Source.ID); err != nil {
		t.Fatalf("the source a reader was just offered was swept before they could follow it: %v", err)
	}
}

func TestCreateNeverHandsOutASourceTheSweepDeletedDuringARegeneration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.gen.generated.Health = HealthNeedsAttention
	if _, err := env.service.Create(ctx, testDID, "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	env.advance(database.FeedSweepGrace + 24*time.Hour)
	env.gen.setupErr = errors.New("the site timed out")
	env.gen.beforeSetup = func(context.Context) {
		if _, err := database.SweepUnfollowedFeeds(ctx, env.writer, env.now); err != nil {
			t.Error(err)
		}
	}
	p, err := env.service.Create(ctx, "did:plc:other", "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if env.gen.setupCount() != 2 {
		t.Fatalf("setups = %d, want the stale unsure source regenerated", env.gen.setupCount())
	}
	if _, err := env.service.read.GetWebSource(ctx, p.Source.ID); err != nil {
		t.Fatalf("the preview names source %s, which the sweep deleted while it regenerated: %v", p.Source.ID, err)
	}
}

func TestCreateWithAnchorRerunsSetupAndMergesItems(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Health = HealthNeedsAttention
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Source.NeedsAnchor {
		t.Fatal("first setup should ask for an anchor")
	}

	env.advance(time.Hour)
	env.gen.generated = Generated{
		Recipe: json.RawMessage(`{"version":1,"mode":"css"}`), Title: "Example Publication", SiteURL: "https://news.example.com/", Health: HealthOK,
		Items:       []Item{{URL: "https://news.example.com/stories/a", Title: "Story A"}},
		Fingerprint: Fingerprint{URLTemplate: "news.example.com/stories/*"},
	}
	anchored, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/stories/a")
	if err != nil {
		t.Fatal(err)
	}
	if anchored.Source.ID != first.Source.ID {
		t.Errorf("anchored setup made a new source %q, want %q", anchored.Source.ID, first.Source.ID)
	}
	if got := env.gen.anchors; !slices.Equal(got, []string{"", "https://news.example.com/stories/a"}) {
		t.Errorf("anchors passed to setup = %v", got)
	}
	if anchored.Source.Health != HealthOK || anchored.Source.NeedsAnchor || anchored.ItemCount != 4 {
		t.Errorf("anchored preview = %+v", anchored)
	}
	items := storedItems(t, env, first.Source.ID)
	if len(items) != 4 || !items["https://news.example.com/stories/a"].history || !items["https://news.example.com/posts/third"].history {
		t.Errorf("items = %+v, want the anchored setup's history joined to the items readers already saw", items)
	}
	if got := pollCount(t, env, first.Source.ID); got != 1 {
		t.Errorf("good polls = %d, want only the anchored setup's", got)
	}
	var recipe string
	if err := env.reader.QueryRow(`SELECT recipe FROM web_sources WHERE id = ?`, first.Source.ID).Scan(&recipe); err != nil {
		t.Fatal(err)
	}
	if recipe != `{"version":1,"mode":"css"}` {
		t.Errorf("recipe = %s", recipe)
	}
}

func TestCreateRejectsUnsafeURLs(t *testing.T) {
	for _, tc := range []struct{ name, listing, anchor string }{
		{"empty", "", ""},
		{"ftp", "ftp://news.example.com/", ""},
		{"credentials", "https://user:secret@news.example.com/", ""},
		{"no host", "https:///posts", ""},
		{"relative", "/posts", ""},
		{"bad anchor scheme", "https://news.example.com/", "javascript:alert(1)"},
		{"anchor with credentials", "https://news.example.com/", "https://user@news.example.com/a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			if _, err := env.service.Create(context.Background(), testDID, tc.listing, tc.anchor); !errors.Is(err, ErrInvalidURL) {
				t.Errorf("err = %v, want ErrInvalidURL", err)
			}
			if len(env.gen.setups) != 0 {
				t.Error("generator ran for an invalid URL")
			}
		})
	}
}

func TestCreateReportsGenerationFailureAndStoresNothing(t *testing.T) {
	env := newTestEnv(t)
	env.gen.setupErr = errors.New("connection refused")
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); !errors.Is(err, ErrGeneration) {
		t.Fatalf("err = %v, want ErrGeneration", err)
	}
	var n int
	if err := env.reader.QueryRow(`SELECT COUNT(*) FROM web_sources`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("web sources = %d, want none after a failed setup", n)
	}
}

func TestCreateStoresASiteThatBlocksUs(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated = Generated{Recipe: json.RawMessage(`{}`), SiteURL: "https://news.example.com/", Health: HealthBlockedBySite, HealthReason: "403 on the listing"}
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source.Health != HealthBlockedBySite || preview.Source.HealthReason != "403 on the listing" || preview.ItemCount != 0 || preview.Items == nil {
		t.Errorf("preview = %+v", preview)
	}
	if preview.Source.Title != "news.example.com" {
		t.Errorf("title = %q, want the host when the page has none", preview.Source.Title)
	}
}

func TestCreateWithoutGeneratorIsUnavailable(t *testing.T) {
	env := newTestEnv(t)
	env.service.gen = nil
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestListingKeyNormalizesTheSite(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://news.example.com/", "news.example.com"},
		{"https://WWW.News.Example.com/blog/#top", "news.example.com/blog"},
		{"http://news.example.com/blog", "news.example.com/blog"},
		{"http://news.example.com/blog?page=1", "news.example.com/blog?page=1"},
		{"https://news.example.com/blog?utm_source=x&ref=y&UTM_Medium=z", "news.example.com/blog"},
		{"https://news.example.com/blog?ref=y&section=world", "news.example.com/blog?section=world"},
		{"https://news.example.com:8443/blog//", "news.example.com:8443/blog"},
	} {
		u, err := parseHTTPURL(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if got := listingKey(u); got != tc.want {
			t.Errorf("listingKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCreateRegeneratesAStaleUnhealthySourceOnReuse(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Health = HealthNeedsAttention
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	env.advance(5 * time.Minute)
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	if len(env.gen.setups) != 1 {
		t.Fatalf("setups = %d, want a recent verdict reused", len(env.gen.setups))
	}

	env.advance(6 * time.Minute)
	env.gen.generated.Health = HealthOK
	again, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(env.gen.setups) != 2 || again.Source.ID != first.Source.ID || again.Source.Health != HealthOK {
		t.Errorf("setups = %d, preview = %+v; want the stale unhealthy source regenerated in place", len(env.gen.setups), again.Source)
	}

	env.advance(time.Hour)
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	if len(env.gen.setups) != 2 {
		t.Errorf("setups = %d, want a healthy source reused however old", len(env.gen.setups))
	}
}

type fakeLimiter struct {
	limit int
	used  map[string]int
}

func (l *fakeLimiter) Allow(key string) (time.Duration, bool) {
	if l.used[key] >= l.limit {
		return 42 * time.Minute, false
	}
	l.used[key]++
	return 0, true
}

func TestCreateRateLimitsGenerationsPerReader(t *testing.T) {
	env := newTestEnv(t)
	env.service.limiter = &fakeLimiter{limit: 2, used: map[string]int{}}
	ctx := context.Background()
	for _, site := range []string{"https://a.example.com/", "https://b.example.com/"} {
		if _, err := env.service.Create(ctx, testDID, site, ""); err != nil {
			t.Fatal(err)
		}
	}
	_, err := env.service.Create(ctx, testDID, "https://c.example.com/", "")
	var limited *RateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter != 42*time.Minute {
		t.Fatalf("third generation err = %v, want a RateLimitError with the limiter's wait", err)
	}
	if len(env.gen.setups) != 2 {
		t.Errorf("setups = %d, want the limited request to generate nothing", len(env.gen.setups))
	}
	if _, err := env.service.Create(ctx, testDID, "https://a.example.com/", ""); err != nil {
		t.Errorf("reusing a healthy source counted against the limit: %v", err)
	}
	if _, err := env.service.Create(ctx, "did:plc:other", "https://c.example.com/", ""); err != nil {
		t.Errorf("another reader was limited: %v", err)
	}
}

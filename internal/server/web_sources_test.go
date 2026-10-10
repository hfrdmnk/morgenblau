package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/mmcdole/gofeed"

	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/session"
	"morgenblau/internal/websource"
)

const webSourceRSS = `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Publication</title><link>https://news.example.com/</link><description>d</description>` +
	`<item><title>First post</title><link>https://news.example.com/posts/first</link><guid>https://news.example.com/posts/first</guid></item></channel></rss>`

func TestPublicBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"explicit", map[string]string{"PUBLIC_BASE_URL": "https://feeds.example.com/", "APP_ENV": "local"}, "https://feeds.example.com", false},
		{"explicit keeps a port", map[string]string{"PUBLIC_BASE_URL": "http://127.0.0.1:9000"}, "http://127.0.0.1:9000", false},
		{"explicit with a path", map[string]string{"PUBLIC_BASE_URL": "https://feeds.example.com/app"}, "", true},
		{"explicit with credentials", map[string]string{"PUBLIC_BASE_URL": "https://user@feeds.example.com"}, "", true},
		{"explicit not http", map[string]string{"PUBLIC_BASE_URL": "ftp://feeds.example.com"}, "", true},
		{"production uses the OAuth client origin", map[string]string{"APP_ENV": "production", "BLUESKY_CLIENT_ID": "https://app.example.com/oauth-client-metadata.json"}, "https://app.example.com", false},
		{"production without a client id", map[string]string{"APP_ENV": "production"}, "", true},
		{"local uses the loopback port", map[string]string{"APP_ENV": "local"}, "http://127.0.0.1:8123", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := publicBaseURL(func(k string) string { return tc.env[k] }, 8123)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("publicBaseURL = %q, %v; want %q, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestOwnWebFeedsAreFetchedInProcess(t *testing.T) {
	var hits atomic.Int32
	feeds := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/web/abc" {
			t.Errorf("in-process path = %q", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = io.WriteString(w, webSourceRSS)
	})
	// Nothing listens on port 1, so only the in-process path can answer.
	f := fetcher.New(fetcher.WrapTransport(func(next http.RoundTripper) http.RoundTripper {
		return ownWebFeeds("http://127.0.0.1:1", feeds, next)
	}))

	res, err := f.Fetch(context.Background(), "http://127.0.0.1:1/web/abc", fetcher.FeedState{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Feed == nil || len(res.Feed.Items) != 1 || res.ETag != `"v1"` {
		t.Fatalf("result = %+v", res)
	}
	res, err = f.Fetch(context.Background(), "http://127.0.0.1:1/web/abc", fetcher.FeedState{ETag: `"v1"`})
	if err != nil || !res.NotModified {
		t.Fatalf("conditional fetch = %+v, %v; want 304", res, err)
	}
	if hits.Load() != 2 {
		t.Errorf("handler hits = %d, want 2", hits.Load())
	}
}

func TestEverythingElseStillGoesThroughTheSafeTransport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, webSourceRSS)
	}))
	defer upstream.Close()
	var hits atomic.Int32
	feeds := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, webSourceRSS)
	})
	f := fetcher.New(fetcher.WrapTransport(func(next http.RoundTripper) http.RoundTripper {
		return ownWebFeeds(upstream.URL, feeds, next)
	}))

	for _, raw := range []string{
		upstream.URL + "/feed.xml",
		upstream.URL + "/api/health",
		upstream.URL + "/webfeed",
		strings.Replace(upstream.URL, "http://", "https://", 1) + "/web/abc",
	} {
		if _, err := f.Fetch(context.Background(), raw, fetcher.FeedState{}); !errors.Is(err, safehttp.ErrBlockedAddress) {
			t.Errorf("%s: err = %v, want the safe transport's loopback block", raw, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("in-process handler served %d requests that are not this origin's /web/ feeds", hits.Load())
	}
	if _, err := f.Fetch(context.Background(), upstream.URL+"/web/abc", fetcher.FeedState{}); err != nil || hits.Load() != 1 {
		t.Errorf("own /web/ feed: err = %v, hits = %d", err, hits.Load())
	}
}

func TestWebSourceRoutesAreRegistered(t *testing.T) {
	dbs := openMigratedRouteDB(t)
	svc := websource.NewService(dbs.Reader, dbs.Writer, nil, websource.Config{PublicBaseURL: "https://app.example.com", PollInterval: time.Minute})
	if _, err := dbs.Writer.Exec(`INSERT INTO web_sources (id, listing_key, listing_url, site_url, title, recipe, health, created_by_did, created_at, updated_at, setup_at, setup_health, setup_tried_at)
		VALUES ('abc', 'news.example.com', 'https://news.example.com/', 'https://news.example.com/', 'Example Publication', '{}', 'ok', 'did:plc:reader', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z', 'ok', '2026-10-10T09:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := dbs.Writer.Exec(`INSERT INTO web_source_items (source_id, url, title, first_seen_at, history, position)
		VALUES ('abc', 'https://news.example.com/posts/first', 'First post', '2026-10-10T09:00:00Z', 1, 0)`); err != nil {
		t.Fatal(err)
	}
	srv := &Server{webSources: svc}

	rr := httptest.NewRecorder()
	srv.RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/web/abc", nil))
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/rss+xml; charset=utf-8" {
		t.Fatalf("anonymous GET /web/abc = %d %q: %s", rr.Code, rr.Header().Get("Content-Type"), rr.Body)
	}
	if feed, err := gofeed.NewParser().ParseString(rr.Body.String()); err != nil || len(feed.Items) != 1 {
		t.Errorf("feed = %v, %v", feed, err)
	}
	rr = httptest.NewRecorder()
	srv.RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/web/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("unknown id = %d, want 404", rr.Code)
	}

	did, _ := syntax.ParseDID("did:plc:reader")
	req := httptest.NewRequest(http.MethodPost, "/api/web-sources", strings.NewReader(`{"url":"https://other.example.com/"}`))
	req = req.WithContext(auth.WithSession(req.Context(), &session.Session{Data: &session.Data{AccountDID: did, SessionID: "sid-1"}}))
	rr = httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), `"code":"unavailable"`) {
		t.Errorf("POST /api/web-sources without a generator = %d: %s", rr.Code, rr.Body)
	}
}

type countingPoller struct {
	calls atomic.Int32
	err   error
}

func (p *countingPoller) PollDue(context.Context) (int, error) {
	p.calls.Add(1)
	return 0, p.err
}

func TestRunWebPollPollsOnEveryTickUntilCancelled(t *testing.T) {
	p := &countingPoller{err: errors.New("one source failed")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { runWebPoll(ctx, p, time.Millisecond); close(done) }()
	deadline := time.After(time.Second)
	for p.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("polled %d times in a second", p.calls.Load())
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runWebPoll did not return after cancel")
	}
}

func TestIsOwnWebFeed(t *testing.T) {
	own := isOwnWebFeed("https://app.example.com")
	for raw, want := range map[string]bool{
		"https://app.example.com/web/abc":      true,
		"https://APP.example.com/web/abc":      true,
		"http://app.example.com/web/abc":       false,
		"https://app.example.com/webfeed":      false,
		"https://app.example.com/api/health":   false,
		"https://other.example.com/web/abc":    false,
		"https://app.example.com:8443/web/abc": false,
		"::not a url":                          false,
	} {
		if got := own(raw); got != want {
			t.Errorf("isOwnWebFeed(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestSourceListMarksOwnGeneratedFeeds(t *testing.T) {
	dbs := openMigratedRouteDB(t)
	svc := websource.NewService(dbs.Reader, dbs.Writer, nil, websource.Config{PublicBaseURL: "https://app.example.com", PollInterval: time.Minute})
	for _, stmt := range []string{
		`INSERT INTO web_sources (id, listing_key, listing_url, site_url, title, recipe, health, health_reason, created_by_did, created_at, updated_at, setup_at, setup_health, setup_tried_at)
		VALUES ('abc', 'news.example.com', 'https://news.example.com/', 'https://news.example.com/', 'Example Publication', '{}', 'needs_attention', 'Morgenblau isn''t sure which links are posts', 'did:plc:reader', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z', 'needs_attention', '2026-10-10T09:00:00Z')`,
		`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES ('https://app.example.com/web/abc', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z')`,
		`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, created_at, updated_at)
		VALUES ('did:plc:reader', '3gen', 'at://did:plc:reader/blue.morgen.feed.subscription/3gen', 'https://app.example.com/web/abc', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z')`,
	} {
		if _, err := dbs.Writer.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	srv := &Server{db: dbs, qr: dbqueries.New(dbs.Reader), webSources: svc}

	did, _ := syntax.ParseDID("did:plc:reader")
	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &session.Session{Data: &session.Data{AccountDID: did, SessionID: "sid-1"}}))
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	want := `"generated":{"health":"needs_attention","healthReason":"Morgenblau isn't sure which links are posts"}`
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), want) {
		t.Errorf("GET /api/subscriptions = %d %s, want it to contain %s", rr.Code, rr.Body, want)
	}
}

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/mmcdole/gofeed"

	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/session"
	"morgenblau/internal/websource"
)

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

	// /web/ is the legacy alias: subscriptions made before /feed/ still name it.
	for _, path := range []string{"/feed/", "/web/"} {
		rr := httptest.NewRecorder()
		srv.RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path+"abc", nil))
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/rss+xml; charset=utf-8" {
			t.Fatalf("anonymous GET %sabc = %d %q: %s", path, rr.Code, rr.Header().Get("Content-Type"), rr.Body)
		}
		if feed, err := gofeed.NewParser().ParseString(rr.Body.String()); err != nil || len(feed.Items) != 1 {
			t.Errorf("%sabc feed = %v, %v", path, feed, err)
		}
		rr = httptest.NewRecorder()
		srv.RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path+"missing", nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("%smissing = %d, want 404", path, rr.Code)
		}
	}

	did, _ := syntax.ParseDID("did:plc:reader")
	req := httptest.NewRequest(http.MethodPost, "/api/web-sources", strings.NewReader(`{"url":"https://other.example.com/"}`))
	req = req.WithContext(auth.WithSession(req.Context(), &session.Session{Data: &session.Data{AccountDID: did, SessionID: "sid-1"}}))
	rr := httptest.NewRecorder()
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

func TestSourceListMarksOwnGeneratedFeeds(t *testing.T) {
	dbs := openMigratedRouteDB(t)
	svc := websource.NewService(dbs.Reader, dbs.Writer, nil, websource.Config{PublicBaseURL: "https://app.example.com", PollInterval: time.Minute})
	for _, stmt := range []string{
		`INSERT INTO web_sources (id, listing_key, listing_url, site_url, title, recipe, health, health_reason, created_by_did, created_at, updated_at, setup_at, setup_health, setup_tried_at)
		VALUES ('abc', 'news.example.com', 'https://news.example.com/', 'https://news.example.com/', 'Example Publication', '{}', 'needs_attention', 'Morgenblau isn''t sure which links are posts', 'did:plc:reader', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z', 'needs_attention', '2026-10-10T09:00:00Z')`,
		`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES ('https://app.example.com/feed/abc', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z'), ('https://app.example.com/web/abc', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z')`,
		`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, created_at, updated_at)
		VALUES ('did:plc:reader', '3gen', 'at://did:plc:reader/blue.morgen.feed.subscription/3gen', 'https://app.example.com/feed/abc', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z'),
		('did:plc:reader', '3old', 'at://did:plc:reader/blue.morgen.feed.subscription/3old', 'https://app.example.com/web/abc', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z')`,
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
	if rr.Code != http.StatusOK || strings.Count(rr.Body.String(), want) != 2 {
		t.Errorf("GET /api/subscriptions = %d %s, want %s on both the /feed/ and the legacy /web/ subscription", rr.Code, rr.Body, want)
	}
}

func TestAddingAYouTubeChannelFindsItsLongFormSubscription(t *testing.T) {
	for name, webSources := range map[string]*websource.Service{
		"without generated feeds": nil,
		"with generated feeds":    websource.NewService(nil, nil, nil, websource.Config{PublicBaseURL: "https://app.example.com"}),
	} {
		t.Run(name, func(t *testing.T) {
			dbs := openMigratedRouteDB(t)
			for _, stmt := range []string{
				`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES ('https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z')`,
				`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, created_at, updated_at)
				VALUES ('did:plc:reader', '3long', 'at://did:plc:reader/blue.morgen.feed.subscription/3long', 'https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv', '2026-10-10T09:00:00Z', '2026-10-10T09:00:00Z')`,
			} {
				if _, err := dbs.Writer.Exec(stmt); err != nil {
					t.Fatal(err)
				}
			}
			srv := &Server{db: dbs, qr: dbqueries.New(dbs.Reader), webSources: webSources}

			did, _ := syntax.ParseDID("did:plc:reader")
			req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", strings.NewReader(`{"feedUrl":"https://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv"}`))
			req = req.WithContext(auth.WithSession(req.Context(), &session.Session{Data: &session.Data{AccountDID: did, SessionID: "sid-1"}}))
			rr := httptest.NewRecorder()
			srv.routes().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"rkey":"3long"`) {
				t.Errorf("POST /api/subscriptions = %d %s, want the existing long-form subscription 3long", rr.Code, rr.Body)
			}
		})
	}
}

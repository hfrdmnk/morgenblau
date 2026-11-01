package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"

	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/session"
)

func TestPublicFollowingIsAnonymousAfterTheReaderOptsIn(t *testing.T) {
	dbs := openMigratedRouteDB(t)
	for _, stmt := range []string{
		`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES ('https://blog.example.com/feed.xml', '2026-10-31T08:00:00Z', '2026-10-31T08:00:00Z')`,
		`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, title, created_at, updated_at)
		VALUES ('did:plc:reader', '3sub', 'at://did:plc:reader/blue.morgen.feed.subscription/3sub', 'https://blog.example.com/feed.xml', 'Example Blog', '2026-10-31T08:00:00Z', '2026-10-31T08:00:00Z')`,
	} {
		if _, err := dbs.Writer.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	srv := &Server{db: dbs, qr: dbqueries.New(dbs.Reader), qw: dbqueries.New(dbs.Writer)}
	anonymous := func(path string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		srv.RegisterRoutes().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		return rr
	}
	for _, path := range []string{"/user/did:plc:reader/following.json", "/user/did:plc:reader/following.opml"} {
		if rr := anonymous(path); rr.Code != http.StatusNotFound {
			t.Fatalf("GET %s before opting in = %d %s, want 404", path, rr.Code, rr.Body)
		}
	}

	did, _ := syntax.ParseDID("did:plc:reader")
	req := httptest.NewRequest(http.MethodPut, "/api/public-following", strings.NewReader(`{"enabled":true}`))
	req = req.WithContext(auth.WithSession(req.Context(), &session.Session{Data: &session.Data{AccountDID: did, SessionID: "sid-1"}}))
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/public-following = %d %s", rr.Code, rr.Body)
	}

	if rr := anonymous("/user/did:plc:reader/following.json"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"feedUrl":"https://blog.example.com/feed.xml"`) || rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("GET following.json = %d %q %s", rr.Code, rr.Header().Get("Access-Control-Allow-Origin"), rr.Body)
	}
	if rr := anonymous("/user/did:plc:reader/following.opml"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `xmlUrl="https://blog.example.com/feed.xml"`) {
		t.Errorf("GET following.opml = %d %s", rr.Code, rr.Body)
	}
	if rr := anonymous("/user/did:plc:stranger/following.json"); rr.Code != http.StatusNotFound {
		t.Errorf("another DID = %d, want 404", rr.Code)
	}
}

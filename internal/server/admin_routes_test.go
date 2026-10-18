package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/session"
)

const adminDID, readerDID = "did:plc:admin", "did:plc:reader"

type anySessionApp struct{}

func (anySessionApp) ResumeSession(_ context.Context, did syntax.DID, sid string) (*oauth.ClientSession, error) {
	return &oauth.ClientSession{Data: &oauth.ClientSessionData{AccountDID: did, SessionID: sid}}, nil
}

func (anySessionApp) Logout(context.Context, syntax.DID, string) error { return nil }

// adminRoutes reads every admin pattern from routes.go, plus the SPA page the catch-all serves, so a new admin route is checked the moment it is wired.
func adminRoutes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "routes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		pattern, _ := strconv.Unquote(lit.Value)
		if _, path, _ := strings.Cut(pattern, " "); strings.Contains(pattern, " ") && auth.IsAdminPath(path) || auth.IsAdminPath(pattern) {
			routes = append(routes, pattern)
		}
		return true
	})
	if len(routes) == 0 {
		t.Fatal("found no admin routes in routes.go; restore route discovery so every admin route is checked")
	}
	return append(routes, "GET /admin")
}

func adminRouteRequest(pattern string) *http.Request {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		method, path = http.MethodGet, pattern
	}
	for strings.Contains(path, "{") {
		start, end := strings.Index(path, "{"), strings.Index(path, "}")
		path = path[:start] + "x" + path[end+1:]
	}
	return httptest.NewRequest(method, path, nil)
}

func newAdminRouteServer(t *testing.T) (*Server, *cookie.Sealer) {
	t.Helper()
	t.Setenv("APP_ENV", "")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	sealer, err := cookie.New(key)
	if err != nil {
		t.Fatal(err)
	}
	dbs := openMigratedRouteDB(t)
	return &Server{
		db: dbs, qr: dbqueries.New(dbs.Reader), qw: dbqueries.New(dbs.Writer),
		sessions: session.NewManager(anySessionApp{}, nil, nil),
		sealer:   sealer,
		profiles: profiles.New(readerIdentity{}, nil),
		admins:   auth.Admins{adminDID: {}},
	}, sealer
}

func TestEveryAdminRouteIsHiddenFromEveryoneButAdmins(t *testing.T) {
	srv, sealer := newAdminRouteServer(t)
	handler := srv.RegisterRoutes()
	for _, pattern := range adminRoutes(t) {
		isAPI := strings.Contains(pattern, "/api/")
		for _, tc := range []struct {
			did  string
			want func(int) bool
			why  string
		}{
			{"", func(code int) bool {
				return code == http.StatusUnauthorized && isAPI || code == http.StatusFound && !isAPI
			}, "a signed-out request must be sent to sign in"},
			{readerDID, func(code int) bool { return code == http.StatusNotFound }, "a reader who is not in ADMIN_DIDS must see no admin route"},
			{adminDID, func(code int) bool { return code == http.StatusOK }, "a listed admin must reach the route"},
		} {
			req := adminRouteRequest(pattern)
			if tc.did != "" {
				set := httptest.NewRecorder()
				sealer.Set(set, tc.did, "sid")
				req.AddCookie(set.Result().Cookies()[0])
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if !tc.want(rr.Code) {
				t.Errorf("%s as %q: status = %d; %s", pattern, tc.did, rr.Code, tc.why)
			}
		}
	}
}

func TestEveryAdminRouteRefusesRequestsTheGateDidNotAdmit(t *testing.T) {
	srv, _ := newAdminRouteServer(t)
	mux := srv.routes()
	// Even a listed admin's session is refused without the mark only the gate sets.
	admin := &session.Session{Data: &session.Data{AccountDID: adminDID, SessionID: "sid"}}
	for _, pattern := range adminRoutes(t) {
		if !strings.Contains(pattern, "/api/") {
			continue
		}
		req := adminRouteRequest(pattern)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req.WithContext(auth.WithSession(req.Context(), admin)))
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s without the gate's admin mark: status = %d, want 404; every admin handler must refuse on its own when mounted outside the gate", pattern, rr.Code)
		}
	}
}

func TestAdminUsageRouteCountsTheRealIndex(t *testing.T) {
	srv, sealer := newAdminRouteServer(t)
	const now = "2026-10-14T08:00:00Z"
	for _, stmt := range []string{
		`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES ('https://blog.example.com/feed.xml', '` + now + `', '` + now + `')`,
		`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, created_at, updated_at) VALUES ('` + readerDID + `', 'r1', 'at://` + readerDID + `/blue.morgen.feed.subscription/r1', 'https://blog.example.com/feed.xml', '` + now + `', '` + now + `')`,
		`INSERT INTO oauth_sessions (did, session_id, data, updated_at) VALUES ('` + readerDID + `', 'sid', x'00', '` + now + `')`,
	} {
		if _, err := srv.db.Writer.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage", nil)
	set := httptest.NewRecorder()
	sealer.Set(set, adminDID, "sid")
	req.AddCookie(set.Result().Cookies()[0])
	rr := httptest.NewRecorder()
	srv.RegisterRoutes().ServeHTTP(rr, req)
	var got struct {
		Totals struct{ People, Regular int }
		People []struct {
			DID, Handle, LastActiveAt string
		}
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("status = %d, err = %v, body = %s", rr.Code, err, rr.Body.String())
	}
	if got.Totals.People != 1 || got.Totals.Regular != 1 || len(got.People) != 1 ||
		got.People[0].DID != readerDID || got.People[0].Handle != "reader.example" || got.People[0].LastActiveAt != now {
		t.Fatalf("usage = %+v", got)
	}
}

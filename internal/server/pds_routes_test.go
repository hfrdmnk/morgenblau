package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database"
	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
	"morgenblau/internal/session"
)

const (
	routeDID         = "did:plc:reader"
	routeSID         = "sid-1"
	routePublication = "at://did:plc:publisher/site.standard.publication/3pub"
	subCollection    = "blue.morgen.feed.subscription"
	stdCollection    = "site.standard.graph.subscription"
	saveCollection   = "blue.morgen.feed.save"
)

var errPDSDown = errors.New("pds down")

// pdsRoute drives one PDS-reaching route on the real mux. maxCommits is the law's budget: one for a mutation, one per source for OPML import.
type pdsRoute struct {
	name       string
	pattern    string
	method     string
	path       string
	body       string
	seed       func(t *testing.T, env *routeEnv)
	wantStatus int
	maxCommits int
	mirrors    bool
	repairs    int
	pdsAfter   func(t *testing.T, pds *memoryPDS)
}

var pdsRoutes = []pdsRoute{
	{
		name: "add feed", pattern: "POST /api/subscriptions", method: http.MethodPost, path: "/api/subscriptions",
		body:       `{"feedUrl":"https://feeds.example.com/a.xml","title":"Example Feed","siteUrl":"https://feeds.example.com"}`,
		wantStatus: http.StatusOK, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "add customized publication", pattern: "POST /api/subscriptions", method: http.MethodPost, path: "/api/subscriptions",
		body:       `{"publication":"` + routePublication + `","title":"Example Publication","tags":["news"]}`,
		wantStatus: http.StatusOK, maxCommits: 1, mirrors: true, repairs: 1,
		pdsAfter: func(t *testing.T, pds *memoryPDS) {
			if len(pds.records[stdCollection]) != 1 || len(pds.records[subCollection]) != 1 {
				t.Errorf("PDS = %v, want the existence record and its sidecar", pds.records)
			}
		},
	},
	{
		name: "add feed for a site followed as a publication", pattern: "POST /api/subscriptions", method: http.MethodPost, path: "/api/subscriptions",
		body: `{"feedUrl":"https://feeds.example.com/c.xml","siteUrl":"https://feeds.example.com/"}`, seed: seedPublicationOnFeedsSite,
		wantStatus: http.StatusConflict,
	},
	{
		name: "add publication for a site followed as a feed", pattern: "POST /api/subscriptions", method: http.MethodPost, path: "/api/subscriptions",
		body: `{"publication":"at://did:plc:publisher/site.standard.publication/3other","siteUrl":"https://feeds.example.com"}`, seed: seedFeedSubscription,
		wantStatus: http.StatusConflict,
	},
	{
		name: "edit feed", pattern: "PATCH /api/subscriptions/{rkey}", method: http.MethodPatch, path: "/api/subscriptions/3feed",
		body: `{"title":"Renamed","tags":["later"]}`, seed: seedFeedSubscription,
		wantStatus: http.StatusOK, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "repoint feed", pattern: "PATCH /api/subscriptions/{rkey}", method: http.MethodPatch, path: "/api/subscriptions/3feed",
		body: `{"feedUrl":"https://feeds.example.com/b.xml"}`, seed: seedFeedSubscription,
		wantStatus: http.StatusOK, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "edit publication", pattern: "PATCH /api/subscriptions/{rkey}", method: http.MethodPatch, path: "/api/subscriptions/3std",
		body: `{"title":"Renamed"}`, seed: seedPublicationSubscription,
		wantStatus: http.StatusOK, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "remove feed", pattern: "DELETE /api/subscriptions/{rkey}", method: http.MethodDelete, path: "/api/subscriptions/3feed",
		seed:       seedFeedSubscription,
		wantStatus: http.StatusNoContent, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "remove publication with duplicate and sidecar", pattern: "DELETE /api/subscriptions/{rkey}", method: http.MethodDelete, path: "/api/subscriptions/3std",
		seed:       seedPublicationSubscription,
		wantStatus: http.StatusNoContent, maxCommits: 1, mirrors: true, repairs: 1,
		pdsAfter: func(t *testing.T, pds *memoryPDS) {
			if len(pds.records[stdCollection]) != 0 || len(pds.records[subCollection]) != 0 {
				t.Errorf("PDS = %v, want the existence records and sidecar gone", pds.records)
			}
		},
	},
	{
		name: "save", pattern: "POST /api/saves", method: http.MethodPost, path: "/api/saves",
		body:       `{"itemUrl":"https://posts.example.com/1"}`,
		wantStatus: http.StatusCreated, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "unsave", pattern: "DELETE /api/saves/{rkey}", method: http.MethodDelete, path: "/api/saves/3save",
		seed:       seedSave,
		wantStatus: http.StatusNoContent, maxCommits: 1, mirrors: true, repairs: 1,
	},
	{
		name: "import", pattern: "POST /api/subscriptions/import", method: http.MethodPost, path: "/api/subscriptions/import",
		body:       `{"sources":[{"feedUrl":"https://feeds.example.com/c.xml","title":"C"},{"feedUrl":"https://feeds.example.com/d.xml","title":"D"}]}`,
		wantStatus: http.StatusOK, maxCommits: 2, mirrors: true, repairs: 2,
	},
	{
		name: "prepare import", pattern: "POST /api/subscriptions/import/prepare", method: http.MethodPost, path: "/api/subscriptions/import/prepare",
		body: `{"provider":"glean"}`, wantStatus: http.StatusOK,
	},
	{
		name: "export", pattern: "POST /api/subscriptions/export", method: http.MethodPost, path: "/api/subscriptions/export",
		seed: seedFeedSubscription, wantStatus: http.StatusOK,
	},
}

// Every route handed the reader's PDS must be driven below, so a new write path cannot skip the law 1 checks.
func TestEveryPDSRouteIsDrivenByTheRouteChecks(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "routes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wired := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 || !isSelector(call.Fun, "mux", "Handle") {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || !reachesPDS(call.Args[1]) {
			return true
		}
		pattern, _ := strconv.Unquote(lit.Value)
		wired[pattern] = true
		return true
	})
	driven := map[string]bool{}
	for _, route := range pdsRoutes {
		driven[route.pattern] = true
	}
	for pattern := range wired {
		if !driven[pattern] {
			t.Errorf("%s reaches the PDS but pdsRoutes never drives it; law 1's route checks cover only routes in that table", pattern)
		}
	}
	for pattern := range driven {
		if !wired[pattern] {
			t.Errorf("pdsRoutes drives %s, which routes.go no longer hands the PDS; drop the stale row so the table matches the real routes", pattern)
		}
	}
	if len(wired) == 0 {
		t.Fatal("found no PDS routes in routes.go")
	}
}

func TestPDSRoutesCommitAtMostOnceThenMirror(t *testing.T) {
	for _, route := range pdsRoutes {
		t.Run(route.name, func(t *testing.T) {
			env := newRouteEnv(t, route)
			before := env.snapshot(t)
			rr := env.serve(route)

			if rr.Code != route.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rr.Code, route.wantStatus, rr.Body.String())
			}
			if got := env.pds.attempts(); got > route.maxCommits {
				t.Errorf("PDS commits = %d, want at most %d; one commit per request keeps a reported failure meaning nothing committed (law 1)", got, route.maxCommits)
			}
			if changed := env.snapshot(t) != before; changed != route.mirrors {
				t.Errorf("SQLite changed = %v, want %v", changed, route.mirrors)
			}
			if got := env.sync.repairs(); got != 0 {
				t.Errorf("repair dispatches = %d, want 0 when the mirror succeeds", got)
			}
			if route.pdsAfter != nil {
				route.pdsAfter(t, env.pds)
			}
		})
	}
}

func TestPDSRoutesLeaveSQLiteUntouchedWhenTheCommitFails(t *testing.T) {
	for _, route := range pdsRoutes {
		t.Run(route.name, func(t *testing.T) {
			env := newRouteEnv(t, route)
			env.pds.failCommits = true
			before := env.snapshot(t)
			rr := env.serve(route)

			if after := env.snapshot(t); after != before {
				t.Errorf("SQLite changed after a failed commit; the index may only mirror a committed PDS write (law 1):\nbefore %s\nafter  %s", before, after)
			}
			if env.pds.attempts() > 0 && route.maxCommits == 1 && rr.Code < 400 {
				t.Errorf("status = %d after the commit failed, want a reported failure", rr.Code)
			}
			if route.maxCommits > 1 {
				var result struct {
					Failures []json.RawMessage `json:"failures"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil || len(result.Failures) != env.pds.attempts() {
					t.Errorf("failures = %s, want one per failed source commit (%d)", rr.Body.String(), env.pds.attempts())
				}
			}
			if got := env.sync.repairs(); got != 0 {
				t.Errorf("repair dispatches = %d, want 0 when nothing committed", got)
			}
		})
	}
}

func TestPDSRoutesReportACommittedWriteWhenTheMirrorFails(t *testing.T) {
	for _, route := range pdsRoutes {
		if !route.mirrors {
			continue
		}
		t.Run(route.name, func(t *testing.T) {
			env := newRouteEnv(t, route)
			env.failMirrors(t)
			rr := env.serve(route)

			if rr.Code != route.wantStatus {
				t.Fatalf("status = %d, want %d; a committed PDS write must be reported as committed. body = %s", rr.Code, route.wantStatus, rr.Body.String())
			}
			if env.pds.attempts() == 0 {
				t.Fatal("no PDS commit reached the PDS")
			}
			if got := env.sync.repairs(); got != route.repairs {
				t.Errorf("repair dispatches = %d, want %d", got, route.repairs)
			}
		})
	}
}

func TestAuthenticatedEntryStartsReconciliation(t *testing.T) {
	env := newRouteEnv(t, pdsRoute{})
	env.server.profiles = profiles.New(readerIdentity{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/profiles/me", nil)
	rr := httptest.NewRecorder()
	env.server.routes().ServeHTTP(rr, req.WithContext(auth.WithSession(req.Context(), env.session)))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	if got := env.sync.logins(); !slices.Equal(got, []string{routeDID + ":" + routeSID}) {
		t.Fatalf("login reconciliations = %v, want one for %s:%s", got, routeDID, routeSID)
	}
}

type readerIdentity struct{}

func (readerIdentity) LookupDID(_ context.Context, did syntax.DID) (*identity.Identity, error) {
	return &identity.Identity{DID: did, Handle: syntax.Handle("reader.example")}, nil
}

type routeEnv struct {
	server  *Server
	db      *database.DB
	pds     *memoryPDS
	sync    *countingSync
	session *session.Session
}

func newRouteEnv(t *testing.T, route pdsRoute) *routeEnv {
	t.Helper()
	dbs := openMigratedRouteDB(t)
	did, _ := syntax.ParseDID(routeDID)
	env := &routeEnv{
		db:      dbs,
		pds:     newMemoryPDS(),
		sync:    &countingSync{},
		session: &session.Session{Data: &session.Data{AccountDID: did, SessionID: routeSID, Scopes: []string{scopes.StandardSubscription}}},
	}
	env.server = &Server{db: dbs, qr: dbqueries.New(dbs.Reader), qw: dbqueries.New(dbs.Writer), pds: env.pds, sync: env.sync}
	if route.seed != nil {
		route.seed(t, env)
		env.pds.resetAttempts()
	}
	return env
}

func (env *routeEnv) serve(route pdsRoute) *httptest.ResponseRecorder {
	req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
	rr := httptest.NewRecorder()
	env.server.routes().ServeHTTP(rr, req.WithContext(auth.WithSession(req.Context(), env.session)))
	return rr
}

// snapshot renders every row the law 1 mirrors write, so any local change shows up as a different string.
func (env *routeEnv) snapshot(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	for _, table := range []string{"feeds", "user_subscriptions", "user_saves"} {
		rows, err := env.db.Reader.Query("SELECT * FROM " + table + " ORDER BY 1, 2")
		if err != nil {
			t.Fatal(err)
		}
		columns, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(columns))
			ptrs := make([]any, len(columns))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "%s%v;", table, values)
		}
		rows.Close()
	}
	return out.String()
}

func (env *routeEnv) failMirrors(t *testing.T) {
	t.Helper()
	for _, table := range []string{"feeds", "user_subscriptions", "user_saves"} {
		for _, op := range []string{"INSERT", "UPDATE", "DELETE"} {
			stmt := fmt.Sprintf("CREATE TRIGGER fail_%s_%s BEFORE %s ON %s BEGIN SELECT RAISE(ABORT, 'mirror down'); END", table, strings.ToLower(op), op, table)
			if _, err := env.db.Writer.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func (env *routeEnv) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := env.db.Writer.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

const seededAt = "2026-09-01T10:00:00Z"

func seedFeedSubscription(t *testing.T, env *routeEnv) {
	const feed = "https://feeds.example.com/a.xml"
	env.pds.put(subCollection, "3feed", map[string]any{
		"source":    map[string]any{"$type": "blue.morgen.feed.subscription#rssFeed", "feedUrl": feed},
		"title":     "Example Feed",
		"createdAt": seededAt,
	})
	env.exec(t, `INSERT INTO feeds (feed_url, kind, created_at, updated_at) VALUES (?, 'rss', ?, ?)`, feed, seededAt, seededAt)
	env.exec(t, `INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, kind, title, created_at, updated_at) VALUES (?, '3feed', ?, ?, 'rss', 'Example Feed', ?, ?)`,
		routeDID, "at://"+routeDID+"/"+subCollection+"/3feed", feed, seededAt, seededAt)
}

func seedPublicationSubscription(t *testing.T, env *routeEnv) {
	env.pds.put(stdCollection, "3std", map[string]any{"publication": routePublication, "createdAt": seededAt})
	env.pds.put(stdCollection, "3dup", map[string]any{"publication": routePublication, "createdAt": seededAt})
	env.pds.put(subCollection, "3side", map[string]any{
		"source":    map[string]any{"$type": "blue.morgen.feed.subscription#standardPublication", "publication": routePublication},
		"title":     "Example Publication",
		"createdAt": seededAt,
	})
	env.exec(t, `INSERT INTO feeds (feed_url, kind, created_at, updated_at) VALUES (?, 'standardfeed', ?, ?)`, routePublication, seededAt, seededAt)
	env.exec(t, `INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, kind, sidecar_rkey, title, created_at, updated_at) VALUES (?, '3std', ?, ?, 'standardfeed', '3side', 'Example Publication', ?, ?)`,
		routeDID, "at://"+routeDID+"/"+stdCollection+"/3std", routePublication, seededAt, seededAt)
}

func seedPublicationOnFeedsSite(t *testing.T, env *routeEnv) {
	seedPublicationSubscription(t, env)
	env.exec(t, `UPDATE feeds SET site_url = 'https://feeds.example.com' WHERE feed_url = ?`, routePublication)
}

func seedSave(t *testing.T, env *routeEnv) {
	env.pds.put(saveCollection, "3save", map[string]any{"itemUrl": "https://posts.example.com/1", "createdAt": seededAt})
	env.exec(t, `INSERT INTO user_saves (did, rkey, at_uri, item_url, created_at, updated_at) VALUES (?, '3save', ?, 'https://posts.example.com/1', ?, ?)`,
		routeDID, "at://"+routeDID+"/"+saveCollection+"/3save", seededAt, seededAt)
}

func openMigratedRouteDB(t *testing.T) *database.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.db")
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("goose CLI is required for route tests; install github.com/pressly/goose/v3/cmd/goose@v3.27.1")
	}
	if output, err := exec.Command(goosePath, "-dir", filepath.Join("..", "database", "migrations"), "sqlite3", path, "up").CombinedOutput(); err != nil {
		t.Fatalf("goose migration: %v\n%s", err, output)
	}
	t.Setenv("DB_PATH", path)
	dbs, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbs.Close() })
	return dbs
}

func isSelector(expr ast.Expr, x, sel string) bool {
	s, ok := expr.(*ast.SelectorExpr)
	if !ok || s.Sel.Name != sel {
		return false
	}
	ident, ok := s.X.(*ast.Ident)
	return ok && ident.Name == x
}

func reachesPDS(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			found = found || n.Name == "gated"
		case *ast.SelectorExpr:
			found = found || isSelector(n, "s", "pds")
		}
		return !found
	})
	return found
}

// countingSync records every dispatch so the route checks can count repairs exactly.
type countingSync struct {
	mu       sync.Mutex
	manual   int
	loginFor []string
}

func (c *countingSync) StartManualRefresh(_ context.Context, _ syntax.DID, _ string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manual++
	return "sync-repair", nil
}

func (c *countingSync) StartLoginRefresh(_ context.Context, did syntax.DID, sessionID string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loginFor = append(c.loginFor, did.String()+":"+sessionID)
	return "sync-login", nil
}

func (c *countingSync) StartFetchOneFeed(syntax.DID, string) string { return "fetch-1" }

func (c *countingSync) repairs() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manual
}

func (c *countingSync) logins() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.loginFor)
}

// memoryPDS is one reader's repo with atomic applyWrites, counting every commit that reaches it.
type memoryPDS struct {
	mu          sync.Mutex
	records     map[string]map[string]map[string]any
	rev         int
	seq         int
	commits     int
	failCommits bool
}

func newMemoryPDS() *memoryPDS {
	return &memoryPDS{records: map[string]map[string]map[string]any{}}
}

func (p *memoryPDS) put(collection, rkey string, record map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.records[collection] == nil {
		p.records[collection] = map[string]map[string]any{}
	}
	p.records[collection][rkey] = record
	p.rev++
}

func (p *memoryPDS) attempts() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commits
}

func (p *memoryPDS) resetAttempts() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commits = 0
}

func (p *memoryPDS) begin() error {
	p.commits++
	if p.failCommits {
		return errPDSDown
	}
	p.rev++
	return nil
}

func (p *memoryPDS) ref(collection, rkey string) *atprepo.RecordRef {
	return &atprepo.RecordRef{URI: "at://" + routeDID + "/" + collection + "/" + rkey, CID: p.cid(rkey)}
}

func (p *memoryPDS) cid(rkey string) string { return "bafy" + rkey + strconv.Itoa(p.rev) }

func (p *memoryPDS) set(collection, rkey string, record map[string]any) {
	if p.records[collection] == nil {
		p.records[collection] = map[string]map[string]any{}
	}
	p.records[collection][rkey] = record
}

func (p *memoryPDS) nextRkey() string {
	p.seq++
	return "3mem" + strconv.Itoa(p.seq)
}

func (p *memoryPDS) CreateRecord(_ context.Context, _ *session.Session, collection syntax.NSID, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.begin(); err != nil {
		return nil, err
	}
	rkey := p.nextRkey()
	p.set(collection.String(), rkey, record)
	return p.ref(collection.String(), rkey), nil
}

func (p *memoryPDS) PutRecord(_ context.Context, _ *session.Session, collection syntax.NSID, rkey string, record map[string]any) (*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.begin(); err != nil {
		return nil, err
	}
	p.set(collection.String(), rkey, record)
	return p.ref(collection.String(), rkey), nil
}

func (p *memoryPDS) DeleteRecord(_ context.Context, _ *session.Session, collection syntax.NSID, rkey string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.begin(); err != nil {
		return err
	}
	delete(p.records[collection.String()], rkey)
	return nil
}

func (p *memoryPDS) ApplyWrites(_ context.Context, _ *session.Session, writes []atprepo.RecordWrite) ([]*atprepo.RecordRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.begin(); err != nil {
		return nil, err
	}
	for _, write := range writes {
		if _, exists := p.records[write.Collection.String()][write.Rkey.String()]; write.Delete && !exists {
			return nil, fmt.Errorf("could not find record %s/%s", write.Collection, write.Rkey)
		}
	}
	refs := make([]*atprepo.RecordRef, 0, len(writes))
	for _, write := range writes {
		if write.Delete {
			delete(p.records[write.Collection.String()], write.Rkey.String())
		} else {
			p.set(write.Collection.String(), write.Rkey.String(), write.Record)
		}
		refs = append(refs, p.ref(write.Collection.String(), write.Rkey.String()))
	}
	return refs, nil
}

func (p *memoryPDS) ListRecords(_ context.Context, _ *session.Session, collection syntax.NSID) ([]atprepo.ListedRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	records := p.records[collection.String()]
	out := make([]atprepo.ListedRecord, 0, len(records))
	for _, rkey := range slices.Sorted(maps.Keys(records)) {
		out = append(out, atprepo.ListedRecord{URI: p.ref(collection.String(), rkey).URI, CID: p.cid(rkey), Value: records[rkey]})
	}
	return out, nil
}

func (p *memoryPDS) GetRecord(_ context.Context, _ *session.Session, collection syntax.NSID, rkey syntax.RecordKey) (*atprepo.ListedRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	record, ok := p.records[collection.String()][rkey.String()]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &atprepo.ListedRecord{URI: p.ref(collection.String(), rkey.String()).URI, CID: p.cid(rkey.String()), Value: record}, nil
}

func (p *memoryPDS) GetLatestCommit(context.Context, *session.Session) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return "rev" + strconv.Itoa(p.rev), nil
}

func (p *memoryPDS) CreateRecordIfCommit(ctx context.Context, sess *session.Session, collection syntax.NSID, record map[string]any, _ string) (*atprepo.RecordRef, error) {
	return p.CreateRecord(ctx, sess, collection, record)
}

func (p *memoryPDS) PutRecordIfCID(ctx context.Context, sess *session.Session, collection syntax.NSID, rkey string, record map[string]any, _ string) (*atprepo.RecordRef, error) {
	return p.PutRecord(ctx, sess, collection, rkey, record)
}

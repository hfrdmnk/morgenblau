package auth

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func TestLoadAdmins(t *testing.T) {
	for _, tc := range []struct {
		name, dids string
		wantError  bool
		want       []syntax.DID
	}{
		{name: "unset admits nobody"},
		{name: "whitespace admits nobody", dids: "  "},
		{name: "multiple trimmed DIDs", dids: " did:plc:admin , did:web:operator.example.com ", want: []syntax.DID{"did:plc:admin", "did:web:operator.example.com"}},
		{name: "handle instead of DID", dids: "operator.example", wantError: true},
		{name: "invalid second entry", dids: "did:plc:admin,operator.example", wantError: true},
		{name: "trailing comma", dids: "did:plc:admin,", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			admins, err := LoadAdmins(func(key string) string {
				if key == "ADMIN_DIDS" {
					return tc.dids
				}
				return ""
			})
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tc.wantError)
			}
			if err != nil {
				return
			}
			for _, did := range tc.want {
				if !admins.Includes(did) {
					t.Errorf("Includes(%q) = false, want listed admin admitted", did)
				}
			}
			for _, did := range []syntax.DID{"did:plc:reader", "did:plc:adminmore", "did:web:other.example.com"} {
				if admins.Includes(did) {
					t.Errorf("Includes(%q) = true, want unlisted account refused", did)
				}
			}
		})
	}
}

func TestGateAdmitsOnlyAdminsToAdminPaths(t *testing.T) {
	const admin, reader = "did:plc:admin", "did:plc:reader"
	for _, tc := range []struct {
		path, did  string
		wantStatus int
		wantAdmin  bool
	}{
		{path: "/api/admin/usage", wantStatus: 401},
		{path: "/admin", wantStatus: 302},
		{path: "/api/admin/usage", did: reader, wantStatus: 404},
		{path: "/api/admin", did: reader, wantStatus: 404},
		{path: "/admin", did: reader, wantStatus: 404},
		{path: "/admin/", did: reader, wantStatus: 404},
		{path: "/api//admin/usage", did: reader, wantStatus: 404},
		{path: "/api/digest/../admin/usage", did: reader, wantStatus: 404},
		{path: "/api/admin/usage", did: admin, wantStatus: 200, wantAdmin: true},
		{path: "/admin", did: admin, wantStatus: 200, wantAdmin: true},
		{path: "/api/digest", did: admin, wantStatus: 200},
		{path: "/administrator", did: reader, wantStatus: 200},
	} {
		t.Run(tc.path+" "+tc.did, func(t *testing.T) {
			sealer := newSealer(t)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.URL.Path = tc.path
			if tc.did != "" {
				set := httptest.NewRecorder()
				sealer.Set(set, tc.did, "sid")
				req.AddCookie(set.Result().Cookies()[0])
			}
			next := &passthroughNext{}
			rr := httptest.NewRecorder()
			New(&alphaResumer{}, noopLocker{}, sealer, nil, Admins{admin: {}})(next).ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if next.hit != (tc.wantStatus == 200) {
				t.Fatalf("handler reached = %v; an admin path must never reach a handler for anyone but a listed admin", next.hit)
			}
			if next.hit && IsAdmin(next.ctx) != tc.wantAdmin {
				t.Fatalf("IsAdmin = %v, want %v", IsAdmin(next.ctx), tc.wantAdmin)
			}
		})
	}
}

func TestGateRefusesAdminsTheAlphaAllowlistDenies(t *testing.T) {
	sealer := newSealer(t)
	set := httptest.NewRecorder()
	sealer.Set(set, "did:plc:admin", "sid")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage", nil)
	req.AddCookie(set.Result().Cookies()[0])
	next := &passthroughNext{}
	rr := httptest.NewRecorder()
	New(&alphaResumer{}, noopLocker{}, sealer, Allowlist{"did:plc:reader": {}}, Admins{"did:plc:admin": {}})(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized || next.hit {
		t.Fatalf("status = %d, reached = %v; alpha admission comes before admin rights", rr.Code, next.hit)
	}
}

func TestOnlyTheGateMarksAdmins(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	gate := filepath.Join(root, "internal", "middleware", "auth", "auth.go")
	found := false
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			if name != "WithAdmin" {
				return true
			}
			if path != gate {
				t.Errorf("%s calls WithAdmin; only the auth gate may mark a request admin, after checking ADMIN_DIDS on an admin path", path)
			}
			found = true
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("the gate never calls WithAdmin; restore discovery so this check guards the real call site")
	}
}

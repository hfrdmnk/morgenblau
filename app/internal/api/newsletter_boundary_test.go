package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const newsletterImport = "morgenblau/internal/newsletter"

// Every PDS read or write in this package goes through one of these clients.
var pdsImports = map[string]bool{
	"morgenblau/internal/atprepo":                       true,
	"github.com/bluesky-social/indigo/atproto/atclient": true,
}

type apiDecl struct {
	refs       map[string]bool
	newsletter bool
	sink       string
}

// Taint flows from callee to caller, so any declaration that can reach newsletter values must not also reach a PDS or shared-table write.
func TestNewsletterDeclarationsNeverReachPDSOrSharedTableWrites(t *testing.T) {
	writes := sharedTableWriteQueries(t)
	decls := parseAPIDecls(t, writes)
	for changed := true; changed; {
		changed = false
		for _, decl := range decls {
			for ref := range decl.refs {
				callee := decls[ref]
				if callee == nil || callee == decl {
					continue
				}
				if callee.newsletter && !decl.newsletter {
					decl.newsletter, changed = true, true
				}
				if callee.sink != "" && decl.sink == "" {
					decl.sink, changed = ref+" -> "+callee.sink, true
				}
			}
		}
	}
	for name, decl := range decls {
		if decl.newsletter && decl.sink != "" {
			t.Errorf("%s handles newsletter data and reaches %s; newsletter data stays out of the PDS and shared tables (law 3)", name, decl.sink)
		}
	}
}

func parseAPIDecls(t *testing.T, writes map[string]bool) map[string]*apiDecl {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	decls := map[string]*apiDecl{}
	declare := func(name string) *apiDecl {
		if decls[name] == nil {
			decls[name] = &apiDecl{refs: map[string]bool{}}
		}
		return decls[name]
	}
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		imports := map[string]string{}
		for _, spec := range file.Imports {
			importPath, _ := strconv.Unquote(spec.Path.Value)
			name := importPath[strings.LastIndex(importPath, "/")+1:]
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
		for _, node := range file.Decls {
			for name, body := range declBodies(node) {
				collectDeclRefs(declare(name), body, imports, writes)
			}
		}
	}
	return decls
}

// Methods fold into their receiver type, since a selector call cannot be resolved without type information.
func declBodies(node ast.Decl) map[string]ast.Node {
	bodies := map[string]ast.Node{}
	switch decl := node.(type) {
	case *ast.FuncDecl:
		name := decl.Name.Name
		if decl.Recv != nil && len(decl.Recv.List) > 0 {
			name = receiverName(decl.Recv.List[0].Type)
		}
		bodies[name] = decl
	case *ast.GenDecl:
		for _, spec := range decl.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				bodies[spec.Name.Name] = spec
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					bodies[name.Name] = spec
				}
			}
		}
	}
	return bodies
}

func receiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return receiverName(expr.X)
	case *ast.IndexExpr:
		return receiverName(expr.X)
	case *ast.Ident:
		return expr.Name
	}
	return ""
}

func collectDeclRefs(decl *apiDecl, body ast.Node, imports map[string]string, writes map[string]bool) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := node.X.(*ast.Ident); ok {
				switch importPath := imports[pkg.Name]; {
				case importPath == newsletterImport:
					decl.newsletter = true
				case pdsImports[importPath] && decl.sink == "":
					decl.sink = importPath
				case importPath == "morgenblau/internal/database/db" && writes[strings.TrimSuffix(node.Sel.Name, "Params")] && decl.sink == "":
					decl.sink = "shared-table write " + node.Sel.Name
				}
			}
			if writes[node.Sel.Name] && decl.sink == "" {
				decl.sink = "shared-table write " + node.Sel.Name
			}
		case *ast.Field:
			for _, name := range node.Names {
				if writes[name.Name] && decl.sink == "" {
					decl.sink = "shared-table write " + name.Name
				}
			}
		case *ast.Ident:
			decl.refs[node.Name] = true
		}
		return true
	})
}

func sharedTableWriteQueries(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "database", "queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	queryName := regexp.MustCompile(`^--\s*name:\s*(\w+)`)
	write := regexp.MustCompile(`(?i)^\s*(insert|update|delete|replace)\b`)
	writes := map[string]bool{}
	for _, path := range paths {
		if filepath.Base(path) == "newsletters.sql" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		current := ""
		for _, line := range strings.Split(string(data), "\n") {
			if match := queryName.FindStringSubmatch(line); match != nil {
				current = match[1]
				continue
			}
			if current == "" || strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			if write.MatchString(line) {
				writes[current] = true
			}
			current = ""
		}
	}
	if len(writes) == 0 {
		t.Fatal("no shared-table write queries found")
	}
	return writes
}

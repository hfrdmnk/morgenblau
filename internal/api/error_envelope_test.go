package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The frontend keys reauth and form errors off the {code, message} body that respond.go writes; any other error shape reaches it as an unclassified failure.
func TestAPIErrorsUseTheRespondEnvelope(t *testing.T) {
	codes := respondCodes(t)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	writeErrorCalls := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				for _, plain := range []string{"Error", "NotFound", "NotFoundHandler"} {
					if isPackageCall(node, "http", plain) {
						t.Errorf("%s: http.%s writes a plain-text body; use writeError so the frontend gets {code, message}", fset.Position(node.Pos()), plain)
					}
				}
				if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "writeError" && len(node.Args) == 4 {
					writeErrorCalls++
					if code, ok := node.Args[2].(*ast.Ident); !ok || !codes[code.Name] {
						t.Errorf("%s: writeError code must be a constant declared in respond.go; the frontend keys off those slugs, so any other one is invisible to it", fset.Position(node.Args[2].Pos()))
					}
				}
			case *ast.CompositeLit:
				if _, ok := node.Type.(*ast.MapType); ok && hasStringKey(node, "message") {
					t.Errorf("%s: ad-hoc error map; use writeError so every error body has the same {code, message} shape", fset.Position(node.Pos()))
				}
			}
			return true
		})
	}
	if writeErrorCalls == 0 {
		t.Error("found no writeError calls; the check is no longer looking at the handlers")
	}
}

func respondCodes(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "respond.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				codes[name.Name] = true
			}
		}
	}
	if len(codes) == 0 {
		t.Fatal("respond.go declares no error codes")
	}
	return codes
}

func isPackageCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg && sel.Sel.Name == name
}

func hasStringKey(lit *ast.CompositeLit, key string) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if basic, ok := kv.Key.(*ast.BasicLit); ok && basic.Kind == token.STRING {
			if value, err := strconv.Unquote(basic.Value); err == nil && value == key {
				return true
			}
		}
	}
	return false
}

package sync

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The fetch_one_feed job finishes on its own fetch; every sync_user completion must go through finishSync's commit proofs.
var setDoneCallers = map[string]bool{
	"Engine.finishSync":              true,
	"Orchestrator.StartFetchOneFeed": true,
}

func TestSyncUserReachesDoneOnlyThroughFinishSync(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, dir := range []string{"cmd", "internal"} {
		_, files := parseGoSources(t, filepath.Join(root, dir))
		for path, file := range files {
			eachFuncDecl(file, func(name string, body ast.Node) {
				ast.Inspect(body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetDone" {
						seen[name] = true
						if !setDoneCallers[name] {
							rel, _ := filepath.Rel(root, path)
							t.Errorf("%s: %s marks a job done outside finishSync; only finishSync checks that every reconcile pass committed (law 2)", rel, name)
						}
					}
					return true
				})
			})
		}
	}
	if !seen["Engine.finishSync"] {
		t.Error("Engine.finishSync no longer marks sync_user jobs done")
	}
}

func TestCommitProofIsMintedOnlyByReconcileCollection(t *testing.T) {
	minted := false
	_, files := parseGoSources(t, ".")
	for path, file := range files {
		for _, decl := range file.Decls {
			name := "package scope"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				name = funcName(fn)
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				if !constructsCommitted(n) {
					return true
				}
				if name != "reconcileCollection" {
					t.Errorf("%s: %s constructs a commit proof; only reconcileCollection may, after its transaction commits, or done stops meaning caught up (law 2)", path, name)
					return true
				}
				minted = true
				return true
			})
		}
	}
	if !minted {
		t.Error("reconcileCollection no longer mints a commit proof")
	}
}

// constructsCommitted matches every syntactic way to obtain a committed value: literal, new, conversion, or zero-value var.
func constructsCommitted(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.CompositeLit:
		return isCommittedType(n.Type)
	case *ast.CallExpr:
		if ident, ok := n.Fun.(*ast.Ident); ok && ident.Name == "new" && len(n.Args) == 1 {
			return isCommittedType(n.Args[0])
		}
		return isCommittedType(n.Fun)
	case *ast.ValueSpec:
		return isCommittedType(n.Type)
	}
	return false
}

func isCommittedType(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "committed"
	case *ast.IndexExpr:
		return isCommittedType(e.X)
	case *ast.IndexListExpr:
		return isCommittedType(e.X)
	case *ast.ParenExpr:
		return isCommittedType(e.X)
	}
	return false
}

func eachFuncDecl(file *ast.File, visit func(name string, body ast.Node)) {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			visit(funcName(fn), fn.Body)
		}
	}
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if index, ok := recv.(*ast.IndexExpr); ok {
		recv = index.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func parseGoSources(t *testing.T, dir string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	files := map[string]*ast.File{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		files[path] = file
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fset, files
}

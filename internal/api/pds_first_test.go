package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The reading index is rebuilt from the PDS by internal/sync; the database package holds the transactional mirror helpers.
var readingIndexWriters = []string{
	filepath.Join("internal", "sync"),
	filepath.Join("internal", "database"),
}

// Every local write to the reading index must sit in commitThenMirror's mirror closure, which runs only after the PDS commit and never fails the response.
func TestReadingIndexWritesRunOnlyAsCommitThenMirrorMirrors(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	writes := readingIndexWrites(t, root)
	mirrored := 0
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if entry.IsDir() {
				for _, owner := range readingIndexWriters {
					if rel == owner {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, name := range indexWritesOutsideMirrors(file, writes, &mirrored) {
				t.Errorf("%s: %s writes the reading index outside a commitThenMirror mirror", rel, name)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if mirrored == 0 {
		t.Error("found no reading-index write inside a commitThenMirror mirror; the check is no longer looking at the handlers")
	}
}

// indexWritesOutsideMirrors returns each reading-index write not lexically inside the mirror argument of a commitThenMirror call.
func indexWritesOutsideMirrors(file *ast.File, writes map[string]bool, mirrored *int) []string {
	mirrors := map[ast.Node]bool{}
	var stack []ast.Node
	var outside []string
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		if call, ok := n.(*ast.CallExpr); ok && isCommitThenMirror(call.Fun) && len(call.Args) == 6 {
			mirrors[call.Args[5]] = true
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || !writes[sel.Sel.Name] {
			return true
		}
		for _, ancestor := range stack {
			if mirrors[ancestor] {
				*mirrored++
				return true
			}
		}
		outside = append(outside, sel.Sel.Name)
		return true
	})
	return outside
}

func isCommitThenMirror(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "commitThenMirror"
	case *ast.IndexExpr:
		return isCommitThenMirror(f.X)
	case *ast.IndexListExpr:
		return isCommitThenMirror(f.X)
	}
	return false
}

// readingIndexWrites names the queries writing user_subscriptions or user_saves, the Tier-2 UpsertFeed a subscription mirror needs, and the database helpers wrapping them.
func readingIndexWrites(t *testing.T, root string) map[string]bool {
	t.Helper()
	writes := map[string]bool{"UpsertFeed": true}
	paths, err := filepath.Glob(filepath.Join(root, "internal", "database", "queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	queryName := regexp.MustCompile(`(?m)^--\s*name:\s*(\w+)`)
	tableWrite := regexp.MustCompile(`(?is)\b(insert\s+(or\s+\w+\s+)?into|update|delete\s+from|replace\s+into)\s+(user_subscriptions|user_saves)\b`)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := string(data)
		bounds := queryName.FindAllStringSubmatchIndex(sql, -1)
		for i, b := range bounds {
			end := len(sql)
			if i+1 < len(bounds) {
				end = bounds[i+1][0]
			}
			if tableWrite.MatchString(sql[b[1]:end]) {
				writes[sql[b[2]:b[3]]] = true
			}
		}
	}
	helpers, err := filepath.Glob(filepath.Join(root, "internal", "database", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range helpers {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !fn.Name.IsExported() {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && writes[sel.Sel.Name] {
					writes[fn.Name.Name] = true
				}
				return true
			})
		}
	}
	for _, want := range []string{"UpsertUserSubscription", "DeleteUserSubscription", "UpsertUserSave", "DeleteUserSave", "MirrorImportedSubscription"} {
		if !writes[want] {
			t.Fatalf("reading-index writes = %v, missing %s; the query scan no longer recognises the write queries", writes, want)
		}
	}
	return writes
}

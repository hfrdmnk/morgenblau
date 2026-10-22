package database

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// eachGoFile parses every hand-written Go file under cmd/ and internal/, either the test files or the rest.
func eachGoFile(t *testing.T, testFiles bool, visit func(path string, fset *token.FileSet, file *ast.File)) {
	t.Helper()
	generated := filepath.Join("..", "..", "internal", "database", "db")
	visited := 0
	for _, root := range []string{filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "internal")} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path == generated {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") != testFiles {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			visited++
			visit(path, fset, file)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if visited == 0 {
		t.Fatal("found no Go files; the scan is no longer looking at the code")
	}
}

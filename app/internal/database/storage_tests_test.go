package database

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

func TestStorageTestsUseAFileNotMemory(t *testing.T) {
	eachGoFile(t, true, func(path string, fset *token.FileSet, file *ast.File) {
		if strings.HasSuffix(path, "storage_tests_test.go") {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, ":memory:") {
				t.Errorf("%s: SQLite :memory: gives each pool its own database; open a file in t.TempDir() instead", fset.Position(lit.Pos()))
			}
			return true
		})
	})
}

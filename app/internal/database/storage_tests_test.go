package database

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// Each :memory: connection opens its own database, so the reader pool would never see the writer pool's rows.
func TestStorageTestsUseAFileNotMemory(t *testing.T) {
	eachGoFile(t, true, func(path string, fset *token.FileSet, file *ast.File) {
		// This file names the DSN it bans.
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

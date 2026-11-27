package comments

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const Policy = "Comments go stale and nothing enforces them. Make the code say it: a clearer name, an extracted function, a type or a union. If the next change must keep a rule, encode it as a test or lint rule whose failure message gives the reason. Put the reason for this change in the commit message. Only a note about something outside our control stays, and it links the upstream issue or docs. The baseline only covers comments that predate this check: never add to it."

const NolintPolicy = "Fix the code instead of silencing the linter, or change .golangci.yml if the rule is wrong for the whole codebase. " + Policy

const BaselineFile = "comments-baseline.json"

var (
	directive = regexp.MustCompile(`^//([a-z0-9]+:[a-z0-9]|line |extern |export )`)
	nolint    = regexp.MustCompile(`^//\s*nolint\b`)
	link      = regexp.MustCompile("https?://[^\\s\"'<>()\\[\\]`]+")
	ownLink   = regexp.MustCompile(`^https?://(([a-z0-9-]+\.)*morgen\.blue([/:?#]|$)|(www\.)?(github\.com|tangled\.(org|sh))/[^/]+/morgenblau([/?#.]|$))`)
	license   = regexp.MustCompile(`(?i)copyright|spdx-license-identifier|@license`)
	output    = regexp.MustCompile(`^//\s*(Unordered output|Output):`)
)

type Report struct {
	Counted []token.Position
	Nolint  []token.Position
}

func Check(fset *token.FileSet, file *ast.File) Report {
	var report Report
	if ast.IsGenerated(file) {
		return report
	}
	for _, group := range groups(fset, file) {
		kept := keep(fset, file, group)
		for _, c := range group {
			switch {
			case nolint.MatchString(c.Text):
				report.Nolint = append(report.Nolint, fset.Position(c.Pos()))
			case !kept && !directive.MatchString(c.Text):
				report.Counted = append(report.Counted, fset.Position(c.Pos()))
			}
		}
	}
	return report
}

func groups(fset *token.FileSet, file *ast.File) [][]*ast.Comment {
	var groups [][]*ast.Comment
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if n := len(groups); n > 0 && continuesLineGroup(fset, groups[n-1][len(groups[n-1])-1], c) {
				groups[n-1] = append(groups[n-1], c)
			} else {
				groups = append(groups, []*ast.Comment{c})
			}
		}
	}
	return groups
}

func continuesLineGroup(fset *token.FileSet, prev, next *ast.Comment) bool {
	p, n := fset.Position(prev.Pos()), fset.Position(next.Pos())
	return strings.HasPrefix(prev.Text, "//") && strings.HasPrefix(next.Text, "//") && n.Line == p.Line+1 && n.Column == p.Column
}

func keep(fset *token.FileSet, file *ast.File, group []*ast.Comment) bool {
	for _, c := range group {
		for _, url := range link.FindAllString(c.Text, -1) {
			if !ownLink.MatchString(url) {
				return true
			}
		}
		if c.End() < file.Package && license.MatchString(c.Text) {
			return true
		}
	}
	return output.MatchString(group[0].Text) && inExample(fset, file, group[0].Pos())
}

func inExample(fset *token.FileSet, file *ast.File, pos token.Pos) bool {
	if !strings.HasSuffix(fset.Position(file.Pos()).Filename, "_test.go") {
		return false
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Body != nil && strings.HasPrefix(fn.Name.Name, "Example") && fn.Body.Pos() < pos && pos < fn.Body.End() {
			return true
		}
	}
	return false
}

func Scan(root string) (map[string]Report, error) {
	reports := map[string]Report{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "node_modules" || d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".") || strings.HasPrefix(d.Name(), "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, filepath.ToSlash(rel), src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		reports[filepath.ToSlash(rel)] = Check(fset, file)
		return nil
	})
	return reports, err
}

func ReadBaseline(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var baseline map[string]int
	return baseline, json.Unmarshal(data, &baseline)
}

func Prune(baseline, counts map[string]int) map[string]int {
	pruned := map[string]int{}
	for file, allowed := range baseline {
		if n := min(allowed, counts[file]); n > 0 {
			pruned[file] = n
		}
	}
	return pruned
}

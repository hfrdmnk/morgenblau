package database

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const newsletterQueryFile = "newsletters.sql"

// Every other newsletter query must bind each newsletter table it touches to the caller's DID.
var unscopedNewsletterQueries = map[string]string{
	"GetNewsletterAddressByLocalPart": "SMTP routing resolves the owner DID from the inbound local part",
	"GetNextNewsletterReceipt":        "the ingest worker drains the queue for every owner",
	"MarkNewsletterReceiptFailed":     "the receipt id comes from the ingest queue, never from a request",
	"DeleteNewsletterReceipt":         "the receipt id comes from the ingest queue, never from a request",
	"GetNewsletterGlobalStorageBytes": "the global quota sums storage across all owners",
}

type sqlQuery struct {
	file string
	name string
	sql  string
}

type tableRef struct {
	table string
	alias string
}

var (
	sqlQueryName   = regexp.MustCompile(`^--\s*name:\s*(\w+)`)
	sqlTableRef    = regexp.MustCompile(`(?i)\b(?:from|join|update|into)\s+(\w+)(?:\s+(?:as\s+)?(\w+))?`)
	sqlInsertCols  = regexp.MustCompile(`(?is)\binsert\s+into\s+\w+\s*\(([^)]*)\)`)
	sqlDIDEquality = regexp.MustCompile(`(?i)(\w+\.did\b|\bdid\b|\?\d*|sqlc\.arg\(\s*did\s*\)|@did)\s*=\s*(\w+\.did\b|\bdid\b|\?\d*|sqlc\.arg\(\s*did\s*\)|@did)`)
	sqlKeywords    = map[string]bool{"where": true, "on": true, "set": true, "join": true, "left": true, "inner": true, "cross": true, "order": true, "group": true, "limit": true, "values": true, "select": true, "and": true, "or": true}
)

func TestNewsletterTablesStayInNewsletterQueries(t *testing.T) {
	for _, query := range loadSQLQueries(t) {
		for _, ref := range sqlTableRefs(query.sql) {
			isNewsletter := strings.HasPrefix(ref.table, "newsletter_")
			if query.file == newsletterQueryFile && !isNewsletter {
				t.Errorf("%s reads or writes %s; newsletter queries touch only newsletter tables", query.name, ref.table)
			}
			if query.file != newsletterQueryFile && isNewsletter {
				t.Errorf("%s in %s touches %s; newsletter tables belong to %s only", query.name, query.file, ref.table, newsletterQueryFile)
			}
		}
	}
}

func TestNewsletterQueriesAreScopedByOwnerDID(t *testing.T) {
	seen := map[string]bool{}
	for _, query := range loadSQLQueries(t) {
		if query.file != newsletterQueryFile {
			continue
		}
		seen[query.name] = true
		problems := newsletterScopeProblems(query.sql)
		_, unscoped := unscopedNewsletterQueries[query.name]
		switch {
		case unscoped && len(problems) == 0:
			t.Errorf("%s is owner-scoped; drop it from unscopedNewsletterQueries", query.name)
		case !unscoped:
			for _, problem := range problems {
				t.Errorf("%s: %s", query.name, problem)
			}
		}
	}
	for name := range unscopedNewsletterQueries {
		if !seen[name] {
			t.Errorf("unscopedNewsletterQueries lists %s, which is not in %s", name, newsletterQueryFile)
		}
	}
}

func TestNewsletterTablesAreQueriedOnlyThroughSQLFiles(t *testing.T) {
	table := regexp.MustCompile(`\bnewsletter_[a-z_]+`)
	for _, root := range []string{filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "internal")} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if path == filepath.Join("..", "..", "internal", "database", "db") {
					return filepath.SkipDir
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
			ast.Inspect(file, func(node ast.Node) bool {
				if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING && table.MatchString(literal.Value) {
					t.Errorf("%s embeds SQL for %s; add the query to queries/%s instead", path, table.FindString(literal.Value), newsletterQueryFile)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func newsletterScopeProblems(sql string) []string {
	var problems []string
	if columns := sqlInsertCols.FindStringSubmatch(sql); columns != nil {
		if !slices.Contains(splitColumns(columns[1]), "did") {
			problems = append(problems, "insert does not set did")
		}
		return problems
	}

	parent := map[string]string{}
	var find func(string) string
	find = func(key string) string {
		if parent[key] == "" || parent[key] == key {
			return key
		}
		return find(parent[key])
	}
	refs := sqlTableRefs(sql)
	occurrences := map[string]int{}
	for _, ref := range refs {
		occurrences[ref.alias]++
	}
	mentions := map[string]int{}
	for _, match := range sqlDIDEquality.FindAllStringSubmatch(sql, -1) {
		left, right := didKey(match[1], refs), didKey(match[2], refs)
		mentions[left]++
		mentions[right]++
		parent[find(left)] = find(right)
	}
	for alias, count := range occurrences {
		if find(alias) != find("$did") {
			problems = append(problems, alias+".did is not bound to the owner DID")
		} else if mentions[alias] < count {
			problems = append(problems, alias+" appears in more scopes than it has did predicates")
		}
	}
	slices.Sort(problems)
	return problems
}

// An unqualified did refers to the block's single unaliased table, which keys under its own name.
func didKey(term string, refs []tableRef) string {
	lower := strings.ToLower(term)
	switch {
	case strings.HasSuffix(lower, ".did"):
		return strings.TrimSuffix(lower, ".did")
	case lower == "did":
		for _, ref := range refs {
			if ref.alias == ref.table {
				return ref.alias
			}
		}
		return "did"
	default:
		return "$did"
	}
}

func sqlTableRefs(sql string) []tableRef {
	var refs []tableRef
	for _, match := range sqlTableRef.FindAllStringSubmatch(sql, -1) {
		table := strings.ToLower(match[1])
		if sqlKeywords[table] {
			continue
		}
		alias := strings.ToLower(match[2])
		if alias == "" || sqlKeywords[alias] {
			alias = table
		}
		refs = append(refs, tableRef{table: table, alias: alias})
	}
	return refs
}

func splitColumns(list string) []string {
	var columns []string
	for _, column := range strings.Split(list, ",") {
		columns = append(columns, strings.ToLower(strings.TrimSpace(column)))
	}
	return columns
}

func loadSQLQueries(t *testing.T) []sqlQuery {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var queries []sqlQuery
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var current *sqlQuery
		for _, line := range strings.Split(string(data), "\n") {
			if match := sqlQueryName.FindStringSubmatch(line); match != nil {
				queries = append(queries, sqlQuery{file: filepath.Base(path), name: match[1]})
				current = &queries[len(queries)-1]
				continue
			}
			if current == nil || strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			current.sql += line + "\n"
		}
	}
	if len(queries) == 0 {
		t.Fatal("no queries found")
	}
	return queries
}

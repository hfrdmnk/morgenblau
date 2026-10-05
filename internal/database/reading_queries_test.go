package database

import (
	"slices"
	"testing"
)

func TestReadingIndexQueriesAreScopedByOwnerDID(t *testing.T) {
	checked := 0
	for _, query := range loadSQLQueries(t) {
		refs := readingTableRefs(query.sql)
		if len(refs) == 0 {
			continue
		}
		checked++
		for _, problem := range ownerScopeProblems(query.sql, refs) {
			t.Errorf("%s in %s: %s; bind every reading-index table to the caller DID so one reader cannot reach another's subscriptions or saves", query.name, query.file, problem)
		}
	}
	if checked == 0 {
		t.Fatal("no reading-index queries checked; ownership protection must cover the SQL that actually runs")
	}
}

func TestReadingOwnerScopeProblems(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		want []string
	}{
		{"shared catalog", "SELECT * FROM feeds", nil},
		{"scoped read", "SELECT * FROM user_saves WHERE did = ? AND rkey = ?", nil},
		{"unscoped delete", "DELETE FROM user_saves WHERE rkey = ?", []string{"user_saves.did is not bound to the owner DID"}},
		{"shared join", "SELECT * FROM user_subscriptions us JOIN feeds f ON f.feed_url = us.feed_url WHERE us.did = sqlc.arg(did)", nil},
		{"correlated owner", "SELECT (SELECT COUNT(*) FROM user_saves s WHERE s.did = us.did) FROM user_subscriptions us WHERE us.did = ?", nil},
		{"correlated other owner", "SELECT (SELECT COUNT(*) FROM user_saves s WHERE s.feed_url = us.feed_url) FROM user_subscriptions us WHERE us.did = ?", []string{"s.did is not bound to the owner DID"}},
		{"insert without owner", "INSERT INTO user_saves (rkey, item_url) VALUES (?, ?)", []string{"insert does not set did"}},
		{"insert with owner", "INSERT INTO user_saves (did, rkey, item_url) VALUES (?, ?, ?)", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ownerScopeProblems(tc.sql, readingTableRefs(tc.sql))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("scope problems = %v, want %v", got, tc.want)
			}
		})
	}
}

func readingTableRefs(sql string) []tableRef {
	var owned []tableRef
	for _, ref := range sqlTableRefs(sql) {
		if ref.table == "user_subscriptions" || ref.table == "user_saves" {
			owned = append(owned, ref)
		}
	}
	return owned
}

package database

import (
	"slices"
	"strings"
	"testing"
)

// Every other reading-index query must bind each reading table it touches to the caller's DID.
var unscopedReadingQueries = map[string]string{
	"IsWebSourceFollowedByOthers": "a shared generated feed asks only whether anyone else follows it, a yes or no that names no reader",
	"ClaimFollowedFeeds":          "the shared-cache sweep asks only whether anyone follows a feed, a yes or no that names no reader",
	"MarkOrphanedFeeds":           "the shared-cache sweep asks only whether anyone follows a feed, a yes or no that names no reader",
	"DeleteOrphanedFeeds":         "the shared-cache sweep asks only whether anyone follows a feed or saved one of its entries, a yes or no that names no reader",
	"ListFollowedFeedURLs":        "the global refresher needs every feed anyone follows; it reads feed URLs and names no reader",
	"ListDueWebSources":           "the web poller asks only whether anyone follows a generated feed, a yes or no that names no reader",
	"CountSubscriptionsByReader":  "admin usage counts each reader's subscriptions by type and names no feed; only the admin gate reaches it",
}

func TestReadingIndexQueriesAreScopedByOwnerDID(t *testing.T) {
	checked := 0
	seen := map[string]bool{}
	for _, query := range loadSQLQueries(t) {
		refs := readingTableRefs(query.sql)
		if len(refs) == 0 {
			continue
		}
		checked++
		seen[query.name] = true
		problems := ownerScopeProblems(query.sql, refs)
		_, unscoped := unscopedReadingQueries[query.name]
		switch {
		case unscoped && len(problems) == 0:
			t.Errorf("%s is owner-scoped; drop it from unscopedReadingQueries so an obsolete exception cannot hide a future ownership regression", query.name)
		case !unscoped:
			for _, problem := range problems {
				t.Errorf("%s in %s: %s; bind every reading-index table to the caller DID so one reader cannot reach another's subscriptions or saves", query.name, query.file, problem)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no reading-index queries checked; ownership protection must cover the SQL that actually runs")
	}
	for name, reason := range unscopedReadingQueries {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("unscopedReadingQueries lists %s without a reason; justify every owner-scoping exception so its isolation trade-off remains auditable", name)
		}
		if !seen[name] {
			t.Errorf("unscopedReadingQueries lists %s, which reads no reading-index table; remove stale exceptions so the audited ownership boundary matches the queries that run", name)
		}
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

package database

import (
	"database/sql"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A deploy rollback runs one goose down; a Down that misses part of its Up leaves a schema the Up can't be re-applied to.
func TestEveryMigrationRollsBackCleanly(t *testing.T) {
	versions, err := filepath.Glob(filepath.Join("migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("no migrations found")
	}
	databasePath := filepath.Join(t.TempDir(), "rollback.db")
	db := openMigrationDB(t, databasePath)
	for _, path := range versions {
		before := schemaSnapshot(t, db)
		runGoose(t, databasePath, "up-by-one")
		runGoose(t, databasePath, "down")
		if after := schemaSnapshot(t, db); after != before {
			t.Errorf("%s: schema after down differs from before up; its Down must undo everything its Up does so a rollback can be re-applied\nbefore:\n%s\nafter:\n%s", filepath.Base(path), before, after)
		}
		runGoose(t, databasePath, "up-by-one")
	}
}

func schemaSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT type || ' ' || name || ': ' || COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name != 'goose_db_version' ORDER BY type, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var entries []string
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(entries, "\n")
}

func openMigrationDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPublicationDateRepairInvalidatesAllRSSValidators(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repair.db")
	db := openMigrationDB(t, path)
	runGoose(t, path, "up-to", "20260920000005")
	for _, fixture := range []struct {
		name, kind, published string
	}{
		{"fallback", "rss", "2026-10-05T12:00:00Z"},
		{"skewed", "rss", "2026-10-05T12:00:02Z"},
		{"dated", "rss", "2025-11-08T00:00:00Z"},
		{"native", "standardfeed", "2026-10-05T12:00:00Z"},
	} {
		url := "https://" + fixture.name + ".example.com/feed"
		if _, err := db.Exec(`INSERT INTO feeds (feed_url, kind, etag, last_modified, created_at, updated_at) VALUES (?, ?, 'validator', 'modified', 'created', 'updated')`, url, fixture.kind); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO feed_entries (feed_url, guid, entry_slug, url, content_type, published_at, fetched_at) VALUES (?, 'post', ?, ?, 'blogpost', ?, '2026-10-05T12:00:00Z')`, url, fixture.name, url+"/post", fixture.published); err != nil {
			t.Fatal(err)
		}
	}
	runGoose(t, path, "up")
	for _, name := range []string{"fallback", "skewed", "dated", "native"} {
		var etag, modified sql.NullString
		if err := db.QueryRow(`SELECT etag, last_modified FROM feeds WHERE feed_url = ?`, "https://"+name+".example.com/feed").Scan(&etag, &modified); err != nil {
			t.Fatal(err)
		}
		if name != "native" {
			if etag.Valid || modified.Valid {
				t.Errorf("%s: validators retained: %v, %v", name, etag, modified)
			}
		} else if etag != (sql.NullString{String: "validator", Valid: true}) || modified != (sql.NullString{String: "modified", Valid: true}) {
			t.Errorf("%s: validators changed: %v, %v", name, etag, modified)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM feed_entries`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("entry count = %d, want 4", count)
	}
}

func runGoose(t *testing.T, databasePath string, args ...string) {
	t.Helper()
	if err := runGooseCommand(databasePath, args...); err != nil {
		t.Fatal(err)
	}
}

func runGooseCommand(databasePath string, args ...string) error {
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		return fmt.Errorf("goose CLI is required for migration tests; install github.com/pressly/goose/v3/cmd/goose@v3.27.1: %w", err)
	}
	migrationDir, err := filepath.Abs("migrations")
	if err != nil {
		return err
	}
	commandArgs := []string{"-dir", migrationDir, "sqlite3", databasePath}
	commandArgs = append(commandArgs, args...)
	output, err := exec.Command(goosePath, commandArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("goose %s: %w\n%s", strings.Join(args, " "), err, output)
	}
	return nil
}

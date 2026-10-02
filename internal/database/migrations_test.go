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

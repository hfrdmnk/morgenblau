package database

import (
	"database/sql"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestNewsletterMigrationsFreshUpDown(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "fresh.db")
	migrations := []struct {
		version string
		table   string
	}{
		{"20260920000000", "newsletter_addresses"},
		{"20260920000001", "newsletter_sources"},
		{"20260920000002", "newsletter_receipts"},
		{"20260920000003", "newsletter_messages"},
		{"20260920000004", "newsletter_inline_assets"},
		{"20260920000005", "newsletter_saves"},
	}
	var expectedTables []string
	for _, migration := range migrations {
		runNewsletterGoose(t, databasePath, "up-to", migration.version)
		expectedTables = append(expectedTables, migration.table)
		db := openNewsletterMigrationDB(t, databasePath)
		assertNewsletterTables(t, db, expectedTables)
		_ = db.Close()
	}

	db := openNewsletterMigrationDB(t, databasePath)
	if !newsletterColumnExists(t, db, "newsletter_receipts", "recipient_local_part") {
		t.Fatal("fresh schema is missing recipient_local_part")
	}
	assertNewsletterForeignKeysValid(t, db)
	assertNewsletterForeignKeysRejectCrossOwnerRows(t, db)
	assertNewsletterForeignKeysValid(t, db)
	_ = db.Close()

	for range migrations {
		runNewsletterGoose(t, databasePath, "down")
	}
	db = openNewsletterMigrationDB(t, databasePath)
	defer db.Close()
	assertNewsletterTables(t, db, nil)
	assertNewsletterForeignKeysValid(t, db)
}

func assertNewsletterTables(t *testing.T, db *sql.DB, want []string) {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master
		WHERE type = 'table' AND name LIKE 'newsletter_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != strings.Join(sortedNewsletterTables(want), ",") {
		t.Fatalf("newsletter tables = %v, want %v", got, sortedNewsletterTables(want))
	}
}

func sortedNewsletterTables(tables []string) []string {
	copyOfTables := append([]string(nil), tables...)
	sort.Strings(copyOfTables)
	return copyOfTables
}

func assertNewsletterForeignKeysRejectCrossOwnerRows(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, address := range []struct{ did, localPart string }{
		{"did:plc:reader-a", "reader-a-random"},
		{"did:plc:reader-b", "reader-b-random"},
	} {
		if _, err := db.Exec(`INSERT INTO newsletter_addresses (did, local_part, created_at) VALUES (?, ?, ?)`, address.did, address.localPart, now); err != nil {
			t.Fatalf("insert newsletter address: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO newsletter_sources (id, did, source_key, identity_kind, title, sender_address, created_at, updated_at)
		VALUES ('source-a', 'did:plc:reader-a', 'manual:one', 'manual', 'One', '', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert newsletter source: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO newsletter_receipts (id, did, envelope_from, recipient, recipient_local_part, received_at, raw_mime, reserved_bytes, created_at)
		VALUES ('receipt-a', 'did:plc:reader-a', '', 'reader-a-random@inbound.example', 'reader-a-random', ?, X'01', 1, ?)`, now, now); err != nil {
		t.Fatalf("insert newsletter receipt: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO newsletter_messages (id, did, source_id, entry_slug, dedupe_key, sender_address, received_at, created_at, updated_at)
		VALUES ('message-a', 'did:plc:reader-a', 'source-a', 'message-a', 'dedupe-a', '', ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert newsletter message: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO newsletter_saves (id, did, message_id, created_at)
		VALUES ('save-a', 'did:plc:reader-a', 'message-a', ?)`, now); err != nil {
		t.Fatalf("insert newsletter save: %v", err)
	}

	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{"unowned source", `INSERT INTO newsletter_sources (id, did, source_key, identity_kind, title, sender_address, created_at, updated_at) VALUES (?, ?, ?, 'manual', ?, '', ?, ?)`, []any{"source-unowned", "did:plc:unknown", "manual:unowned", "Unowned", now, now}},
		{"cross-owner message", `INSERT INTO newsletter_messages (id, did, source_id, entry_slug, dedupe_key, sender_address, received_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, '', ?, ?, ?)`, []any{"message-b", "did:plc:reader-b", "source-a", "message-b", "dedupe-b", now, now, now}},
		{"cross-owner save", `INSERT INTO newsletter_saves (id, did, message_id, created_at) VALUES (?, ?, ?, ?)`, []any{"save-b", "did:plc:reader-b", "message-a", now}},
		{"cross-owner receipt", `INSERT INTO newsletter_receipts (id, did, envelope_from, recipient, recipient_local_part, received_at, raw_mime, reserved_bytes, created_at) VALUES (?, ?, '', ?, ?, ?, X'01', 1, ?)`, []any{"receipt-b", "did:plc:reader-a", "reader-b-random@inbound.example", "reader-b-random", now, now}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err == nil {
			t.Errorf("%s insert succeeded", statement.name)
		}
	}
}

func assertNewsletterForeignKeysValid(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID, parentTable string
		var fkID int
		if err := rows.Scan(&table, &rowID, &parentTable, &fkID); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("foreign key violation: table %s row %s parent %s constraint %d", table, rowID, parentTable, fkID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func newsletterColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func openNewsletterMigrationDB(t *testing.T, path string) *sql.DB {
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

func runNewsletterGoose(t *testing.T, databasePath string, args ...string) {
	t.Helper()
	if err := runNewsletterGooseCommand(databasePath, args...); err != nil {
		t.Fatal(err)
	}
}

func runNewsletterGooseCommand(databasePath string, args ...string) error {
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

package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryNewsletterTableIsOwnerScopedBySchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "schema.db")
	runGoose(t, databasePath, "up")
	db := openMigrationDB(t, databasePath)

	tables := newsletterTableNames(t, db)
	if len(tables) == 0 {
		t.Fatal("no newsletter tables found")
	}
	for _, table := range tables {
		if !newsletterDIDIsNotNull(t, db, table) {
			t.Errorf("%s.did must be declared NOT NULL; a row without an owner is readable by no one and scoped by nothing (law 3)", table)
		}
		parents := 0
		for _, fk := range newsletterForeignKeys(t, db, table) {
			if !strings.HasPrefix(fk.parent, "newsletter_") {
				continue
			}
			parents++
			if fk.columns["did"] != "did" {
				t.Errorf("%s foreign key to %s must map did -> did, got %v; otherwise a row can point at another reader's data (law 3)", table, fk.parent, fk.columns)
			}
		}
		// The address row is the owner root; every other newsletter row hangs off an owned parent.
		if table != "newsletter_addresses" && parents == 0 {
			t.Errorf("%s has no owner-scoped foreign key to a newsletter table; the schema must tie every row to its reader (law 3)", table)
		}
	}
}

func newsletterTableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'newsletter\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return tables
}

func newsletterDIDIsNotNull(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var notNull int
	err := db.QueryRow(`SELECT "notnull" FROM pragma_table_info(?) WHERE name = 'did'`, table).Scan(&notNull)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return notNull == 1
}

type newsletterForeignKey struct {
	parent  string
	columns map[string]string
}

func newsletterForeignKeys(t *testing.T, db *sql.DB, table string) []newsletterForeignKey {
	t.Helper()
	rows, err := db.Query(`SELECT id, "table", "from", "to" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []newsletterForeignKey
	lastID := -1
	for rows.Next() {
		var id int
		var parent, from, to string
		if err := rows.Scan(&id, &parent, &from, &to); err != nil {
			t.Fatal(err)
		}
		if id != lastID {
			keys = append(keys, newsletterForeignKey{parent: parent, columns: map[string]string{}})
			lastID = id
		}
		keys[len(keys)-1].columns[from] = to
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return keys
}

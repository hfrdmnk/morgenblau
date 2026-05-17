package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/joho/godotenv/autoload"
	_ "modernc.org/sqlite"
)

// Open returns a *sql.DB connected to the SQLite file at DB_PATH
// (defaults to ./data/morgenblau.db). Parent dir is created if missing.
// Callers own the lifecycle and should defer Close.
func Open() (*sql.DB, error) {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./data/morgenblau.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)&_pragma=synchronous(normal)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite serialises writes; cap at one conn so upsert-heavy fan-out can't race into SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	return db, nil
}

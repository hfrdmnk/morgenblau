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
	return sql.Open("sqlite", dsn)
}

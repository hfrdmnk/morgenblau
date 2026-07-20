package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"morgenblau/internal/database/db"
)

// DB holds two connection pools over the same SQLite file: WAL lets readers run concurrently with the single writer, so reads never queue behind a write-heavy sync sweep.
type DB struct {
	// Writer is capped at one connection (SQLite serialises writes); BEGIN IMMEDIATE takes the write lock up front.
	Writer *sql.DB
	// Reader is a small pool of WAL readers that don't block the writer or each other.
	Reader *sql.DB
}

// Open returns a DB backed by DB_PATH (default ./data/morgenblau.db); callers own the lifecycle and should defer Close.
func Open() (*DB, error) {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./data/morgenblau.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	base := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)&_pragma=synchronous(normal)",
		path,
	)

	// _txlock=immediate takes the write lock at BEGIN, guarding against an out-of-process writer (goose CLI, sqlite3 shell) grabbing it mid-transaction.
	writer, err := sql.Open("sqlite", base+"&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", base)
	if err != nil {
		writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(4)

	return &DB{Writer: writer, Reader: reader}, nil
}

// Close closes both pools, returning the first error.
func (d *DB) Close() error {
	err := d.Writer.Close()
	if rerr := d.Reader.Close(); err == nil {
		err = rerr
	}
	return err
}

// WithTx runs fn in a single transaction on the writer pool, committing on success and rolling back on error.
// The transaction holds the sole writer connection: fn must never touch a non-transaction writer Queries, or it deadlocks until the context times out.
func WithTx(ctx context.Context, w *sql.DB, fn func(*db.Queries) error) error {
	tx, err := w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(db.New(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

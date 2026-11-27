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

type DB struct {
	Writer *sql.DB
	Reader *sql.DB
}

func Open() (*DB, error) {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./data/morgenblau.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	base := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)&_pragma=synchronous(full)",
		path,
	)

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

func (d *DB) Close() error {
	err := d.Writer.Close()
	if rerr := d.Reader.Close(); err == nil {
		err = rerr
	}
	return err
}

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

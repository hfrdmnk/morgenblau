package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"morgenblau/internal/database"
)

func TestCloseDatabaseWaitsForCancelledWorker(t *testing.T) {
	workerCtx, cancel := context.WithCancel(context.Background())
	srv := &Server{gcCancel: cancel, db: shutdownTestDB(t)}
	cancelled := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	srv.gcWG.Go(func() {
		<-workerCtx.Done()
		close(cancelled)
		<-release
		close(finished)
	})
	t.Cleanup(func() {
		cancel()
		close(release)
		srv.gcWG.Wait()
	})
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- srv.closeDatabase(ctx) }()
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("shutdown did not cancel the worker")
	}
	select {
	case err := <-result:
		t.Fatalf("shutdown returned before the worker finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := srv.db.Writer.PingContext(ctx); err != nil {
		t.Fatalf("storage closed while a worker still owns it: %v", err)
	}
	release <- struct{}{}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown success must prove the worker finished")
	}
	if err := srv.db.Writer.PingContext(ctx); err == nil {
		t.Fatal("storage stayed open after every worker drained")
	}
}

func TestCloseDatabaseDeadlineKeepsStorageOpen(t *testing.T) {
	workerCtx, cancel := context.WithCancel(context.Background())
	srv := &Server{gcCancel: cancel, db: shutdownTestDB(t)}
	release := make(chan struct{})
	srv.gcWG.Go(func() { <-release })
	t.Cleanup(func() { close(release); srv.gcWG.Wait() })
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if err := srv.closeDatabase(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown error = %v; do not report drained workers after the deadline", err)
	}
	if workerCtx.Err() == nil {
		t.Fatal("shutdown deadline must not leave workers uncancelled")
	}
	if err := srv.db.Writer.Ping(); err != nil {
		t.Fatalf("deadline closed storage underneath an active worker: %v", err)
	}
}

func shutdownTestDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "shutdown.db"))
	db, err := database.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

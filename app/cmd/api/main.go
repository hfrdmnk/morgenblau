package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"morgenblau/internal/server"
)

const shutdownDrainTimeout = 30 * time.Second

func gracefulShutdown(apiServer *http.Server, cleanup func(context.Context) error, done chan bool) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	slog.Info("shutting down gracefully, press Ctrl+C again to force")
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "err", err)
	}

	if err := cleanup(ctx); err != nil {
		slog.Error("cleanup", "err", err)
	}

	slog.Info("server exiting")

	done <- true
}

func main() {

	srv, cleanup, err := server.NewServer()
	if err != nil {
		slog.Error("server bootstrap", "err", err)
		os.Exit(1)
	}

	done := make(chan bool, 1)

	go gracefulShutdown(srv, cleanup, done)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server error", "err", err)
		os.Exit(1)
	}

	<-done
	slog.Info("graceful shutdown complete")
}

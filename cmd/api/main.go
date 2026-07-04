package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"morgenblau/internal/server"
)

func gracefulShutdown(apiServer *http.Server, cleanup func(context.Context) error, done chan bool) {
	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Listen for the interrupt signal.
	<-ctx.Done()

	log.Println("shutting down gracefully, press Ctrl+C again to force")
	stop() // Allow Ctrl+C to force shutdown

	// 30s gives in-flight HTTP requests + the sync orchestrator's background
	// goroutines room to drain (sync_user can hold writes mid-flight).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown with error: %v", err)
	}

	// Drain in-flight sync writes and close the DB. http.Server.Shutdown doesn't
	// wait for RegisterOnShutdown funcs, so the drain has to run here explicitly,
	// blocking before we signal done.
	if err := cleanup(ctx); err != nil {
		log.Printf("Cleanup error: %v", err)
	}

	log.Println("Server exiting")

	// Notify the main goroutine that the shutdown is complete
	done <- true
}

func main() {

	srv, cleanup, err := server.NewServer()
	if err != nil {
		log.Fatalf("server bootstrap: %v", err)
	}

	// Create a done channel to signal when the shutdown is complete
	done := make(chan bool, 1)

	// Run graceful shutdown in a separate goroutine
	go gracefulShutdown(srv, cleanup, done)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server error: %v", err)
	}

	// Wait for the graceful shutdown to complete
	<-done
	log.Println("Graceful shutdown complete.")
}

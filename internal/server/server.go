package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database"
	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/jobs"
	"morgenblau/internal/oauth/config"
	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/oauth/store"
	"morgenblau/internal/safehttp"
	internalsync "morgenblau/internal/sync"
)

type Server struct {
	port int

	db         *sql.DB
	queries    *dbqueries.Queries
	oauthCfg   *config.Config
	oauthApp   *oauth.ClientApp
	store      *store.Store
	sealer     *cookie.Sealer
	profiles   *profiles.Cache
	jobs       *jobs.Tracker
	sync       *internalsync.Orchestrator
	fetcher    *fetcher.Fetcher
	feedfinder *feedfinder.Finder
	safeClient *http.Client

	gcCancel context.CancelFunc
}

func NewServer() (*http.Server, error) {
	port := 8000
	if raw := os.Getenv("PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT %q: %w", raw, err)
		}
		port = p
	}

	db, err := database.Open()
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	oauthCfg, err := config.FromOS()
	if err != nil {
		return nil, fmt.Errorf("load oauth config: %w", err)
	}

	sealer, err := loadCookieSealer()
	if err != nil {
		return nil, fmt.Errorf("load cookie sealer: %w", err)
	}

	st := store.New(db)
	oauthApp := oauth.NewClientApp(oauthCfg.Indigo, st)

	gcCtx, gcCancel := context.WithCancel(context.Background())
	go runAuthRequestGC(gcCtx, st)

	profileCache := profiles.New(oauthApp.Dir, profiles.PDSFetcher{})
	tracker := jobs.New()
	go runJobsGC(gcCtx, tracker)
	fetcherInst := fetcher.New()
	safeClient := safehttp.NewClient(30*time.Second, 5)
	finder := feedfinder.New(safeClient)
	queries := dbqueries.New(db)
	pipeline := internalsync.NewFeedPipeline(fetcherInst, queries)
	engine := internalsync.NewEngine(tracker, queries, internalsync.SessionPDSLister{}, pipeline, oauthApp)
	orchestrator := internalsync.New(tracker, pipeline, engine)

	NewServer := &Server{
		port:       port,
		db:         db,
		queries:    queries,
		oauthCfg:   oauthCfg,
		oauthApp:   oauthApp,
		store:      st,
		sealer:     sealer,
		profiles:   profileCache,
		jobs:       tracker,
		sync:       orchestrator,
		fetcher:    fetcherInst,
		feedfinder: finder,
		safeClient: safeClient,
		gcCancel:   gcCancel,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	server.RegisterOnShutdown(gcCancel)
	server.RegisterOnShutdown(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := orchestrator.Shutdown(shutdownCtx); err != nil {
			slog.Warn("sync orchestrator shutdown", "err", err)
		}
	})

	return server, nil
}

func loadCookieSealer() (*cookie.Sealer, error) {
	raw := os.Getenv("SESSION_COOKIE_KEY")
	if raw == "" {
		return nil, fmt.Errorf("SESSION_COOKIE_KEY is required (generate with: openssl rand -base64 32)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("SESSION_COOKIE_KEY base64 decode: %w", err)
	}
	return cookie.New(key)
}

// runJobsGC sweeps finished jobs past the retention window every minute so
// users who never poll /api/jobs/active don't leave ghosts behind.
func runJobsGC(ctx context.Context, tracker *jobs.Tracker) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tracker.GC()
		}
	}
}

// runAuthRequestGC sweeps stale oauth_auth_requests rows every 5 minutes.
// Stops when ctx is cancelled (on graceful shutdown).
func runAuthRequestGC(ctx context.Context, st *store.Store) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := st.GCExpired(ctx)
			if err != nil {
				slog.Warn("auth request GC", "err", err)
				continue
			}
			if n > 0 {
				slog.Debug("auth request GC", "deleted", n)
			}
		}
	}
}

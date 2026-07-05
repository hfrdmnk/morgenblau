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

	"morgenblau/internal/atidentity"
	"morgenblau/internal/atprepo"
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
	"morgenblau/internal/standardfeed"
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

// NewServer builds the HTTP server and returns a cleanup func the caller must
// invoke after http.Server.Shutdown returns: it drains in-flight sync writes and
// closes the database. The drain can't hang off server.RegisterOnShutdown because
// net/http fires those as detached goroutines it never waits for.
func NewServer() (*http.Server, func(context.Context) error, error) {
	port := 8000
	if raw := os.Getenv("PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid PORT %q: %w", raw, err)
		}
		port = p
	}

	fetchMinutes := 30
	if raw := os.Getenv("FETCH_INTERVAL_MINUTES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid FETCH_INTERVAL_MINUTES %q: %w", raw, err)
		}
		fetchMinutes = n
	}

	db, err := database.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	oauthCfg, err := config.FromOS()
	if err != nil {
		return nil, nil, fmt.Errorf("load oauth config: %w", err)
	}

	sealer, err := loadCookieSealer()
	if err != nil {
		return nil, nil, fmt.Errorf("load cookie sealer: %w", err)
	}

	st := store.New(db)
	oauthApp := oauth.NewClientApp(oauthCfg.Indigo, st)

	gcCtx, gcCancel := context.WithCancel(context.Background())
	go runAuthRequestGC(gcCtx, st)

	safeClient := safehttp.NewClient(30*time.Second, 5)
	identityDir := atidentity.Guarded(safeClient)
	profileCache := profiles.New(identityDir, profiles.PDSFetcher{Client: safeClient})
	tracker := jobs.New()
	go runJobsGC(gcCtx, tracker)
	fetcherInst := fetcher.New()
	queries := dbqueries.New(db)
	pipeline := internalsync.NewFeedPipeline(fetcherInst, queries)
	stdClient := standardfeed.NewClient(identityDir, safeClient)
	finder := feedfinder.New(safeClient).WithStandardResolver(stdClient)
	stdPipeline := internalsync.NewStandardfeedPipeline(stdClient, queries)
	router := internalsync.NewSourceRouter(pipeline, stdPipeline)
	engine := internalsync.NewEngine(tracker, queries, internalsync.SessionPDSLister{}, router, oauthApp, atprepo.SessionWriter{})
	orchestrator := internalsync.New(tracker, router, engine)

	if fetchMinutes > 0 {
		interval := time.Duration(fetchMinutes) * time.Minute
		refresher := internalsync.NewGlobalRefresher(queries, router)
		go runGlobalFetch(gcCtx, refresher, interval)
		slog.Info("global feed fetch enabled", "interval", interval)
	} else {
		slog.Info("global feed fetch disabled (FETCH_INTERVAL_MINUTES <= 0)")
	}

	srv := &Server{
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
		Addr:         fmt.Sprintf(":%d", srv.port),
		Handler:      srv.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	// Fire-and-forget is right for the GC/global-sweep tickers — they hold no
	// writes worth draining.
	server.RegisterOnShutdown(gcCancel)

	cleanup := func(ctx context.Context) error {
		if err := orchestrator.Shutdown(ctx); err != nil {
			slog.Warn("sync orchestrator shutdown", "err", err)
		}
		return db.Close()
	}

	return server, cleanup, nil
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

// runGlobalFetch re-fetches every feed in the shared catalog on a timer. It's
// not tied to any user, so it logs via slog rather than minting jobs.Tracker
// entries. Stops when ctx is cancelled (graceful shutdown).
func runGlobalFetch(ctx context.Context, r *internalsync.GlobalRefresher, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := r.RefreshAll(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("global feed fetch", "err", err)
				continue
			}
			slog.Info("global feed fetch", "feeds", n)
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

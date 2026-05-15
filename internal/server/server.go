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

	"morgenblau/internal/database"
	"morgenblau/internal/oauth/config"
	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/oauth/store"
)

type Server struct {
	port int

	db       *sql.DB
	oauthCfg *config.Config
	oauthApp *oauth.ClientApp
	store    *store.Store
	sealer   *cookie.Sealer

	gcCancel context.CancelFunc
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))

	db, err := database.Open()
	if err != nil {
		panic(fmt.Errorf("open database: %w", err))
	}

	oauthCfg, err := config.FromOS()
	if err != nil {
		panic(fmt.Errorf("load oauth config: %w", err))
	}

	sealer, err := loadCookieSealer()
	if err != nil {
		panic(fmt.Errorf("load cookie sealer: %w", err))
	}

	st := store.New(db)
	oauthApp := oauth.NewClientApp(oauthCfg.Indigo, st)

	gcCtx, gcCancel := context.WithCancel(context.Background())
	go runAuthRequestGC(gcCtx, st)

	NewServer := &Server{
		port:     port,
		db:       db,
		oauthCfg: oauthCfg,
		oauthApp: oauthApp,
		store:    st,
		sealer:   sealer,
		gcCancel: gcCancel,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", NewServer.port),
		Handler:      NewServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	server.RegisterOnShutdown(gcCancel)

	return server
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

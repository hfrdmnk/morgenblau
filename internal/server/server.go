package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"

	"morgenblau/internal/api"
	"morgenblau/internal/atidentity"
	"morgenblau/internal/atprepo"
	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database"
	dbqueries "morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverfavicon"
	"morgenblau/internal/discoveringest"
	"morgenblau/internal/discovermemo"
	"morgenblau/internal/discoverperson"
	"morgenblau/internal/discoverposts"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/jobs"
	"morgenblau/internal/leafletfeed"
	"morgenblau/internal/oauth/config"
	"morgenblau/internal/oauth/cookie"
	"morgenblau/internal/oauth/store"
	"morgenblau/internal/personsearch"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/secret"
	"morgenblau/internal/sharemeta"
	"morgenblau/internal/standardfeed"
	internalsync "morgenblau/internal/sync"
)

type Server struct {
	port int

	db                  *database.DB
	qr                  *dbqueries.Queries
	qw                  *dbqueries.Queries
	oauthCfg            *config.Config
	oauthApp            *oauth.ClientApp
	store               *store.Store
	sealer              *cookie.Sealer
	profiles            *profiles.Cache
	identityDir         identity.Directory
	jobs                *jobs.Tracker
	sync                *internalsync.Orchestrator
	fetcher             *fetcher.Fetcher
	feedfinder          *feedfinder.Finder
	safeClient          *http.Client
	discover            *discovercrawl.CachedCrawler
	discoverAuthored    *discovercrawl.CachedAuthoredCrawler
	discoverShares      *discovercrawl.CachedShareCrawler
	discoverAdjacent    *discovercrawl.CachedAdjacentFollowCrawler
	discoverOwnForeign  *discovercrawl.CachedOwnForeignCrawler
	discoverFollows     *discovercrawl.CachedReaderFollowCrawler
	discoverPosts       *discoverposts.CachedFetcher
	discoverFavicon     *discoverfavicon.Resolver
	discoverSourcesMemo *discovermemo.Cache[api.DiscoverSourcesPayload]
	discoverPeopleMemo  *discovermemo.Cache[api.DiscoverPeoplePayload]
	discoverMemos       *discovermemo.Group
	peopleSearcher      *personsearch.Searcher
	personInspector     *discoverperson.Inspector
	shareMetadata       *sharemeta.Resolver

	gcCancel context.CancelFunc
}

// NewServer returns a cleanup func the caller must run after Shutdown returns (draining sync writes, closing the DB); it can't hang off server.RegisterOnShutdown, since net/http fires those as unwaited detached goroutines.
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

	jetstreamURL := os.Getenv("JETSTREAM_URL")
	if jetstreamURL == "" {
		jetstreamURL = "wss://jetstream.us-east.bsky.network"
	}
	jetstreamAPIKey := os.Getenv("JETSTREAM_API_KEY")

	appviewHost := os.Getenv("APPVIEW_HOST")
	if appviewHost == "" {
		appviewHost = "https://public.api.bsky.app"
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

	keyset, err := loadSessionKeyset()
	if err != nil {
		return nil, nil, fmt.Errorf("load session keyset: %w", err)
	}

	// Writer pool: session store, pipelines, sync engine (all mutate). Reader pool: read-only handlers and refresher.
	qw := dbqueries.New(db.Writer)
	qr := dbqueries.New(db.Reader)

	st := store.New(db.Writer, keyset)

	safeClient := safehttp.NewClient(30*time.Second, 5)
	identityDir := atidentity.Guarded(safeClient)
	oauthApp := newOAuthApp(oauthCfg.Indigo, st, safeClient, identityDir)

	gcCtx, gcCancel := context.WithCancel(context.Background())
	go runAuthRequestGC(gcCtx, st)

	profileCache := profiles.New(identityDir, profiles.PDSFetcher{Client: safeClient})
	tracker := jobs.New()
	go runJobsGC(gcCtx, tracker)
	fetcherInst := fetcher.New()
	pipeline := internalsync.NewFeedPipeline(fetcherInst, qw).WithTxRunner(db.Writer)
	stdClient := standardfeed.NewClient(identityDir, safeClient)
	shareMetadataFetcher := sharemeta.NewFetcher(stdClient, safeClient)
	shareMetadata := sharemeta.NewResolver(qr, qr, shareMetadataFetcher, sharemeta.DefaultTTL).WithTxRunner(db.Writer)
	postsFetcher := discoverposts.NewFetcher(fetcherInst, stdClient).WithPublicationResolutions(qr)
	discoverPosts := discoverposts.NewCachedFetcher(postsFetcher, qr, discoverposts.DefaultTTL).WithTxRunner(db.Writer)
	discoverFavicon := discoverfavicon.NewResolver(qr, qr, qr, discoverfavicon.NewHTTPDiscoverer(), qr).WithTxRunner(db.Writer)
	finder := feedfinder.New(safeClient).WithStandardResolver(stdClient)
	stdPipeline := internalsync.NewStandardfeedPipeline(stdClient, qw).WithTxRunner(db.Writer)
	leafletClient := leafletfeed.NewClient(identityDir, safeClient)
	crawlClient := discovercrawl.NewClient(identityDir, safeClient, stdClient, stdClient, leafletClient).WithResolutionCache(qr, qw)
	discover := discovercrawl.NewCachedCrawler(crawlClient, qr, discovercrawl.DefaultTTL).WithTxRunner(db.Writer)
	discoverAuthored := discovercrawl.NewCachedAuthoredCrawler(crawlClient, qr, discovercrawl.DefaultTTL).WithTxRunner(db.Writer)
	discoverShares := discovercrawl.NewCachedShareCrawler(crawlClient, qr, discovercrawl.DefaultTTL).WithTxRunner(db.Writer)
	personInspector := discoverperson.New(discover, discoverAuthored, discoverShares)
	discoverFollows := discovercrawl.NewCachedReaderFollowCrawler(crawlClient, qr, discovercrawl.DefaultTTL).WithTxRunner(db.Writer)
	peopleSearcher := personsearch.NewSearcher(personsearch.NewAppView(appviewHost, safeClient), personsearch.NewSQLitePresenceReader(qr))
	// Same-user crawls (session user's own repo, not a followed person's) get a shorter TTL: staleness here would hide the viewer's own recent actions.
	discoverAdjacent := discovercrawl.NewCachedAdjacentFollowCrawler(crawlClient, qr, discovercrawl.SelfCrawlTTL).WithTxRunner(db.Writer)
	discoverOwnForeign := discovercrawl.NewCachedOwnForeignCrawler(crawlClient, qr, discovercrawl.SelfCrawlTTL).WithTxRunner(db.Writer)
	router := internalsync.NewSourceRouter(pipeline, stdPipeline)
	engine := internalsync.NewEngine(tracker, qw, internalsync.SessionPDSLister{}, router, oauthApp, atprepo.SessionWriter{}).WithLocker(st).WithTxRunner(db.Writer)
	orchestrator := internalsync.New(tracker, router, engine)

	if fetchMinutes > 0 {
		interval := time.Duration(fetchMinutes) * time.Minute
		refresher := internalsync.NewGlobalRefresher(qr, router)
		go runGlobalFetch(gcCtx, refresher, interval)
		slog.Info("global feed fetch enabled", "interval", interval)
	} else {
		slog.Info("global feed fetch disabled (FETCH_INTERVAL_MINUTES <= 0)")
	}

	// One assembled discover payload per user, short-lived; every local write that changes what they should see stales both.
	discoverSourcesMemo := discovermemo.New[api.DiscoverSourcesPayload](discovermemo.DefaultTTL)
	discoverPeopleMemo := discovermemo.New[api.DiscoverPeoplePayload](discovermemo.DefaultTTL)
	discoverMemos := discovermemo.NewGroup(discoverSourcesMemo, discoverPeopleMemo)

	// SPEC <discovery> Global/Trending: the ingest consumer mirrors the reader network's records straight off Jetstream, and the rebuild worker turns them into aggregates.
	discoverRepoFetcher := discoveringest.NewRepoFetcher(identityDir, safeClient)
	discoverIngest := discoveringest.NewConsumer(discoveringest.Config{URL: jetstreamURL, APIKey: jetstreamAPIKey}, qr, discoverRepoFetcher).WithTxRunner(db.Writer)
	discoverIngest.Start()
	discoverRebuild := discoveringest.NewRebuildWorker(qr, crawlClient, identityDir, qr).
		WithTxRunner(db.Writer).
		WithInvalidator(discoverMemos.InvalidateAll)
	discoverRebuild.Start()
	slog.Info("discover jetstream ingest enabled", "url", jetstreamURL)

	srv := &Server{
		port:                port,
		db:                  db,
		qr:                  qr,
		qw:                  qw,
		oauthCfg:            oauthCfg,
		oauthApp:            oauthApp,
		store:               st,
		sealer:              sealer,
		profiles:            profileCache,
		identityDir:         identityDir,
		jobs:                tracker,
		sync:                orchestrator,
		fetcher:             fetcherInst,
		feedfinder:          finder,
		safeClient:          safeClient,
		discover:            discover,
		discoverAuthored:    discoverAuthored,
		discoverShares:      discoverShares,
		discoverAdjacent:    discoverAdjacent,
		discoverOwnForeign:  discoverOwnForeign,
		discoverFollows:     discoverFollows,
		discoverPosts:       discoverPosts,
		discoverFavicon:     discoverFavicon,
		discoverSourcesMemo: discoverSourcesMemo,
		discoverPeopleMemo:  discoverPeopleMemo,
		discoverMemos:       discoverMemos,
		peopleSearcher:      peopleSearcher,
		personInspector:     personInspector,
		shareMetadata:       shareMetadata,
		gcCancel:            gcCancel,
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", srv.port),
		Handler:      srv.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	// Fire-and-forget is fine here: GC/global-sweep tickers hold no writes worth draining.
	server.RegisterOnShutdown(gcCancel)

	cleanup := func(ctx context.Context) error {
		if err := orchestrator.Shutdown(ctx); err != nil {
			slog.Warn("sync orchestrator shutdown", "err", err)
		}
		if err := discoverIngest.Shutdown(ctx); err != nil {
			slog.Warn("discover ingest consumer shutdown", "err", err)
		}
		if err := discoverRebuild.Shutdown(ctx); err != nil {
			slog.Warn("discover rebuild worker shutdown", "err", err)
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

// loadSessionKeyset reads SESSION_STORE_KEYS: comma-separated base64 32-byte keys (openssl rand -base64 32).
func loadSessionKeyset() (*secret.Keyset, error) {
	raw := os.Getenv("SESSION_STORE_KEYS")
	if raw == "" {
		return nil, fmt.Errorf("SESSION_STORE_KEYS is required (comma-separated base64 32-byte keys; generate with: openssl rand -base64 32)")
	}
	var keys [][]byte
	for i, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, err := base64.StdEncoding.DecodeString(part)
		if err != nil {
			return nil, fmt.Errorf("SESSION_STORE_KEYS[%d] base64 decode: %w", i, err)
		}
		keys = append(keys, key)
	}
	return secret.NewKeyset(keys...)
}

// runJobsGC sweeps finished jobs so users who never poll /api/jobs/active don't leave ghosts behind.
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

// runGlobalFetch isn't tied to a user, so it logs via slog instead of minting jobs.Tracker entries.
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

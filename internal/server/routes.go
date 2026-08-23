package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"morgenblau/internal/api"
	"morgenblau/internal/atprepo"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/handler"
)

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.healthHandler)
	mux.Handle("/oauth-client-metadata.json", handler.ClientMetadataHandler(s.oauthCfg))
	mux.Handle("/oauth-jwks.json", handler.JWKSHandler(s.oauthCfg))
	mux.Handle("POST /oauth/login", handler.LoginHandler(s.oauthApp))
	mux.Handle("GET /oauth/callback", handler.CallbackHandler(s.oauthApp, s.sealer, s.sync))
	mux.Handle("POST /oauth/logout", handler.LogoutHandler(s.oauthApp, s.sealer, s.store))
	mux.Handle("GET /api/profiles", api.ProfilesBatchHandler(s.profiles))
	mux.Handle("GET /api/profiles/me", api.MeProfileHandler(s.profiles))
	mux.Handle("GET /api/profiles/{did}", api.ProfileByDIDHandler(s.profiles))

	pdsWriter := atprepo.SessionWriter{}
	// POST/PATCH/DELETE routes below make PDS writes; auth.holdsSessionLock must match this set.
	mux.Handle("GET /api/subscriptions", api.SubscriptionsListHandler(s.qr))
	mux.Handle("POST /api/subscriptions/resolve", api.SubscriptionsResolveHandler(s.qr, s.feedfinder))
	mux.Handle("POST /api/subscriptions", api.SubscriptionsCreateHandler(s.qr, s.qw, pdsWriter, s.sync, s.discoverMemos))
	mux.Handle("GET /api/subscriptions/tags", api.SubscriptionsTagsHandler(s.qr))
	mux.Handle("GET /api/subscriptions/{rkey}", api.SubscriptionGetHandler(s.qr))
	mux.Handle("GET /api/subscriptions/{rkey}/entries", api.SubscriptionEntriesHandler(s.qr))
	mux.Handle("PATCH /api/subscriptions/{rkey}", api.SubscriptionsPatchHandler(s.qr, s.qw, pdsWriter, s.sync, s.discoverMemos))
	mux.Handle("DELETE /api/subscriptions/{rkey}", api.SubscriptionsDeleteHandler(s.qr, s.qw, pdsWriter, s.sync, s.discoverMemos))

	mux.Handle("GET /api/favicon", api.FaviconProxyHandler(s.qr, s.qr, s.qr, s.discoverFavicon, s.safeClient))

	mux.Handle("GET /api/saves", api.SavesListHandler(s.qr))
	mux.Handle("POST /api/saves", api.SavesCreateHandler(s.qr, s.qw, pdsWriter, s.sync))
	mux.Handle("DELETE /api/saves/{rkey}", api.SavesDeleteHandler(s.qr, s.qw, pdsWriter, s.sync))

	mux.Handle("GET /api/shares", api.SharesListHandler(s.qr, s.shareMetadata))
	mux.Handle("POST /api/shares", api.SharesCreateHandler(s.qr, s.qw, pdsWriter, s.sync))
	mux.Handle("DELETE /api/shares/{rkey}", api.SharesDeleteHandler(s.qr, s.qw, pdsWriter, s.sync))

	mux.Handle("GET /api/follows", api.FollowsListHandler(s.qr))
	mux.Handle("POST /api/follows", api.FollowsCreateHandler(s.qr, s.qw, pdsWriter, s.identityDir, s.sync, s.discoverMemos))
	mux.Handle("DELETE /api/follows/{rkey}", api.FollowsDeleteHandler(s.qr, s.qw, pdsWriter, s.sync, s.discoverMemos))

	mux.Handle("GET /api/discover/sources", api.DiscoverSourcesHandler(s.qr, s.discoverAdjacent, s.discoverOwnForeign, s.qr, s.discover, s.discoverAuthored, s.discoverShares, s.qr, s.qr, s.qr, s.qr, s.qr, s.discoverSourcesMemo))
	mux.Handle("GET /api/discover/people", api.DiscoverPeopleHandler(s.qr, s.discoverAdjacent, s.discoverFollows, s.qr, s.discover, s.discoverAuthored, s.discoverShares, s.qr, s.qr, s.qr, s.discoverPeopleMemo, s.qr))
	mux.Handle("POST /api/discover/hides", api.DiscoverHidesCreateHandler(s.qr, s.qw, s.discoverMemos))
	mux.Handle("GET /api/discover/sources/posts", api.DiscoverSourcePostsHandler(s.discoverPosts))
	mux.Handle("GET /api/discover/people/preview", api.DiscoverPersonPreviewHandler(s.personInspector, s.qr, s.shareMetadata))
	mux.Handle("GET /api/search/people", api.SearchPeopleHandler(s.peopleSearcher))

	mux.Handle("GET /api/profile/{id}", api.ProfileHandler(s.identityDir, s.profiles, s.qr, s.qr, s.personInspector))
	mux.Handle("GET /api/profile/{id}/{segment}", api.ProfileSegmentHandler(s.identityDir, s.qr, s.personInspector, s.shareMetadata))

	mux.Handle("GET /api/library/network-shares", api.LibraryNetworkSharesHandler(s.qr, s.discoverShares, s.shareMetadata))

	mux.Handle("GET /api/jobs/active", api.JobsActiveHandler(s.jobs))
	mux.Handle("GET /api/jobs/{id}", api.JobsGetHandler(s.jobs))
	mux.Handle("GET /api/digest", api.DigestHandler(s.qr, s.jobs))
	mux.Handle("POST /api/digest/refresh", api.DigestRefreshHandler(s.sync))
	mux.Handle("GET /api/entries/{slug}", api.EntryHandler(s.qr))
	mux.Handle("POST /api/entries/{slug}/extract", api.EntryExtractHandler(s.qr, s.qw, s.safeClient))

	mux.HandleFunc("/api/", http.NotFound)

	mux.Handle("GET /about", api.AboutHandler())
	mux.Handle("/", spaHandler())

	gate := auth.New(s.oauthApp, s.store, s.sealer)
	return s.corsMiddleware(gate(mux))
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "false")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	status := map[string]string{"status": "up"}
	if err := s.db.Reader.PingContext(ctx); err != nil {
		status["status"] = "down"
		status["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("health: failed to write response", "err", err)
	}
}

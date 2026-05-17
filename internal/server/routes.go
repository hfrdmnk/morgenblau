package server

import (
	"context"
	"encoding/json"
	"log"
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
	mux.Handle("POST /oauth/logout", handler.LogoutHandler(s.oauthApp, s.sealer))
	mux.Handle("GET /api/profiles/me", api.MeProfileHandler(s.profiles))
	mux.Handle("GET /api/profiles/{did}", api.ProfileByDIDHandler(s.profiles))

	pdsWriter := atprepo.SessionWriter{}
	mux.Handle("GET /api/subscriptions", api.SubscriptionsListHandler(s.queries))
	mux.Handle("POST /api/subscriptions/resolve", api.SubscriptionsResolveHandler(s.queries, s.feedfinder))
	mux.Handle("POST /api/subscriptions", api.SubscriptionsCreateHandler(s.queries, s.queries, pdsWriter, s.sync))
	mux.Handle("PATCH /api/subscriptions/{rkey}", api.SubscriptionsPatchHandler(s.queries, s.queries, pdsWriter))
	mux.Handle("DELETE /api/subscriptions/{rkey}", api.SubscriptionsDeleteHandler(s.queries, s.queries, pdsWriter))

	mux.Handle("GET /api/jobs/active", api.JobsActiveHandler(s.jobs))
	mux.Handle("GET /api/jobs/{id}", api.JobsGetHandler(s.jobs))
	mux.Handle("GET /api/digest", api.DigestHandler(s.queries, s.jobs))
	mux.Handle("POST /api/digest/refresh", api.DigestRefreshHandler(s.sync))
	mux.Handle("GET /api/entries/{slug}", api.EntryHandler(s.queries))
	mux.Handle("POST /api/entries/{slug}/extract", api.EntryExtractHandler(s.queries, s.queries, s.safeClient))

	// v1-deferred endpoints: stubbed to 501 so frontend callers fail loudly.
	stub := api.NotImplementedHandler()
	mux.Handle("/api/saved", stub)
	mux.Handle("/api/shares", stub)
	mux.Handle("/api/follows", stub)
	mux.Handle("/api/entries/{slug}/social", stub)

	mux.Handle("GET /about", api.AboutHandler())
	mux.Handle("/", spaHandler())

	gate := auth.New(s.oauthApp, s.store, s.sealer, s.routes)
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
	if err := s.db.PingContext(ctx); err != nil {
		status["status"] = "down"
		status["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

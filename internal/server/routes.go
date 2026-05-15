package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"morgenblau/internal/api"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/handler"
)

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.healthHandler)
	mux.Handle("/oauth-client-metadata.json", handler.ClientMetadataHandler(s.oauthCfg))
	mux.Handle("/oauth-jwks.json", handler.JWKSHandler(s.oauthCfg))
	mux.Handle("POST /oauth/login", handler.LoginHandler(s.oauthApp))
	mux.Handle("GET /oauth/callback", handler.CallbackHandler(s.oauthApp, s.sealer))
	mux.Handle("POST /oauth/logout", handler.LogoutHandler(s.oauthApp, s.sealer))
	mux.Handle("GET /api/me", api.MeHandler(s.oauthApp.Dir))
	mux.Handle("GET /api/subscriptions", api.SubscriptionsHandler(api.PDSLister{}))
	mux.Handle("/", spaHandler())

	gate := auth.New(s.oauthApp, s.sealer, s.routes)
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

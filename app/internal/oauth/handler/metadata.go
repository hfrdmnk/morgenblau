// Package handler exposes the OAuth HTTP handlers: metadata, JWKS, login, callback, and logout.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"morgenblau/internal/oauth/config"
)

// ClientMetadataHandler serves /oauth-client-metadata.json, advertising the JWKS URI when set.
func ClientMetadataHandler(cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		meta := cfg.Indigo.ClientMetadata()
		if cfg.JWKSURI != "" {
			uri := cfg.JWKSURI
			meta.JWKSURI = &uri
		}
		if cfg.ClientName != "" {
			name := cfg.ClientName
			meta.ClientName = &name
		}
		if cfg.ClientURI != "" {
			uri := cfg.ClientURI
			meta.ClientURI = &uri
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		if err := json.NewEncoder(w).Encode(meta); err != nil {
			slog.Error("encode client metadata", "err", err)
		}
	})
}

// JWKSHandler serves /oauth-jwks.json, the public half of the configured client signing key.
func JWKSHandler(cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		if err := json.NewEncoder(w).Encode(cfg.Indigo.PublicJWKS()); err != nil {
			slog.Error("encode jwks", "err", err)
		}
	})
}

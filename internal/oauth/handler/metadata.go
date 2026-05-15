// Package handler exposes the OAuth HTTP handlers (metadata, JWKS,
// login/callback/logout — the latter three land in slice 03).
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"morgenblau/internal/oauth/config"
)

// ClientMetadataHandler serves /oauth-client-metadata.json from the
// indigo-built ClientMetadata struct, advertising the JWKS URI in
// non-loopback mode.
func ClientMetadataHandler(cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		meta := cfg.Indigo.ClientMetadata()
		if cfg.JWKSURI != "" {
			uri := cfg.JWKSURI
			meta.JWKSURI = &uri
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		if err := json.NewEncoder(w).Encode(meta); err != nil {
			slog.Error("encode client metadata", "err", err)
		}
	})
}

// JWKSHandler serves /oauth-jwks.json from the public half of the
// configured client signing key.
func JWKSHandler(cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		if err := json.NewEncoder(w).Encode(cfg.Indigo.PublicJWKS()); err != nil {
			slog.Error("encode jwks", "err", err)
		}
	})
}

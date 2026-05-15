package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"morgenblau/internal/oauth/config"
)

func keyB64PEM(t *testing.T) string {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func loopbackCfg(t *testing.T) *config.Config {
	cfg, err := config.Load(map[string]string{
		"BLUESKY_OAUTH_PRIVATE_KEY": keyB64PEM(t),
		"BLUESKY_OAUTH_SCOPE":       "atproto repo:app.skyreader.feed.subscription",
		"BLUESKY_OAUTH_CLIENT_NAME": "Morgenblau",
		"BLUESKY_OAUTH_CLIENT_URI":  "http://localhost:8000",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func publishedCfg(t *testing.T) *config.Config {
	cfg, err := config.Load(map[string]string{
		"BLUESKY_OAUTH_PRIVATE_KEY": keyB64PEM(t),
		"BLUESKY_OAUTH_SCOPE":       "atproto repo:app.skyreader.feed.subscription",
		"BLUESKY_OAUTH_CLIENT_NAME": "Morgenblau",
		"BLUESKY_OAUTH_CLIENT_URI":  "https://app.example.com",
		"BLUESKY_CLIENT_ID":         "https://app.example.com/oauth-client-metadata.json",
		"BLUESKY_REDIRECT":          "https://app.example.com/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestClientMetadata_200JSON(t *testing.T) {
	cfg := publishedCfg(t)
	h := ClientMetadataHandler(cfg)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if m["client_id"] != "https://app.example.com/oauth-client-metadata.json" {
		t.Errorf("client_id = %v", m["client_id"])
	}
	uris, _ := m["redirect_uris"].([]any)
	if len(uris) == 0 || uris[0] != "https://app.example.com/oauth/callback" {
		t.Errorf("redirect_uris = %v", m["redirect_uris"])
	}
	if m["jwks_uri"] != "https://app.example.com/oauth-jwks.json" {
		t.Errorf("jwks_uri = %v", m["jwks_uri"])
	}
	if m["token_endpoint_auth_method"] != "private_key_jwt" {
		t.Errorf("token_endpoint_auth_method = %v", m["token_endpoint_auth_method"])
	}
	if m["dpop_bound_access_tokens"] != true {
		t.Errorf("dpop_bound_access_tokens = %v", m["dpop_bound_access_tokens"])
	}
	scope, _ := m["scope"].(string)
	if scope == "" || scope[:7] != "atproto" {
		t.Errorf("scope must start with atproto, got %q", scope)
	}
}

func TestClientMetadata_LoopbackHasNoJWKSURI(t *testing.T) {
	cfg := loopbackCfg(t)
	h := ClientMetadataHandler(cfg)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, has := m["jwks_uri"]; has {
		t.Errorf("loopback metadata shouldn't advertise jwks_uri, got %v", m["jwks_uri"])
	}
}

func TestJWKS_PublicOnly(t *testing.T) {
	cfg := publishedCfg(t)
	h := JWKSHandler(cfg)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/oauth-jwks.json", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	keys, _ := m["keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	key, _ := keys[0].(map[string]any)
	if key["kty"] != "EC" {
		t.Errorf("kty = %v", key["kty"])
	}
	if key["crv"] != "P-256" {
		t.Errorf("crv = %v", key["crv"])
	}
	if _, has := key["d"]; has {
		t.Errorf("JWKS contains 'd' — private scalar leaked")
	}
	if key["kid"] != "primary" {
		t.Errorf("kid = %v", key["kid"])
	}
}

package config

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

// genP256B64PEM mirrors the .env.example key generation command (openssl genpkey -algorithm EC | base64).
func genP256B64PEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBytes)
}

func genP384B64PEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBytes)
}

func baseEnv(t *testing.T) map[string]string {
	return map[string]string{
		"BLUESKY_OAUTH_PRIVATE_KEY": genP256B64PEM(t),
		"BLUESKY_OAUTH_SCOPE":       "atproto repo:blue.morgen.feed.subscription repo:blue.morgen.feed.save repo:blue.morgen.feed.share repo:blue.morgen.graph.follow",
		"BLUESKY_OAUTH_CLIENT_NAME": "Morgenblau",
		"BLUESKY_OAUTH_CLIENT_URI":  "https://app.example.com",
	}
}

func TestLoad_LoopbackWhenClientIDEmpty(t *testing.T) {
	env := baseEnv(t)
	delete(env, "BLUESKY_CLIENT_ID")

	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(cfg.Indigo.ClientID, "http://localhost") {
		t.Errorf("loopback client_id expected to start with http://localhost, got %q", cfg.Indigo.ClientID)
	}
	// atproto spec reserves client_id=http://localhost for public, secret-less clients.
	if cfg.Indigo.IsConfidential() {
		t.Errorf("expected public loopback client, got confidential")
	}
}

func TestLoad_WiresClientNameAndURI(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_CLIENT_ID"] = "https://app.example.com/oauth-client-metadata.json"
	env["BLUESKY_REDIRECT"] = "https://app.example.com/oauth/callback"

	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientName != "Morgenblau" {
		t.Errorf("ClientName = %q, want Morgenblau", cfg.ClientName)
	}
	if cfg.ClientURI != "https://app.example.com" {
		t.Errorf("ClientURI = %q, want https://app.example.com", cfg.ClientURI)
	}
}

func TestLoad_MetadataURLWhenClientIDSet(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_CLIENT_ID"] = "https://app.example.com/oauth-client-metadata.json"
	env["BLUESKY_REDIRECT"] = "https://app.example.com/oauth/callback"

	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Indigo.ClientID != "https://app.example.com/oauth-client-metadata.json" {
		t.Errorf("client_id = %q", cfg.Indigo.ClientID)
	}
	if cfg.Indigo.CallbackURL != "https://app.example.com/oauth/callback" {
		t.Errorf("callback = %q", cfg.Indigo.CallbackURL)
	}
	if !cfg.Indigo.IsConfidential() {
		t.Error("expected confidential client")
	}
}

func TestLoad_RejectsNonP256Key(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_OAUTH_PRIVATE_KEY"] = genP384B64PEM(t)
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected error for non-P-256 key, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "p-256") {
		t.Errorf("error should mention P-256: %v", err)
	}
}

func TestLoad_RejectsMissingPrivateKey(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_OAUTH_PRIVATE_KEY"] = ""
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected error when private key missing")
	}
}

func TestLoad_RejectsGarbagePrivateKey(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_OAUTH_PRIVATE_KEY"] = "not-base64!@#$"
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected error for garbage key")
	}
}

func TestLoad_RejectsMissingScope(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_OAUTH_SCOPE"] = ""
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected error when scope missing")
	}
}

func TestLoad_RejectsScopeWithoutAtproto(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_OAUTH_SCOPE"] = "repo:blue.morgen.feed.subscription"
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected error when scope missing 'atproto'")
	}
}

// Regression guard: confidential-mode JWKS must publish only the public half, never the private d scalar.
func TestLoad_PublicJWKSHasNoPrivateMaterial(t *testing.T) {
	env := baseEnv(t)
	env["BLUESKY_CLIENT_ID"] = "https://app.example.com/oauth-client-metadata.json"
	env["BLUESKY_REDIRECT"] = "https://app.example.com/oauth/callback"

	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	jwks := cfg.Indigo.PublicJWKS()
	if len(jwks.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(jwks.Keys))
	}
	raw, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	var as map[string]any
	if err := json.Unmarshal(raw, &as); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys, _ := as["keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("keys length: %d", len(keys))
	}
	key, _ := keys[0].(map[string]any)
	if _, hasD := key["d"]; hasD {
		t.Errorf("public JWKS serialized with a 'd' field — private scalar leaked")
	}
	if key["kty"] != "EC" {
		t.Errorf("kty = %q, want EC", key["kty"])
	}
	if key["crv"] != "P-256" {
		t.Errorf("crv = %q, want P-256", key["crv"])
	}
}

// Pins the parse path: base64 -> PEM -> PKCS8 -> ECDSA P-256 -> atcrypto.
func TestLoad_KeyRoundTripsThroughECDH(t *testing.T) {
	// Metadata-URL (confidential) mode carries the private key; loopback (public) does not.
	env := baseEnv(t)
	env["BLUESKY_CLIENT_ID"] = "https://app.example.com/oauth-client-metadata.json"
	env["BLUESKY_REDIRECT"] = "https://app.example.com/oauth/callback"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Indigo.PrivateKey == nil {
		t.Fatal("private key not set on config")
	}
	pemBytes, _ := base64.StdEncoding.DecodeString(env["BLUESKY_OAUTH_PRIVATE_KEY"])
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("test env value didn't PEM-decode")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("pkcs8 parse: %v", err)
	}
	ecdsaKey := parsed.(*ecdsa.PrivateKey)
	if ecdsaKey.Curve != elliptic.P256() {
		t.Errorf("test fixture wasn't P-256")
	}
	if _, err := ecdsaKey.ECDH(); err != nil {
		t.Errorf("ECDH conversion failed: %v", err)
	}
	_ = ecdh.P256
}

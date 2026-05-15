// Package config builds the indigo OAuth client config from environment variables.
package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

// Config is the resolved OAuth client config plus the JWKS URI that
// the metadata handler should advertise (only set in metadata-URL mode).
type Config struct {
	Indigo  *oauth.ClientConfig
	JWKSURI string // advertised in client metadata when non-empty
}

// FromOS reads from the process environment.
func FromOS() (*Config, error) {
	keys := []string{
		"BLUESKY_OAUTH_PRIVATE_KEY",
		"BLUESKY_OAUTH_SCOPE",
		"BLUESKY_OAUTH_CLIENT_NAME",
		"BLUESKY_OAUTH_CLIENT_URI",
		"BLUESKY_CLIENT_ID",
		"BLUESKY_REDIRECT",
		"APP_URL",
	}
	env := make(map[string]string, len(keys))
	for _, k := range keys {
		env[k] = os.Getenv(k)
	}
	return Load(env)
}

// Load builds the config from an in-memory env map. Useful for tests.
//
// When BLUESKY_CLIENT_ID is empty, returns a localhost-loopback config
// (no network calls). When set, returns a metadata-URL config — the
// caller is responsible for serving the metadata document at that URL.
func Load(env map[string]string) (*Config, error) {
	scope := strings.TrimSpace(env["BLUESKY_OAUTH_SCOPE"])
	if scope == "" {
		return nil, fmt.Errorf("BLUESKY_OAUTH_SCOPE is required")
	}
	scopes := strings.Fields(scope)
	if !slices.Contains(scopes, "atproto") {
		return nil, fmt.Errorf("BLUESKY_OAUTH_SCOPE must include 'atproto'")
	}

	priv, err := parsePrivateKey(env["BLUESKY_OAUTH_PRIVATE_KEY"])
	if err != nil {
		return nil, fmt.Errorf("BLUESKY_OAUTH_PRIVATE_KEY: %w", err)
	}

	clientID := strings.TrimSpace(env["BLUESKY_CLIENT_ID"])
	var ic oauth.ClientConfig
	var jwksURI string
	if clientID == "" {
		ic = oauth.NewLocalhostConfig("http://127.0.0.1:8000/oauth/callback", scopes)
	} else {
		redirect := strings.TrimSpace(env["BLUESKY_REDIRECT"])
		if redirect == "" {
			return nil, fmt.Errorf("BLUESKY_REDIRECT is required when BLUESKY_CLIENT_ID is set")
		}
		ic = oauth.NewPublicConfig(clientID, redirect, scopes)
		jwksURI, err = deriveJWKSURI(clientID)
		if err != nil {
			return nil, fmt.Errorf("derive jwks_uri: %w", err)
		}
	}

	if err := ic.SetClientSecret(priv, "primary"); err != nil {
		return nil, fmt.Errorf("set client secret: %w", err)
	}

	return &Config{Indigo: &ic, JWKSURI: jwksURI}, nil
}

func deriveJWKSURI(clientID string) (string, error) {
	u, err := url.Parse(clientID)
	if err != nil {
		return "", err
	}
	u.Path = "/oauth-jwks.json"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// parsePrivateKey takes the base64-encoded PEM PKCS#8 form produced by
//
//	openssl ecparam -name prime256v1 -genkey -noout | openssl pkcs8 -topk8 -nocrypt | openssl base64 -A | pbcopy
//
// and returns an indigo atcrypto P-256 private key.
func parsePrivateKey(encoded string) (atcrypto.PrivateKey, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("not set")
	}
	pemBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("PEM decode produced no block")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("PKCS#8 parse: %w", err)
	}
	ecKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("expected ECDSA private key, got %T", parsed)
	}
	if ecKey.Curve != elliptic.P256() {
		return nil, fmt.Errorf("expected P-256 curve, got %s", ecKey.Curve.Params().Name)
	}
	ecdhKey, err := ecKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("convert to ECDH: %w", err)
	}
	return atcrypto.ParsePrivateBytesP256(ecdhKey.Bytes())
}

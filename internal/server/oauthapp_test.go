package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"

	"morgenblau/internal/atidentity"
	"morgenblau/internal/safehttp"
)

// Guards against indigo's ClientApp defaulting to unguarded http.DefaultClient / DefaultDirectory (SSRF).
func TestNewOAuthApp_InstallsGuardedClientAndDir(t *testing.T) {
	cfg := oauth.NewLocalhostConfig("http://127.0.0.1:8000/oauth/callback", []string{"atproto"})
	client := safehttp.NewClient(30*time.Second, 5)
	dir := atidentity.Guarded(client)

	app := newOAuthApp(&cfg, nil, client, dir)

	if app.Client != client {
		t.Errorf("app.Client = %p, want guarded client %p; OAuth discovery must retain SSRF protection for attacker-influenced URLs", app.Client, client)
	}
	if app.Client == http.DefaultClient {
		t.Error("install the guarded OAuth client: http.DefaultClient can follow discovery URLs into private networks")
	}
	if app.Dir != dir {
		t.Error("install the guarded identity directory: handle and DID resolution must retain SSRF protection")
	}
}

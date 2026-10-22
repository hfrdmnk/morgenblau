package server

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
)

// newOAuthApp installs the SSRF-guarded client/directory; indigo's defaults (http.DefaultClient, identity.DefaultDirectory) are unguarded and would let attacker-supplied handles reach loopback/private addresses.
func newOAuthApp(cfg *oauth.ClientConfig, st oauth.ClientAuthStore, client *http.Client, dir identity.Directory) *oauth.ClientApp {
	app := oauth.NewClientApp(cfg, st)
	app.Client = client
	app.Dir = dir
	return app
}

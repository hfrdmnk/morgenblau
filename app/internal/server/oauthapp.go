package server

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
)

func newOAuthApp(cfg *oauth.ClientConfig, st oauth.ClientAuthStore, client *http.Client, dir identity.Directory) *oauth.ClientApp {
	app := oauth.NewClientApp(cfg, st)
	app.Client = client
	app.Dir = dir
	return app
}

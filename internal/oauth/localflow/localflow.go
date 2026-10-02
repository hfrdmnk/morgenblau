// Package localflow starts atproto OAuth for accounts on a throwaway loopback PDS, which indigo's StartAuthFlow refuses because its resolver accepts only public https hosts.
package localflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

// App is indigo's ClientApp with StartAuthFlow taking over for accounts whose PDS is PDS; ProcessCallback and Logout stay indigo's.
type App struct {
	*oauth.ClientApp
	PDS string
}

// StartAuthFlow mirrors indigo's StartAuthFlow after identity lookup, with the loopback-aware resolution below in place of its resolver.
func (a App) StartAuthFlow(ctx context.Context, identifier string) (string, error) {
	atid, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return a.ClientApp.StartAuthFlow(ctx, identifier)
	}
	ident, err := a.Dir.Lookup(ctx, atid)
	if err != nil || ident.PDSEndpoint() != a.PDS {
		return a.ClientApp.StartAuthFlow(ctx, identifier)
	}

	var resource oauth.ProtectedResourceMetadata
	if err := a.getJSON(ctx, a.PDS+"/.well-known/oauth-protected-resource", &resource); err != nil {
		return "", fmt.Errorf("fetching protected resource document: %w", err)
	}
	if len(resource.AuthorizationServers) == 0 || resource.AuthorizationServers[0] != a.PDS {
		return "", fmt.Errorf("local PDS %s names another authorization server: %v", a.PDS, resource.AuthorizationServers)
	}
	var meta oauth.AuthServerMetadata
	if err := a.getJSON(ctx, a.PDS+"/.well-known/oauth-authorization-server", &meta); err != nil {
		return "", fmt.Errorf("fetching auth server metadata: %w", err)
	}
	if err := validateAuthServer(meta, a.PDS); err != nil {
		return "", err
	}

	info, err := a.SendAuthRequest(ctx, &meta, a.Config.Scopes, identifier)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %w", err)
	}
	info.AccountDID = &ident.DID
	if err := a.Store.SaveAuthRequestInfo(ctx, *info); err != nil {
		return "", fmt.Errorf("saving auth request: %w", err)
	}
	params := url.Values{"client_id": {a.Config.ClientID}, "request_uri": {info.RequestURI}}
	return meta.AuthorizationEndpoint + "?" + params.Encode(), nil
}

func (a App) getJSON(ctx context.Context, docURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return err
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, docURL)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// validateAuthServer checks the parts of indigo's AuthServerMetadata.Validate that demand https against a loopback origin instead, then runs Validate itself on a copy moved to a placeholder https origin for everything else.
func validateAuthServer(meta oauth.AuthServerMetadata, serverURL string) error {
	iss, err := url.Parse(meta.Issuer)
	if err != nil || iss.Scheme != "http" || !safehttp.IsLoopbackHost(iss.Hostname()) || iss.User != nil || iss.Path != "" || iss.RawQuery != "" || iss.Fragment != "" {
		return fmt.Errorf("%w: issuer %q is not a loopback HTTP origin", oauth.ErrInvalidAuthServerMetadata, meta.Issuer)
	}
	if meta.Issuer != serverURL {
		return fmt.Errorf("%w: issuer must match request URL", oauth.ErrInvalidAuthServerMetadata)
	}
	authorize, err := url.Parse(meta.AuthorizationEndpoint)
	if err != nil || authorize.Scheme != iss.Scheme || authorize.Host != iss.Host || authorize.RawQuery != "" || authorize.Fragment != "" {
		return fmt.Errorf("%w: invalid auth endpoint URL: %s", oauth.ErrInvalidAuthServerMetadata, meta.AuthorizationEndpoint)
	}

	const placeholder = "https://issuer.invalid"
	meta.Issuer = placeholder
	meta.AuthorizationEndpoint = placeholder + authorize.Path
	return meta.Validate(placeholder)
}

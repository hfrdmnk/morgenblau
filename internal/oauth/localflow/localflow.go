// Package localflow starts atproto OAuth for accounts on a throwaway loopback PDS, which indigo's StartAuthFlow refuses because its resolver accepts only public https hosts.
package localflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

var (
	errOtherLoopbackPDS  = errors.New("the account's PDS runs on this machine but is not the configured local PDS")
	errForeignAuthServer = errors.New("the local PDS names an authorization server elsewhere")
)

// App is indigo's ClientApp with StartAuthFlow taking over for accounts on PDS, the run's loopback PDS; ProcessCallback and Logout stay indigo's.
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
	if err != nil {
		return a.ClientApp.StartAuthFlow(ctx, identifier)
	}
	_, accountPort, err := safehttp.LoopbackOrigin(ident.PDSEndpoint())
	if err != nil {
		return a.ClientApp.StartAuthFlow(ctx, identifier)
	}
	pds, pdsPort, err := safehttp.LoopbackOrigin(a.PDS)
	if err != nil {
		return "", fmt.Errorf("local PDS: %w", err)
	}
	if accountPort != pdsPort {
		return "", fmt.Errorf("%w: %s, not %s", errOtherLoopbackPDS, ident.PDSEndpoint(), pds)
	}

	var resource oauth.ProtectedResourceMetadata
	if err := a.getJSON(ctx, pds+"/.well-known/oauth-protected-resource", &resource); err != nil {
		return "", fmt.Errorf("fetching protected resource document: %w", err)
	}
	if len(resource.AuthorizationServers) == 0 {
		return "", fmt.Errorf("%w: none listed", errForeignAuthServer)
	}
	authServer, authPort, err := safehttp.LoopbackOrigin(resource.AuthorizationServers[0])
	if err != nil || authPort != pdsPort {
		return "", fmt.Errorf("%w: %s", errForeignAuthServer, resource.AuthorizationServers[0])
	}
	var meta oauth.AuthServerMetadata
	if err := a.getJSON(ctx, authServer+"/.well-known/oauth-authorization-server", &meta); err != nil {
		return "", fmt.Errorf("fetching auth server metadata: %w", err)
	}
	if err := validateAuthServer(meta, authServer); err != nil {
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
	_, issuerPort, err := safehttp.LoopbackOrigin(meta.Issuer)
	if err != nil {
		return fmt.Errorf("%w: issuer: %w", oauth.ErrInvalidAuthServerMetadata, err)
	}
	if _, serverPort, err := safehttp.LoopbackOrigin(serverURL); err != nil || issuerPort != serverPort {
		return fmt.Errorf("%w: issuer must match request URL", oauth.ErrInvalidAuthServerMetadata)
	}
	iss, _ := url.Parse(meta.Issuer)
	authorize, err := url.Parse(meta.AuthorizationEndpoint)
	if err != nil || authorize.Scheme != iss.Scheme || authorize.Host != iss.Host || authorize.RawQuery != "" || authorize.Fragment != "" {
		return fmt.Errorf("%w: invalid auth endpoint URL: %s", oauth.ErrInvalidAuthServerMetadata, meta.AuthorizationEndpoint)
	}

	for _, endpoint := range []struct {
		name string
		url  string
	}{
		{"PAR", meta.PushedAuthorizationRequestEndpoint},
		{"token", meta.TokenEndpoint},
		{"revocation", meta.RevocationEndpoint},
	} {
		if endpoint.name == "revocation" && endpoint.url == "" {
			continue
		}
		u, err := url.Parse(endpoint.url)
		if err != nil {
			return fmt.Errorf("%w: invalid %s endpoint URL", oauth.ErrInvalidAuthServerMetadata, endpoint.name)
		}
		u.Path, u.RawPath, u.RawQuery, u.ForceQuery = "", "", "", false
		if _, port, err := safehttp.LoopbackOrigin(u.String()); err != nil || port != issuerPort {
			return fmt.Errorf("%w: %s endpoint must be on the issuer's loopback port", oauth.ErrInvalidAuthServerMetadata, endpoint.name)
		}
	}

	const placeholder = "https://issuer.invalid"
	meta.Issuer = placeholder
	meta.AuthorizationEndpoint = placeholder + authorize.Path
	return meta.Validate(placeholder)
}

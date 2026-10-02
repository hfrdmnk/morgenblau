package localflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	localDID  = syntax.DID("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa")
	remoteDID = syntax.DID("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
)

func metadata(issuer string) oauth.AuthServerMetadata {
	required := true
	return oauth.AuthServerMetadata{
		Issuer:                                     issuer,
		AuthorizationEndpoint:                      issuer + "/oauth/authorize",
		TokenEndpoint:                              issuer + "/oauth/token",
		PushedAuthorizationRequestEndpoint:         issuer + "/oauth/par",
		RevocationEndpoint:                         issuer + "/oauth/revoke",
		ResponseTypesSupported:                     []string{"code"},
		GrantTypesSupported:                        []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:              []string{"S256"},
		TokenEndpointAuthMethodsSupoorted:          []string{"none", "private_key_jwt"},
		TokenEndpointAuthSigningAlgValuesSupported: []string{"ES256"},
		ScopesSupported:                            []string{"atproto"},
		AuthorizationReponseISSParameterSupported:  true,
		RequirePushedAuthorizationRequests:         true,
		DPoPSigningAlgValuesSupported:              []string{"ES256"},
		RequireRequestURIRegistration:              &required,
		ClientIDMetadataDocumentSupported:          true,
	}
}

// fakePDS is a loopback PDS that is its own authorization server, as the dev-env PDS is.
type fakePDS struct {
	*httptest.Server
	meta  func(issuer string) oauth.AuthServerMetadata
	hits  atomic.Int32
	state atomic.Value
}

func newFakePDS(t *testing.T) *fakePDS {
	t.Helper()
	f := &fakePDS{meta: metadata}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": f.URL, "authorization_servers": []string{f.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(f.meta(f.URL))
		case "/oauth/par":
			if err := r.ParseForm(); err != nil || r.Header.Get("DPoP") == "" || r.PostForm.Get("code_challenge_method") != "S256" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.state.Store(r.PostForm.Get("state"))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"request_uri":"urn:ietf:params:oauth:request_uri:req-example","expires_in":299}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func newApp(t *testing.T, pds *fakePDS) (App, *oauth.MemStore) {
	t.Helper()
	cfg := oauth.NewLocalhostConfig("http://127.0.0.1:8123/oauth/callback", []string{"atproto"})
	store := oauth.NewMemStore()
	app := oauth.NewClientApp(&cfg, store)
	app.Client = pds.Client()
	dir := identity.NewMockDirectory()
	dir.Insert(identity.Identity{
		DID: localDID, Handle: "reader.test", AlsoKnownAs: []string{"at://reader.test"},
		Services: map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: pds.URL}},
	})
	dir.Insert(identity.Identity{
		DID: remoteDID, Handle: "writer.example.com", AlsoKnownAs: []string{"at://writer.example.com"},
		Services: map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: "http://pds.example.com:2583"}},
	})
	app.Dir = dir
	return App{ClientApp: app, PDS: pds.URL}, store
}

func TestStartAuthFlow_LocalAccountGetsAnAuthorizeURLOnTheLocalPDS(t *testing.T) {
	pds := newFakePDS(t)
	app, store := newApp(t, pds)

	redirect, err := app.StartAuthFlow(context.Background(), "reader.test")
	if err != nil {
		t.Fatalf("StartAuthFlow: %v", err)
	}
	u, err := url.Parse(redirect)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if got := u.Scheme + "://" + u.Host + u.Path; got != pds.URL+"/oauth/authorize" {
		t.Errorf("authorize endpoint = %q, want %q", got, pds.URL+"/oauth/authorize")
	}
	if u.Query().Get("client_id") != app.Config.ClientID || u.Query().Get("request_uri") != "urn:ietf:params:oauth:request_uri:req-example" {
		t.Errorf("authorize query = %v", u.Query())
	}

	state, _ := pds.state.Load().(string)
	info, err := store.GetAuthRequestInfo(context.Background(), state)
	if err != nil {
		t.Fatalf("auth request info not saved: %v", err)
	}
	if info.AccountDID == nil || *info.AccountDID != localDID {
		t.Errorf("AccountDID = %v, want %s", info.AccountDID, localDID)
	}
	if info.AuthServerURL != pds.URL || info.AuthServerTokenEndpoint != pds.URL+"/oauth/token" {
		t.Errorf("auth server = %q token = %q", info.AuthServerURL, info.AuthServerTokenEndpoint)
	}
}

func TestStartAuthFlow_AccountsOffTheLocalPDSGoThroughIndigo(t *testing.T) {
	pds := newFakePDS(t)
	app, _ := newApp(t, pds)

	_, err := app.StartAuthFlow(context.Background(), "writer.example.com")
	if err == nil || !strings.Contains(err.Error(), "not a valid public host URL") {
		t.Errorf("err = %v, want indigo's public-host refusal", err)
	}
	if pds.hits.Load() != 0 {
		t.Errorf("local PDS hits = %d, want 0", pds.hits.Load())
	}
}

func TestStartAuthFlow_RefusesInvalidMetadata(t *testing.T) {
	pds := newFakePDS(t)
	pds.meta = func(issuer string) oauth.AuthServerMetadata {
		m := metadata(issuer)
		m.RequirePushedAuthorizationRequests = false
		return m
	}
	app, _ := newApp(t, pds)

	if _, err := app.StartAuthFlow(context.Background(), "reader.test"); !errors.Is(err, oauth.ErrInvalidAuthServerMetadata) {
		t.Errorf("err = %v, want ErrInvalidAuthServerMetadata", err)
	}
}

func TestValidateAuthServer(t *testing.T) {
	const issuer = "http://localhost:2701"
	for name, tc := range map[string]struct {
		mutate    func(*oauth.AuthServerMetadata)
		serverURL string
		ok        bool
	}{
		"dev-env shape":            {mutate: func(*oauth.AuthServerMetadata) {}, ok: true},
		"issuer from elsewhere":    {mutate: func(m *oauth.AuthServerMetadata) { m.Issuer = "http://localhost:2799" }},
		"issuer not loopback":      {mutate: func(m *oauth.AuthServerMetadata) { m.Issuer = "http://as.example.com" }, serverURL: "http://as.example.com"},
		"issuer with a path":       {mutate: func(m *oauth.AuthServerMetadata) { m.Issuer = issuer + "/as" }, serverURL: issuer + "/as"},
		"authorize off the issuer": {mutate: func(m *oauth.AuthServerMetadata) { m.AuthorizationEndpoint = "https://as.example.com/oauth/authorize" }},
		"authorize with a query":   {mutate: func(m *oauth.AuthServerMetadata) { m.AuthorizationEndpoint += "?x=1" }},
		"no S256":                  {mutate: func(m *oauth.AuthServerMetadata) { m.CodeChallengeMethodsSupported = []string{"plain"} }},
		"no ES256 DPoP":            {mutate: func(m *oauth.AuthServerMetadata) { m.DPoPSigningAlgValuesSupported = []string{"RS256"} }},
		"no refresh grant":         {mutate: func(m *oauth.AuthServerMetadata) { m.GrantTypesSupported = []string{"authorization_code"} }},
		"no iss parameter":         {mutate: func(m *oauth.AuthServerMetadata) { m.AuthorizationReponseISSParameterSupported = false }},
	} {
		m := metadata(issuer)
		tc.mutate(&m)
		serverURL := tc.serverURL
		if serverURL == "" {
			serverURL = issuer
		}
		err := validateAuthServer(m, serverURL)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok %v", name, err, tc.ok)
		}
		if err != nil && !errors.Is(err, oauth.ErrInvalidAuthServerMetadata) {
			t.Errorf("%s: err = %v, want ErrInvalidAuthServerMetadata", name, err)
		}
	}
}

package session

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
	"morgenblau/internal/safehttp"
)

// A colon cannot occur in Indigo's base64url OAuth session IDs.
const devSessionPrefix = "dev:"

type DevConfig struct {
	PDS, Handle, Password string
}

func LoadDevConfig(getenv func(string) string) (*DevConfig, error) {
	if getenv("APP_ENV") != "local" || getenv("DEV_LOGIN_ENABLED") != "true" {
		return nil, nil
	}
	cfg := &DevConfig{PDS: strings.TrimRight(getenv("ATPROTO_PDS"), "/"), Handle: getenv("ATPROTO_HANDLE"), Password: getenv("ATPROTO_PASSWORD")}
	u, err := url.Parse(cfg.PDS)
	httpsOrigin := err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == ""
	// A verify run's throwaway PDS listens on plain HTTP on this machine.
	if _, _, loopbackErr := safehttp.LoopbackOrigin(cfg.PDS); !httpsOrigin && loopbackErr != nil {
		return nil, errors.New("development login requires an HTTPS ATPROTO_PDS origin, or plain HTTP on loopback")
	}
	if _, err := syntax.ParseAtIdentifier(cfg.Handle); err != nil || cfg.Password == "" {
		return nil, errors.New("development login requires ATPROTO_HANDLE and ATPROTO_PASSWORD")
	}
	return cfg, nil
}

type OAuthApp interface {
	OAuthResumer
	Logout(context.Context, syntax.DID, string) error
}

type Manager struct {
	app    OAuthApp
	dev    *DevConfig
	client *http.Client
	mu     sync.Mutex
	active *Session
}

func NewManager(app OAuthApp, dev *DevConfig, client *http.Client) *Manager {
	return &Manager{app: app, dev: dev, client: client}
}

func (m *Manager) DevEnabled() bool { return m != nil && m.dev != nil }

func (m *Manager) LoginDev(ctx context.Context) (*Session, error) {
	if !m.DevEnabled() {
		return nil, errors.New("development login disabled")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil {
		var out any
		if err := m.active.APIClient().Get(ctx, "com.atproto.server.getSession", nil, &out); err == nil {
			return m.active, nil
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else {
			var apiErr *atclient.APIError
			if !errors.As(err, &apiErr) ||
				(apiErr.StatusCode != http.StatusBadRequest && apiErr.StatusCode != http.StatusUnauthorized) ||
				(apiErr.Name != "ExpiredToken" && apiErr.Name != "InvalidToken") {
				return nil, errors.New("could not validate development session")
			}
		}
	}
	client := atxrpc.New(m.dev.PDS, m.client)
	// A 307/308 redirect would forward the password-bearing request body.
	client.Client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	var result struct {
		DID     string `json:"did"`
		Access  string `json:"accessJwt"`
		Refresh string `json:"refreshJwt"`
	}
	// The library login helper cannot accept our guarded client for createSession.
	err := client.Post(ctx, "com.atproto.server.createSession", map[string]string{"identifier": m.dev.Handle, "password": m.dev.Password}, &result)
	if err != nil {
		return nil, errors.New("could not sign in to development PDS")
	}
	did, err := syntax.ParseDID(result.DID)
	if err != nil || result.Access == "" || result.Refresh == "" {
		return nil, errors.New("invalid development PDS session")
	}
	client.AccountDID = &did
	client.Auth = &atclient.PasswordAuth{Session: atclient.PasswordSessionData{AccountDID: did, Host: m.dev.PDS, AccessToken: result.Access, RefreshToken: result.Refresh}}
	m.active = NewPassword(client, devSessionPrefix+rand.Text())
	return m.active, nil
}

func (m *Manager) ResumeSession(ctx context.Context, did syntax.DID, sid string) (*Session, error) {
	if strings.HasPrefix(sid, devSessionPrefix) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.active == nil || m.active.Data.AccountDID != did || m.active.Data.SessionID != sid {
			return nil, errors.New("development session not found")
		}
		return m.active, nil
	}
	sess, err := m.app.ResumeSession(ctx, did, sid)
	if err != nil {
		return nil, err
	}
	return WrapOAuth(sess), nil
}

func (m *Manager) Logout(ctx context.Context, did syntax.DID, sid string) error {
	if !strings.HasPrefix(sid, devSessionPrefix) {
		return m.app.Logout(ctx, did, sid)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil || m.active.Data.AccountDID != did || m.active.Data.SessionID != sid {
		return nil
	}
	client := m.active.APIClient()
	m.active = nil
	return client.Auth.(*atclient.PasswordAuth).Logout(ctx, client.Client)
}

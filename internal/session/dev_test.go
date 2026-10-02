package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func TestDevConfigRequiresExplicitLocalOptIn(t *testing.T) {
	for _, env := range []string{"", "production", "local"} {
		for _, enabled := range []string{"", "false", "true"} {
			t.Run(env+"/"+enabled, func(t *testing.T) {
				values := map[string]string{"APP_ENV": env, "DEV_LOGIN_ENABLED": enabled, "ATPROTO_PDS": "https://pds.example.com", "ATPROTO_HANDLE": "reader.example", "ATPROTO_PASSWORD": "example-app-password"}
				cfg, err := LoadDevConfig(func(key string) string { return values[key] })
				if err != nil {
					t.Fatal(err)
				}
				if (cfg != nil) != (env == "local" && enabled == "true") {
					t.Fatalf("unexpected enabled state: %v", cfg != nil)
				}
			})
		}
	}
	for _, host := range []string{"", "http://pds.example.com", "https://user:password@pds.example.com", "https://pds.example.com/?token=secret"} {
		values := map[string]string{"APP_ENV": "local", "DEV_LOGIN_ENABLED": "true", "ATPROTO_PDS": host, "ATPROTO_HANDLE": "reader.example", "ATPROTO_PASSWORD": "example-app-password"}
		if _, err := LoadDevConfig(func(key string) string { return values[key] }); err == nil {
			t.Errorf("accepted invalid PDS")
		}
	}
	values := map[string]string{"APP_ENV": "local", "DEV_LOGIN_ENABLED": "true", "ATPROTO_PDS": "https://pds.example.com", "ATPROTO_HANDLE": "reader.example"}
	if _, err := LoadDevConfig(func(key string) string { return values[key] }); err == nil {
		t.Fatal("accepted missing password")
	}
}

func TestDevConfigAcceptsPlainHTTPOnlyForALoopbackPDS(t *testing.T) {
	for host, ok := range map[string]bool{
		"http://localhost:2583":      true,
		"http://127.0.0.1:2583":      true,
		"http://[::1]:2583":          true,
		"http://pds.example.com":     false,
		"http://10.0.0.1:2583":       false,
		"http://localhost.example":   false,
		"http://localhost:2583/xrpc": false,
	} {
		values := map[string]string{"APP_ENV": "local", "DEV_LOGIN_ENABLED": "true", "ATPROTO_PDS": host, "ATPROTO_HANDLE": "reader.test", "ATPROTO_PASSWORD": "example-app-password"}
		cfg, err := LoadDevConfig(func(key string) string { return values[key] })
		if (err == nil) != ok {
			t.Errorf("%s: err = %v, want accepted %v", host, err, ok)
		}
		if ok && (cfg == nil || cfg.PDS != host) {
			t.Errorf("%s: cfg = %+v", host, cfg)
		}
	}
}

func TestDevSessionLoginRefreshLogout(t *testing.T) {
	var creates, refreshes, deletes atomic.Int32
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			creates.Add(1)
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["identifier"] != "reader.example" || body["password"] != "example-app-password" {
				t.Error("wrong login credentials")
			}
			_, _ = w.Write([]byte(`{"did":"did:plc:abcdefghijklmnopqrstuvwx","accessJwt":"expired","refreshJwt":"refresh-one"}`))
		case "/xrpc/com.atproto.server.refreshSession":
			refreshes.Add(1)
			if r.Header.Get("Authorization") != "Bearer refresh-one" {
				t.Error("wrong refresh token")
			}
			_, _ = w.Write([]byte(`{"did":"did:plc:abcdefghijklmnopqrstuvwx","accessJwt":"fresh","refreshJwt":"refresh-two"}`))
		case "/xrpc/com.atproto.server.deleteSession":
			deletes.Add(1)
			if r.Header.Get("Authorization") != "Bearer refresh-two" {
				t.Error("logout must use rotated refresh token")
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			if r.Header.Get("Authorization") == "Bearer expired" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"ExpiredToken"}`))
				return
			}
			if r.Header.Get("Authorization") != "Bearer fresh" {
				t.Error("missing refreshed authentication")
			}
			_, _ = w.Write([]byte(`{"records":[]}`))
		}
	}))
	defer pds.Close()
	m := NewManager(nil, &DevConfig{PDS: pds.URL, Handle: "reader.example", Password: "example-app-password"}, pds.Client())
	ctx := context.Background()
	sess, err := m.LoginDev(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Data.Scopes) != 0 || !sess.IsPassword() {
		t.Fatal("password authority must not fabricate OAuth scopes")
	}
	if !strings.HasPrefix(sess.Data.SessionID, "dev:") {
		t.Fatal("development session ID must be outside the OAuth base64url namespace")
	}
	if resumed, err := m.ResumeSession(ctx, sess.Data.AccountDID, sess.Data.SessionID); err != nil || resumed != sess {
		t.Fatalf("development session did not resume: %v", err)
	}
	if _, err := m.ResumeSession(ctx, syntax.DID("did:plc:other"), sess.Data.SessionID); err == nil {
		t.Fatal("resumed as another DID")
	}
	if _, err := m.ResumeSession(ctx, sess.Data.AccountDID, sess.Data.SessionID+"wrong"); err == nil {
		t.Fatal("resumed with wrong session ID")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			var out any
			if err := sess.APIClient().Get(ctx, "com.atproto.repo.listRecords", nil, &out); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	again, err := m.LoginDev(ctx)
	if err != nil || again != sess {
		t.Fatalf("repeat login did not reuse session: %v", err)
	}
	if creates.Load() != 1 || refreshes.Load() != 1 {
		t.Fatalf("creates=%d refreshes=%d", creates.Load(), refreshes.Load())
	}
	if err := m.Logout(ctx, sess.Data.AccountDID, sess.Data.SessionID); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() != 1 {
		t.Fatal("logout did not revoke session")
	}
	if _, err := m.ResumeSession(ctx, sess.Data.AccountDID, sess.Data.SessionID); err == nil {
		t.Fatal("logged-out session resumed")
	}
	if _, err := NewManager(nil, nil, pds.Client()).LoginDev(ctx); err == nil {
		t.Fatal("disabled dev login succeeded")
	}
	if _, err := NewManager(nil, &DevConfig{}, pds.Client()).ResumeSession(ctx, sess.Data.AccountDID, sess.Data.SessionID); err == nil {
		t.Fatal("dev session survived restart")
	}
}

func TestDevLoginDoesNotForwardCredentialsOnRedirect(t *testing.T) {
	var leaked atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(true)
		w.WriteHeader(401)
	}))
	defer destination.Close()
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer pds.Close()
	m := NewManager(nil, &DevConfig{PDS: pds.URL, Handle: "reader.example", Password: "example-app-password"}, pds.Client())
	if _, err := m.LoginDev(context.Background()); err == nil {
		t.Fatal("redirect login succeeded")
	}
	if leaked.Load() {
		t.Fatal("forwarded credentials to redirect target")
	}
}

func TestDevLoginRecoversRevocationButNotTransientFailure(t *testing.T) {
	var status atomic.Int32
	var creates atomic.Int32
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			creates.Add(1)
			_, _ = w.Write([]byte(`{"did":"did:plc:abcdefghijklmnopqrstuvwx","accessJwt":"access","refreshJwt":"refresh"}`))
			return
		}
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"error":"InvalidToken"}`))
	}))
	defer pds.Close()
	m := NewManager(nil, &DevConfig{PDS: pds.URL, Handle: "reader.example", Password: "example-app-password"}, pds.Client())
	ctx := context.Background()
	original, err := m.LoginDev(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status.Store(500)
	if _, err := m.LoginDev(ctx); err == nil {
		t.Fatal("transient failure should not silently create a new session")
	}
	if creates.Load() != 1 {
		t.Fatal("created a session on transient failure")
	}
	status.Store(401)
	fresh, err := m.LoginDev(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Data.SessionID == original.Data.SessionID || creates.Load() != 2 {
		t.Fatal("revocation did not create a fresh session reference")
	}
	if _, err := m.ResumeSession(ctx, original.Data.AccountDID, original.Data.SessionID); err == nil {
		t.Fatal("old reference still valid")
	}
	if err := m.Logout(ctx, fresh.Data.AccountDID, fresh.Data.SessionID); err == nil {
		t.Fatal("expected upstream revocation failure")
	}
	if _, err := m.ResumeSession(ctx, fresh.Data.AccountDID, fresh.Data.SessionID); err == nil {
		t.Fatal("failed upstream logout retained local session")
	}
}

package session

import (
	"context"
	"errors"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

func TestWrapOAuthPreservesGrantedScopes(t *testing.T) {
	did := syntax.DID("did:plc:example")
	original := &oauth.ClientSession{Data: &oauth.ClientSessionData{
		AccountDID: did,
		SessionID:  "session-1",
		Scopes:     []string{"atproto", "repo:example.collection"},
	}}

	got := WrapOAuth(original)

	if got.Data.AccountDID != did || got.Data.SessionID != "session-1" {
		t.Fatalf("data = %#v", got.Data)
	}
	if len(got.Data.Scopes) != 2 || got.Data.Scopes[1] != "repo:example.collection" {
		t.Fatalf("scopes = %v", got.Data.Scopes)
	}
	if got.IsPassword() {
		t.Fatal("OAuth session reported password authority")
	}
}

func TestWrapOAuthPreservesNilAndEmptySessions(t *testing.T) {
	if got := WrapOAuth(nil); got != nil {
		t.Fatalf("nil OAuth session wrapped as %#v", got)
	}
	if got := WrapOAuth(&oauth.ClientSession{}); got == nil || got.Data != nil {
		t.Fatalf("empty OAuth session wrapped as %#v", got)
	}
}

func TestNewPasswordHasNoOAuthScopes(t *testing.T) {
	did := syntax.DID("did:plc:example")
	client := &atclient.APIClient{AccountDID: &did}

	got := NewPassword(client, "session-1")

	if !got.IsPassword() {
		t.Fatal("password session not reported as password authority")
	}
	if got.Data.AccountDID != did || got.Data.SessionID != "session-1" {
		t.Fatalf("data = %#v", got.Data)
	}
	if len(got.Data.Scopes) != 0 {
		t.Fatalf("password scopes = %v", got.Data.Scopes)
	}
	if got.APIClient() != client {
		t.Fatal("APIClient did not return password client")
	}
}

type failingOAuthResumer struct{ err error }

func (f failingOAuthResumer) ResumeSession(context.Context, syntax.DID, string) (*oauth.ClientSession, error) {
	return nil, f.err
}

func (f failingOAuthResumer) Logout(context.Context, syntax.DID, string) error {
	return f.err
}

func TestManagerPropagatesOAuthErrors(t *testing.T) {
	want := errors.New("resume failed")
	adapter := NewManager(failingOAuthResumer{err: want}, nil, nil)

	got, err := adapter.ResumeSession(context.Background(), syntax.DID("did:plc:example"), "session-1")

	if got != nil {
		t.Fatalf("session = %#v, want nil", got)
	}
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if err := adapter.Logout(context.Background(), syntax.DID("did:plc:example"), "session-1"); !errors.Is(err, want) {
		t.Fatalf("logout error = %v, want %v", err, want)
	}
}

func TestManagerRoutesOAuthIDWithDevDashPrefixToOAuth(t *testing.T) {
	const sid = "dev-abcdefghijklmnopqQ"
	did := syntax.DID("did:plc:abcdefghijklmnopqrstuvwx")
	want := errors.New("OAuth called")
	for _, tc := range []struct {
		name string
		dev  *DevConfig
	}{
		{"development disabled", nil},
		{"development enabled", &DevConfig{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewManager(failingOAuthResumer{err: want}, tc.dev, nil)
			if _, err := manager.ResumeSession(context.Background(), did, sid); !errors.Is(err, want) {
				t.Errorf("resume did not delegate to OAuth: %v", err)
			}
			if err := manager.Logout(context.Background(), did, sid); !errors.Is(err, want) {
				t.Errorf("logout did not delegate to OAuth: %v", err)
			}
		})
	}
}

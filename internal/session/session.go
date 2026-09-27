package session

import (
	"context"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type Data struct {
	AccountDID syntax.DID
	SessionID  string
	Scopes     []string
}

type Session struct {
	Data *Data

	client *atclient.APIClient
	oauth  *oauth.ClientSession
}

func WrapOAuth(sess *oauth.ClientSession) *Session {
	if sess == nil {
		return nil
	}
	wrapped := &Session{oauth: sess}
	if sess.Data != nil {
		wrapped.Data = &Data{
			AccountDID: sess.Data.AccountDID,
			SessionID:  sess.Data.SessionID,
			Scopes:     sess.Data.Scopes,
		}
	}
	return wrapped
}

func NewPassword(client *atclient.APIClient, sessionID string) *Session {
	data := &Data{SessionID: sessionID}
	if client != nil && client.AccountDID != nil {
		data.AccountDID = *client.AccountDID
	}
	return &Session{Data: data, client: client}
}

func (s *Session) IsPassword() bool {
	return s != nil && s.client != nil && s.oauth == nil
}

func (s *Session) APIClient() *atclient.APIClient {
	if s == nil {
		return nil
	}
	if s.oauth != nil {
		return s.oauth.APIClient()
	}
	return s.client
}

func (s *Session) RefreshTokens(ctx context.Context) error {
	if s == nil || s.oauth == nil {
		return nil
	}
	_, err := s.oauth.RefreshTokens(ctx)
	return err
}

type OAuthResumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error)
}

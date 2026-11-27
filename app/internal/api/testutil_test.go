package api

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"morgenblau/internal/session"

	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
)

func withSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &session.Session{
		Data: &session.Data{AccountDID: d, SessionID: sid},
	}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

func withStandardWriteSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &session.Session{
		Data: &session.Data{
			AccountDID: d,
			SessionID:  sid,
			Scopes:     []string{scopes.StandardSubscription},
		},
	}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

func ptrString(s string) *string { return &s }

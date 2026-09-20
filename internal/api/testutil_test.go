package api

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
)

// withSession injects a scope-less oauth.ClientSession (the pre-standardfeed grant shape) into the context, sparing tests a real OAuth dance.
func withSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &oauth.ClientSession{
		Data: &oauth.ClientSessionData{AccountDID: d, SessionID: sid},
	}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

// withStandardWriteSession is withSession plus the standard subscription write scope.
func withStandardWriteSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &oauth.ClientSession{
		Data: &oauth.ClientSessionData{
			AccountDID: d,
			SessionID:  sid,
			Scopes:     []string{scopes.StandardSubscription},
		},
	}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

func ptrString(s string) *string { return &s }

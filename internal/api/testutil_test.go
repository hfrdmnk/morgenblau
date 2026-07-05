package api

import (
	"net/http"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/oauth/scopes"
)

// withSession injects an oauth.ClientSession into the request context the
// same shape the auth middleware would inject. Used by handler tests so they
// don't need a real cookie / store / OAuth dance. The session carries no
// scopes — the grant shape of a session minted before the standardfeed
// scope change.
func withSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &oauth.ClientSession{
		Data: &oauth.ClientSessionData{AccountDID: d, SessionID: sid},
	}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

// withStandardWriteSession is withSession plus the site.standard.graph.*
// write scopes — a session minted after the scope change.
func withStandardWriteSession(req *http.Request, did string, sid string) *http.Request {
	d, _ := syntax.ParseDID(did)
	sess := &oauth.ClientSession{
		Data: &oauth.ClientSessionData{
			AccountDID: d,
			SessionID:  sid,
			Scopes:     []string{scopes.StandardSubscription, scopes.StandardRecommend},
		},
	}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

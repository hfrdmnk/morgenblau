// Package scopes inspects the granted OAuth scopes of a resumed session.
// Indigo never widens the scope grant on refresh, so a stale session keeps its old grant until re-auth.
package scopes

import (
	"slices"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

const StandardSubscription = "repo:site.standard.graph.subscription"

// HasStandardSubscriptionWrite reports whether the grant covers the standard subscription collection.
func HasStandardSubscriptionWrite(sess *oauth.ClientSession) bool {
	if sess == nil || sess.Data == nil {
		return false
	}
	return slices.Contains(sess.Data.Scopes, StandardSubscription)
}

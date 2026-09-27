// Package scopes inspects the granted OAuth scopes of a resumed session.
// Indigo never widens the scope grant on refresh, so a stale session keeps its old grant until re-auth.
package scopes

import (
	"slices"

	"morgenblau/internal/session"
)

const StandardSubscription = "repo:site.standard.graph.subscription"

// HasStandardSubscriptionWrite reports whether the grant covers the standard subscription collection.
func HasStandardSubscriptionWrite(sess *session.Session) bool {
	if sess == nil || sess.Data == nil {
		return false
	}
	// Password sessions have PDS-enforced authority rather than OAuth grants.
	if sess.IsPassword() {
		return true
	}
	return slices.Contains(sess.Data.Scopes, StandardSubscription)
}

// Package scopes inspects the granted OAuth scopes of a resumed session.
// Indigo never widens the scope grant on refresh, so a stale session keeps its old grant until re-auth.
package scopes

import (
	"slices"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

const (
	StandardSubscription = "repo:site.standard.graph.subscription"
	StandardRecommend    = "repo:site.standard.graph.recommend"
)

// HasStandardWrite reports whether the grant covers both site.standard.graph.*
// scopes; both are plain repo: grants, so a literal check suffices and nil/empty just reads as stale.
func HasStandardWrite(sess *oauth.ClientSession) bool {
	if sess == nil || sess.Data == nil {
		return false
	}
	granted := sess.Data.Scopes
	return slices.Contains(granted, StandardSubscription) && slices.Contains(granted, StandardRecommend)
}

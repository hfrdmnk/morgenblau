// Package scopes inspects the granted OAuth scopes of a resumed session.
// Indigo persists the token response's scope list on the session and never
// widens it on refresh, so a session minted before a scope change carries the
// old grant until the user re-authenticates.
package scopes

import (
	"slices"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

const (
	StandardSubscription = "repo:site.standard.graph.subscription"
	StandardRecommend    = "repo:site.standard.graph.recommend"
)

// HasStandardWrite reports whether the session's grant covers writing both
// site.standard.graph.* collections. The two scopes are plain repo: grants
// requested alongside the permission set, so a literal check suffices — no
// permission-set expansion involved. Nil or empty Scopes reads as stale
// (worst case: one extra re-auth prompt).
func HasStandardWrite(sess *oauth.ClientSession) bool {
	if sess == nil || sess.Data == nil {
		return false
	}
	granted := sess.Data.Scopes
	return slices.Contains(granted, StandardSubscription) && slices.Contains(granted, StandardRecommend)
}

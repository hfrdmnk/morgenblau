package scopes

import (
	"slices"

	"morgenblau/internal/session"
)

const StandardSubscription = "repo:site.standard.graph.subscription"

func HasStandardSubscriptionWrite(sess *session.Session) bool {
	if sess == nil || sess.Data == nil {
		return false
	}
	if sess.IsPassword() {
		return true
	}
	return slices.Contains(sess.Data.Scopes, StandardSubscription)
}

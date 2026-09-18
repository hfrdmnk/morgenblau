package scopes

import (
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
)

func sessionWith(scopes []string) *oauth.ClientSession {
	return &oauth.ClientSession{Data: &oauth.ClientSessionData{Scopes: scopes}}
}

func TestHasStandardSubscriptionWrite(t *testing.T) {
	cases := []struct {
		name string
		sess *oauth.ClientSession
		want bool
	}{
		{"subscription grant", sessionWith([]string{"atproto", "include:blue.morgen.access", StandardSubscription}), true},
		{"unrelated repo grant", sessionWith([]string{"atproto", "repo:site.standard.graph.recommend"}), false},
		{"pre-change grant", sessionWith([]string{"atproto", "include:blue.morgen.access"}), false},
		{"nil scopes", sessionWith(nil), false},
		{"empty string elements", sessionWith([]string{"", ""}), false},
		{"nil session", nil, false},
		{"nil data", &oauth.ClientSession{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasStandardSubscriptionWrite(tc.sess); got != tc.want {
				t.Fatalf("HasStandardSubscriptionWrite = %v, want %v", got, tc.want)
			}
		})
	}
}

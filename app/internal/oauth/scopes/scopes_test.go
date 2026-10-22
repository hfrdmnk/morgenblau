package scopes

import (
	"testing"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/session"
)

func sessionWith(scopes []string) *session.Session {
	return &session.Session{Data: &session.Data{Scopes: scopes}}
}

func TestHasStandardSubscriptionWrite(t *testing.T) {
	did := syntax.DID("did:plc:abcdefghijklmnopqrstuvwx")
	password := session.NewPassword(&atclient.APIClient{AccountDID: &did, Auth: &atclient.PasswordAuth{}}, "dev-example")
	cases := []struct {
		name string
		sess *session.Session
		want bool
	}{
		{"subscription grant", sessionWith([]string{"atproto", "include:blue.morgen.access", StandardSubscription}), true},
		{"unrelated repo grant", sessionWith([]string{"atproto", "repo:site.standard.graph.recommend"}), false},
		{"pre-change grant", sessionWith([]string{"atproto", "include:blue.morgen.access"}), false},
		{"nil scopes", sessionWith(nil), false},
		{"empty string elements", sessionWith([]string{"", ""}), false},
		{"nil session", nil, false},
		{"nil data", &session.Session{}, false},
		{"password authority", password, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasStandardSubscriptionWrite(tc.sess); got != tc.want {
				t.Fatalf("HasStandardSubscriptionWrite = %v, want %v", got, tc.want)
			}
		})
	}
}

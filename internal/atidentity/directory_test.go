package atidentity

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

var errSentinel = errors.New("sentinel: injected transport used")

type sentinelTransport struct{ called bool }

func (s *sentinelTransport) RoundTrip(*http.Request) (*http.Response, error) {
	s.called = true
	return nil, errSentinel
}

// Guarded must route identity HTTP fetches through the injected client; this pins the wiring, since falling back to an unguarded client would defeat the SSRF guard.
func TestGuarded_UsesInjectedClient(t *testing.T) {
	tr := &sentinelTransport{}
	dir := Guarded(&http.Client{Transport: tr})

	did, err := syntax.ParseDID("did:web:example.com")
	if err != nil {
		t.Fatalf("parse did: %v", err)
	}
	if _, err := dir.LookupDID(context.Background(), did); err == nil {
		t.Fatal("expected the sentinel transport to fail resolution")
	}
	if !tr.called {
		t.Fatal("identity resolution did not use the injected client")
	}
}

package atidentity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
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

// gatedDIDTransport holds the first fetch until the test cancels its caller, then answers like a server that saw the request through.
type gatedDIDTransport struct {
	first   sync.Once
	entered chan struct{}
	proceed chan struct{}
}

func (g *gatedDIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	g.first.Do(func() {
		close(g.entered)
		<-g.proceed
	})
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"did:web:example.com"}`)),
		Request:    req,
	}, nil
}

// A caller that gives up mid-lookup must not leave its cancellation cached as the identity's failure for the next caller.
func TestGuarded_CallerCancelDoesNotPoisonCache(t *testing.T) {
	tr := &gatedDIDTransport{entered: make(chan struct{}), proceed: make(chan struct{})}
	dir := Guarded(&http.Client{Transport: tr})
	did := syntax.DID("did:web:example.com")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = dir.LookupDID(ctx, did)
	}()
	<-tr.entered
	cancel()
	close(tr.proceed)
	<-done

	ident, err := dir.LookupDID(context.Background(), did)
	if err != nil {
		t.Fatalf("lookup after a cancelled caller: %v", err)
	}
	if ident.DID != did {
		t.Fatalf("DID = %s, want %s", ident.DID, did)
	}
}

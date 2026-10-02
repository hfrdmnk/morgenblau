package atidentity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
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

const (
	testDID    = syntax.DID("did:web:example.com")
	testHandle = syntax.Handle("user.example.com")
	didDocPath = "/.well-known/did.json"
	handlePath = "/.well-known/atproto-did"
)

type lookupFunc func(context.Context, identity.Directory) (*identity.Identity, error)

var (
	lookupDID lookupFunc = func(ctx context.Context, d identity.Directory) (*identity.Identity, error) {
		return d.LookupDID(ctx, testDID)
	}
	lookupHandle lookupFunc = func(ctx context.Context, d identity.Directory) (*identity.Identity, error) {
		return d.LookupHandle(ctx, testHandle)
	}
	lookupByDID lookupFunc = func(ctx context.Context, d identity.Directory) (*identity.Identity, error) {
		return d.Lookup(ctx, testDID.AtIdentifier())
	}
	lookupByHandle lookupFunc = func(ctx context.Context, d identity.Directory) (*identity.Identity, error) {
		return d.Lookup(ctx, testHandle.AtIdentifier())
	}
)

// gatedTransport serves testDID's document and testHandle's well-known, holding the first request to gatePath until the test has cancelled that caller.
type gatedTransport struct {
	gatePath string
	gate     sync.Once
	entered  chan struct{}
	proceed  chan struct{}
}

func (g *gatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == g.gatePath {
		g.gate.Do(func() {
			close(g.entered)
			<-g.proceed
		})
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	body := testDID.String()
	if req.URL.Path == didDocPath {
		body = `{"id":"did:web:example.com","alsoKnownAs":["at://user.example.com"]}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// A caller that gives up mid-lookup must not leave its cancellation cached as the identity's failure, or as an invalid handle, for the next caller.
func TestCached_CallerCancelDoesNotPoisonCache(t *testing.T) {
	tests := []struct {
		name     string
		gatePath string
		lookup   lookupFunc
	}{
		{"LookupDID during DID fetch", didDocPath, lookupDID},
		{"LookupDID during handle resolution", handlePath, lookupDID},
		{"LookupHandle during handle resolution", handlePath, lookupHandle},
		{"LookupHandle during DID fetch", didDocPath, lookupHandle},
		{"Lookup by DID during DID fetch", didDocPath, lookupByDID},
		{"Lookup by handle during handle resolution", handlePath, lookupByHandle},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &gatedTransport{gatePath: tc.gatePath, entered: make(chan struct{}), proceed: make(chan struct{})}
			dir := cached(&identity.BaseDirectory{
				HTTPClient:            http.Client{Transport: tr},
				SkipDNSDomainSuffixes: []string{".example.com"},
			})

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = tc.lookup(ctx, dir)
			}()
			<-tr.entered
			cancel()
			close(tr.proceed)
			<-done

			ident, err := tc.lookup(context.Background(), dir)
			if err != nil {
				t.Fatalf("lookup after a cancelled caller: %v", err)
			}
			if ident.Handle != testHandle {
				t.Fatalf("handle = %s, want %s", ident.Handle, testHandle)
			}
		})
	}
}

// ctxProbe records the context the inner directory receives.
type ctxProbe struct{ alive, deadline bool }

func (p *ctxProbe) record(ctx context.Context) (*identity.Identity, error) {
	p.alive = ctx.Err() == nil
	_, p.deadline = ctx.Deadline()
	return &identity.Identity{DID: testDID, Handle: testHandle}, nil
}

func (p *ctxProbe) LookupHandle(ctx context.Context, _ syntax.Handle) (*identity.Identity, error) {
	return p.record(ctx)
}

func (p *ctxProbe) LookupDID(ctx context.Context, _ syntax.DID) (*identity.Identity, error) {
	return p.record(ctx)
}

func (p *ctxProbe) Lookup(ctx context.Context, _ syntax.AtIdentifier) (*identity.Identity, error) {
	return p.record(ctx)
}

func (p *ctxProbe) Purge(context.Context, syntax.AtIdentifier) error { return nil }

// CacheDirectory never calls its inner Lookup, so only a direct call pins that method's detach.
func TestDetached_InnerContextOutlivesCallerWithDeadline(t *testing.T) {
	tests := []struct {
		name   string
		lookup lookupFunc
	}{
		{"LookupHandle", lookupHandle},
		{"LookupDID", lookupDID},
		{"Lookup", lookupByDID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe := &ctxProbe{}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := tc.lookup(ctx, detached{probe}); err != nil {
				t.Fatalf("lookup: %v", err)
			}
			if !probe.alive {
				t.Error("inner lookup saw the caller's cancellation")
			}
			if !probe.deadline {
				t.Error("inner lookup has no deadline")
			}
		})
	}
}

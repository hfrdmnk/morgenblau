package atxrpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/safehttp"
)

func TestNew_SetsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := New(srv.URL, nil)
	var out map[string]any
	if err := client.Get(context.Background(), syntax.NSID("com.atproto.repo.listRecords"), nil, &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotUA != safehttp.UserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, safehttp.UserAgent)
	}
}

func TestNew_NilHTTPClientWrapsDefaultTransport(t *testing.T) {
	client := New("https://example.test", nil)

	if client.Client == http.DefaultClient {
		t.Fatal("Client = http.DefaultClient, want a private clone so wrapping never mutates the shared default")
	}
	ct, ok := client.Client.Transport.(*cooldownTransport)
	if !ok {
		t.Fatalf("Transport = %T, want *cooldownTransport", client.Client.Transport)
	}
	if ct.inner != http.DefaultTransport {
		t.Errorf("inner transport = %v, want http.DefaultTransport", ct.inner)
	}
}

func TestNew_ClonesCallerClient(t *testing.T) {
	caller := &http.Client{Transport: &http.Transport{}, Timeout: 7 * time.Second}
	callerTransport := caller.Transport

	client := New("https://example.test", caller)

	if client.Client == caller {
		t.Fatal("Client is the caller's client, want a clone")
	}
	if caller.Transport != callerTransport {
		t.Error("caller's Transport was mutated, want it left alone")
	}
	if client.Client.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want the caller's 7s preserved", client.Client.Timeout)
	}
	ct, ok := client.Client.Transport.(*cooldownTransport)
	if !ok {
		t.Fatalf("Transport = %T, want *cooldownTransport", client.Client.Transport)
	}
	if ct.inner != callerTransport {
		t.Errorf("inner transport = %v, want the caller's transport", ct.inner)
	}
}

func TestNew_CooldownIsSharedAcrossClients(t *testing.T) {
	orig := defaultCooldown
	defaultCooldown = &HostCooldown{}
	t.Cleanup(func() { defaultCooldown = orig })

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	var out map[string]any
	listRecords := syntax.NSID("com.atproto.repo.listRecords")
	if err := New(srv.URL, nil).Get(context.Background(), listRecords, nil, &out); err == nil {
		t.Fatal("first Get succeeded, want the upstream 429 surfaced")
	}

	err := New(srv.URL, nil).Get(context.Background(), listRecords, nil, &out)
	if !IsHostCooling(err) {
		t.Fatalf("IsHostCooling(%v) = false, want a separately constructed client to share the cooldown", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("upstream hits = %d, want 1 (the second client must not reach the host)", got)
	}
}

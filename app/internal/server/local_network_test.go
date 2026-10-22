package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atidentity"
	"morgenblau/internal/fetcher"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/session"
)

func localEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadLocalNetwork_InertOutsideLocalOrWithoutPLC(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"production": {"APP_ENV": "production", "PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "http://127.0.0.1:2584"},
		"unset":      {"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "http://127.0.0.1:2584"},
		"no PLC_URL": {"APP_ENV": "local", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "http://127.0.0.1:2584"},
	} {
		n, err := loadLocalNetwork(localEnv(env))
		if err != nil || n != nil {
			t.Errorf("%s: loadLocalNetwork = %+v, %v; want nil, nil", name, n, err)
		}
	}
}

func TestLoadLocalNetwork_RejectsAnythingButLoopbackHTTPOrigins(t *testing.T) {
	for _, env := range []map[string]string{
		{"PLC_URL": "https://plc.example.com", "ATPROTO_PDS": "http://localhost:2583"},
		{"PLC_URL": "http://10.0.0.1:2582", "ATPROTO_PDS": "http://localhost:2583"},
		{"PLC_URL": "http://localhost", "ATPROTO_PDS": "http://localhost:2583"},
		{"PLC_URL": "http://localhost:2582/plc", "ATPROTO_PDS": "http://localhost:2583"},
		{"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "https://pds.example.com"},
		{"PLC_URL": "http://localhost:2582"},
		{"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "https://feeds.example.com"},
		{"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "http://10.0.0.1:2584"},
		{"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "http://127.0.0.1"},
		{"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": "http://127.0.0.1:2584/feeds"},
	} {
		env["APP_ENV"] = "local"
		if n, err := loadLocalNetwork(localEnv(env)); err == nil {
			t.Errorf("%v: accepted %+v", env, n)
		}
	}
}

// The client and directory NewServer builds from the local network must reach the throwaway PLC and PDS, and nothing else on loopback.
func TestLocalNetwork_ResolvesLocalAccountsThroughTheSafeClient(t *testing.T) {
	const did = "did:plc:aaaaaaaaaaaaaaaaaaaaaaaa"
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"did": did})
	}))
	defer pds.Close()
	plc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/") != did {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ident := identity.Identity{
			DID:         syntax.DID(did),
			AlsoKnownAs: []string{"at://reader.test"},
			Services:    map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: pds.URL}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ident.DIDDocument())
	}))
	defer plc.Close()
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer elsewhere.Close()

	n, err := loadLocalNetwork(localEnv(map[string]string{"APP_ENV": "local", "PLC_URL": plc.URL, "ATPROTO_PDS": pds.URL + "/"}))
	if err != nil || n == nil {
		t.Fatalf("loadLocalNetwork = %+v, %v", n, err)
	}
	client := safehttp.NewClient(5*time.Second, 5, safehttp.WithAllowLoopbackPorts(n.ports...))

	ident, err := atidentity.Local(client, n.plc, n.pds).LookupHandle(context.Background(), "reader.test")
	if err != nil {
		t.Fatalf("LookupHandle: %v", err)
	}
	if ident.DID != did || ident.PDSEndpoint() != pds.URL {
		t.Errorf("identity = %s at %s, want %s at %s", ident.DID, ident.PDSEndpoint(), did, pds.URL)
	}
	if _, err := client.Get(elsewhere.URL); !errors.Is(err, safehttp.ErrBlockedAddress) {
		t.Errorf("Get another loopback port err = %v, want ErrBlockedAddress", err)
	}
}

// The verify harness's fixture feed server is the one extra loopback origin feeds may come from: not other loopback ports, not through its redirects, and never outside a local network.
func TestLocalNetwork_FeedFixturesReachOnlyTheFixtureOrigin(t *testing.T) {
	feed := func(title string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>` + title + `</title><item><title>Item</title><guid>1</guid></item></channel></rss>`))
		}
	}
	decoy := httptest.NewServer(feed("Decoy"))
	defer decoy.Close()
	mux := http.NewServeMux()
	mux.Handle("/feed.xml", feed("Example Fixture Feed"))
	mux.Handle("/redirect.xml", http.RedirectHandler(decoy.URL+"/feed.xml", http.StatusFound))
	fixtures := httptest.NewServer(mux)
	defer fixtures.Close()

	env := map[string]string{"APP_ENV": "local", "PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583", "FEED_FIXTURE_ORIGIN": fixtures.URL}
	n, err := loadLocalNetwork(localEnv(env))
	if err != nil || n == nil {
		t.Fatalf("loadLocalNetwork = %+v, %v", n, err)
	}
	ctx := context.Background()
	f := fetcher.New(feedFetchOptions(n)...)
	got, err := f.Fetch(ctx, fixtures.URL+"/feed.xml", fetcher.FeedState{})
	if err != nil || got.Feed.Title != "Example Fixture Feed" {
		t.Fatalf("Fetch fixture = %+v, %v", got, err)
	}
	client := safehttp.NewClient(5*time.Second, 5, safehttp.WithAllowLoopbackPorts(n.ports...))
	resp, err := client.Get(fixtures.URL + "/feed.xml")
	if err != nil {
		t.Fatalf("safe client Get fixture: %v", err)
	}
	_ = resp.Body.Close()

	for name, target := range map[string]string{
		"another loopback port":         decoy.URL + "/feed.xml",
		"a redirect off the fixture":    fixtures.URL + "/redirect.xml",
		"a private address":             "http://10.0.0.1/feed.xml",
		"the link-local metadata space": "http://169.254.169.254/feed.xml",
	} {
		if _, err := f.Fetch(ctx, target, fetcher.FeedState{}); !errors.Is(err, safehttp.ErrBlockedAddress) {
			t.Errorf("fetcher, %s: err = %v, want ErrBlockedAddress", name, err)
		}
		if _, err := client.Get(target); !errors.Is(err, safehttp.ErrBlockedAddress) {
			t.Errorf("safe client, %s: err = %v, want ErrBlockedAddress", name, err)
		}
	}

	env["APP_ENV"] = "production"
	production, err := loadLocalNetwork(localEnv(env))
	if err != nil {
		t.Fatalf("loadLocalNetwork(production): %v", err)
	}
	if _, err := fetcher.New(feedFetchOptions(production)...).Fetch(ctx, fixtures.URL+"/feed.xml", fetcher.FeedState{}); !errors.Is(err, safehttp.ErrBlockedAddress) {
		t.Errorf("production fetcher reached the fixture origin: err = %v, want ErrBlockedAddress", err)
	}
}

// A loopback dev PDS is reachable only through the local network's port exception, so it must be that network's PDS.
func TestCheckDevPDS(t *testing.T) {
	local := &localNetwork{plc: "http://localhost:2700", pds: "http://localhost:2701", ports: []int{2700, 2701}}
	for name, tc := range map[string]struct {
		dev   *session.DevConfig
		local *localNetwork
		ok    bool
	}{
		"no dev login":             {ok: true},
		"https PDS":                {dev: &session.DevConfig{PDS: "https://pds.example.com"}, ok: true},
		"https PDS beside a run":   {dev: &session.DevConfig{PDS: "https://pds.example.com"}, local: local, ok: true},
		"the run's PDS":            {dev: &session.DevConfig{PDS: "http://127.0.0.1:2701"}, local: local, ok: true},
		"loopback without PLC_URL": {dev: &session.DevConfig{PDS: "http://localhost:2701"}},
		"loopback on another port": {dev: &session.DevConfig{PDS: "http://localhost:2799"}, local: local},
	} {
		if err := checkDevPDS(tc.dev, tc.local); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok %v", name, err, tc.ok)
		}
	}
}

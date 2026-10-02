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
	"morgenblau/internal/safehttp"
	"morgenblau/internal/session"
)

func localEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadLocalNetwork_InertOutsideLocalOrWithoutPLC(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"production": {"APP_ENV": "production", "PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583"},
		"unset":      {"PLC_URL": "http://localhost:2582", "ATPROTO_PDS": "http://localhost:2583"},
		"no PLC_URL": {"APP_ENV": "local", "ATPROTO_PDS": "http://localhost:2583"},
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
	} {
		env["APP_ENV"] = "local"
		if n, err := loadLocalNetwork(localEnv(env)); err == nil {
			t.Errorf("PLC_URL=%q ATPROTO_PDS=%q: accepted %+v", env["PLC_URL"], env["ATPROTO_PDS"], n)
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

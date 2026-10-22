package atidentity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	localDID     = syntax.DID("did:plc:aaaaaaaaaaaaaaaaaaaaaaaa")
	localHandle  = syntax.Handle("reader.test")
	publicDID    = syntax.DID("did:plc:bbbbbbbbbbbbbbbbbbbbbbbb")
	publicHandle = syntax.Handle("writer.example.com")
)

// localNetwork fakes a throwaway PLC and PDS: the PLC knows localDID, and the PDS resolves handles to whatever handles maps them to.
type localNetwork struct {
	plc, pds *httptest.Server
	handles  map[string]string
	plcHits  int
}

func newLocalNetwork(t *testing.T) *localNetwork {
	t.Helper()
	n := &localNetwork{handles: map[string]string{localHandle.String(): localDID.String()}}
	n.pds = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/xrpc/com.atproto.identity.resolveHandle" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		did, ok := n.handles[r.URL.Query().Get("handle")]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"HandleNotFound","message":"Unable to resolve handle"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"did": did})
	}))
	t.Cleanup(n.pds.Close)
	n.plc = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.plcHits++
		if strings.TrimPrefix(r.URL.Path, "/") != localDID.String() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ident := identity.Identity{
			DID:         localDID,
			AlsoKnownAs: []string{"at://" + localHandle.String()},
			Services:    map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: n.pds.URL}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ident.DIDDocument())
	}))
	t.Cleanup(n.plc.Close)
	return n
}

func (n *localNetwork) directory(public identity.Resolver) identity.Directory {
	return newLocalDirectory(public, n.plc.Client(), n.plc.URL, n.pds.URL)
}

func publicResolver() *identity.MockDirectory {
	public := identity.NewMockDirectory()
	public.Insert(identity.Identity{
		DID:         publicDID,
		Handle:      publicHandle,
		AlsoKnownAs: []string{"at://" + publicHandle.String()},
		Services:    map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: "https://pds.example.com"}},
	})
	return public
}

func TestLocal_LocalDIDResolvesOnTheLocalPLCAndVerifiesItsHandleThroughTheLocalPDS(t *testing.T) {
	n := newLocalNetwork(t)
	dir := n.directory(publicResolver())

	ident, err := dir.LookupDID(context.Background(), localDID)
	if err != nil {
		t.Fatalf("LookupDID: %v", err)
	}
	if ident.Handle != localHandle {
		t.Errorf("Handle = %q, want %q", ident.Handle, localHandle)
	}
	if ident.PDSEndpoint() != n.pds.URL {
		t.Errorf("PDSEndpoint = %q, want %q", ident.PDSEndpoint(), n.pds.URL)
	}

	byHandle, err := dir.Lookup(context.Background(), localHandle.AtIdentifier())
	if err != nil {
		t.Fatalf("Lookup handle: %v", err)
	}
	if byHandle.DID != localDID {
		t.Errorf("Lookup(%s).DID = %q, want %q", localHandle, byHandle.DID, localDID)
	}
}

func TestLocal_HandleTheLocalPDSMapsElsewhereIsInvalid(t *testing.T) {
	n := newLocalNetwork(t)
	n.handles[localHandle.String()] = publicDID.String()
	dir := n.directory(publicResolver())

	ident, err := dir.LookupDID(context.Background(), localDID)
	if err != nil {
		t.Fatalf("LookupDID: %v", err)
	}
	if !ident.Handle.IsInvalidHandle() {
		t.Errorf("Handle = %q, want handle.invalid", ident.Handle)
	}
	if _, err := dir.LookupHandle(context.Background(), localHandle); !errors.Is(err, identity.ErrHandleMismatch) {
		t.Errorf("LookupHandle err = %v, want ErrHandleMismatch", err)
	}
}

func TestLocal_UnknownTestHandleIsNotFound(t *testing.T) {
	n := newLocalNetwork(t)
	dir := n.directory(publicResolver())

	if _, err := dir.LookupHandle(context.Background(), syntax.Handle("nobody.test")); !errors.Is(err, identity.ErrHandleNotFound) {
		t.Errorf("LookupHandle err = %v, want ErrHandleNotFound", err)
	}
}

// Publications a reader subscribes to live on the real network, so the local PLC must not hide them.
func TestLocal_DIDUnknownToTheLocalPLCFallsBackToThePublicNetwork(t *testing.T) {
	n := newLocalNetwork(t)
	dir := n.directory(publicResolver())

	ident, err := dir.LookupDID(context.Background(), publicDID)
	if err != nil {
		t.Fatalf("LookupDID: %v", err)
	}
	if ident.Handle != publicHandle || ident.PDSEndpoint() != "https://pds.example.com" {
		t.Errorf("identity = %s at %s, want %s at https://pds.example.com", ident.Handle, ident.PDSEndpoint(), publicHandle)
	}
	if n.plcHits != 1 {
		t.Errorf("local PLC hits = %d, want 1 (asked first)", n.plcHits)
	}
}

func TestLocal_PublicHandleNeverReachesTheLocalPDS(t *testing.T) {
	n := newLocalNetwork(t)
	n.handles[publicHandle.String()] = localDID.String()
	dir := n.directory(publicResolver())

	ident, err := dir.LookupHandle(context.Background(), publicHandle)
	if err != nil {
		t.Fatalf("LookupHandle: %v", err)
	}
	if ident.DID != publicDID {
		t.Errorf("DID = %q, want the public network's %q", ident.DID, publicDID)
	}
}

func TestLocal_LocalPLCOutageIsAnErrorNotAFallback(t *testing.T) {
	n := newLocalNetwork(t)
	n.plc.Close()
	dir := n.directory(publicResolver())

	if _, err := dir.LookupDID(context.Background(), publicDID); err == nil {
		t.Fatal("LookupDID succeeded through the public network while the local PLC was down")
	}
}

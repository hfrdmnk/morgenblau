package atidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/atxrpc"
)

// localHandleTLD is reserved for testing (RFC 2606), so no public handle can live under it for the local PDS to shadow.
const localHandleTLD = "test"

// Local is Guarded for an APP_ENV=local server beside a throwaway PLC and PDS: did:plc lookups ask plcURL first and fall back to the public PLC on not-found, and .test handles resolve through the PDS at pdsURL.
func Local(client *http.Client, plcURL, pdsURL string) identity.Directory {
	return cached(newLocalDirectory(guardedBase(client), client, plcURL, pdsURL))
}

func newLocalDirectory(public identity.Resolver, client *http.Client, plcURL, pdsURL string) *localDirectory {
	return &localDirectory{
		public: public,
		plc:    &identity.BaseDirectory{PLCURL: plcURL, HTTPClient: *client, UserAgent: "morgenblau-identity"},
		pds:    atxrpc.New(pdsURL, client),
	}
}

// localDirectory repeats BaseDirectory's bidirectional checks, because BaseDirectory has no seam for routing a handle or DID to another resolver.
type localDirectory struct {
	public identity.Resolver
	plc    identity.Resolver
	pds    *atclient.APIClient
}

func (d *localDirectory) ResolveDID(ctx context.Context, did syntax.DID) (*identity.DIDDocument, error) {
	if did.Method() != "plc" {
		return d.public.ResolveDID(ctx, did)
	}
	doc, err := d.plc.ResolveDID(ctx, did)
	if errors.Is(err, identity.ErrDIDNotFound) {
		return d.public.ResolveDID(ctx, did)
	}
	return doc, err
}

func (d *localDirectory) ResolveHandle(ctx context.Context, h syntax.Handle) (syntax.DID, error) {
	h = h.Normalize()
	if h.TLD() != localHandleTLD {
		return d.public.ResolveHandle(ctx, h)
	}
	var out struct {
		DID string `json:"did"`
	}
	err := d.pds.Get(ctx, "com.atproto.identity.resolveHandle", map[string]any{"handle": h.String()}, &out)
	var apiErr *atclient.APIError
	if errors.As(err, &apiErr) && apiErr.Name == "HandleNotFound" {
		return "", fmt.Errorf("%w: %s on the local PDS", identity.ErrHandleNotFound, h)
	}
	if err != nil {
		return "", fmt.Errorf("%w: local PDS: %w", identity.ErrHandleResolutionFailed, err)
	}
	did, err := syntax.ParseDID(out.DID)
	if err != nil {
		return "", fmt.Errorf("%w: local PDS answered %q", identity.ErrHandleResolutionFailed, out.DID)
	}
	return did, nil
}

func (d *localDirectory) ResolveDIDRaw(ctx context.Context, did syntax.DID) (json.RawMessage, error) {
	doc, err := d.ResolveDID(ctx, did)
	if err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

func (d *localDirectory) LookupHandle(ctx context.Context, h syntax.Handle) (*identity.Identity, error) {
	h = h.Normalize()
	did, err := d.ResolveHandle(ctx, h)
	if err != nil {
		return nil, err
	}
	doc, err := d.ResolveDID(ctx, did)
	if err != nil {
		return nil, err
	}
	ident := identity.ParseIdentity(doc)
	declared, err := ident.DeclaredHandle()
	if err != nil {
		return nil, fmt.Errorf("could not verify handle/DID match: %w", err)
	}
	if declared != h {
		return nil, fmt.Errorf("%w: %s != %s", identity.ErrHandleMismatch, declared, h)
	}
	ident.Handle = declared
	return &ident, nil
}

func (d *localDirectory) LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error) {
	doc, err := d.ResolveDID(ctx, did)
	if err != nil {
		return nil, err
	}
	ident := identity.ParseIdentity(doc)
	ident.Handle = syntax.HandleInvalid
	declared, err := ident.DeclaredHandle()
	if errors.Is(err, identity.ErrHandleNotDeclared) {
		return &ident, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not parse handle from DID document: %w", err)
	}
	resolved, err := d.ResolveHandle(ctx, declared)
	switch {
	case errors.Is(err, identity.ErrHandleNotFound), errors.Is(err, identity.ErrHandleResolutionFailed):
	case err != nil:
		return nil, err
	case resolved == did:
		ident.Handle = declared
	}
	return &ident, nil
}

func (d *localDirectory) Lookup(ctx context.Context, atid syntax.AtIdentifier) (*identity.Identity, error) {
	if h, err := atid.AsHandle(); err == nil {
		return d.LookupHandle(ctx, h)
	}
	if did, err := atid.AsDID(); err == nil {
		return d.LookupDID(ctx, did)
	}
	return nil, errors.New("at-identifier neither a Handle nor a DID")
}

func (d *localDirectory) Purge(context.Context, syntax.AtIdentifier) error { return nil }

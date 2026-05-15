// Package api implements the authenticated HTTP handlers behind /api/*.
// All handlers expect the auth middleware to have already injected the
// *oauth.ClientSession into the request context.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/middleware/auth"
)

// Resolver is the slice of identity.Directory we depend on. Stubbed in tests.
type Resolver interface {
	LookupDID(ctx context.Context, did syntax.DID) (*identity.Identity, error)
}

// MeHandler returns {did, handle} for the session in the request context.
// Resolution failures (DNS, bidirectional verification, missing PDS) all
// collapse to 500 — the contract is "real handle present or fail", never
// fall back to displaying the DID.
//
// TODO: extend the response with `{avatar, displayName}` from the user's
// `app.bsky.actor.profile/self` record (com.atproto.repo.getRecord on the
// resolved PDS). Both fields nullable; absent record collapses to null,
// not an error. Avatar resolves through the PDS blob URL. Consumed by the
// avatar dropdown in the frontend WindowChrome.
func MeHandler(resolver Resolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := auth.SessionFromContext(r.Context())
		if sess == nil || sess.Data == nil {
			slog.Error("/api/me: no session in context (middleware bypassed?)")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		ident, err := resolver.LookupDID(r.Context(), sess.Data.AccountDID)
		if err != nil {
			slog.Warn("/api/me: identity resolution failed", "did", sess.Data.AccountDID, "err", err)
			http.Error(w, "could not resolve identity", http.StatusInternalServerError)
			return
		}
		if ident.Handle.IsInvalidHandle() {
			slog.Warn("/api/me: bidirectional handle verification failed", "did", sess.Data.AccountDID)
			http.Error(w, "could not resolve identity", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"did":    sess.Data.AccountDID.String(),
			"handle": ident.Handle.String(),
		})
	})
}

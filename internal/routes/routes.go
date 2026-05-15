// Package routes is the single source of truth for product (SPA) routes:
// path, whether the route is public or authed, and where to send an
// already-authed user who hits a public route (e.g. /login → /consume).
//
// Infrastructure paths (/api/health, /oauth/*, /assets/*) are not product
// routes and live in the auth middleware's hardcoded allowlist.
package routes

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Auth is the access requirement for a product route.
type Auth string

const (
	AuthPublic Auth = "public"
	AuthAuthed Auth = "authed"
)

// Route describes one product path.
type Route struct {
	Path           string `json:"path"`
	Auth           Auth   `json:"auth"`
	AuthedRedirect string `json:"authedRedirect,omitempty"`
}

//go:embed routes.json
var embedded []byte

// Load parses the embedded routes.json.
func Load() ([]Route, error) {
	return Parse(embedded)
}

// Parse decodes the given JSON bytes and validates invariants.
func Parse(data []byte) ([]Route, error) {
	var rs []Route
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("routes: invalid JSON: %w", err)
	}

	seen := make(map[string]struct{}, len(rs))
	for i, r := range rs {
		if r.Path == "" {
			return nil, fmt.Errorf("routes[%d]: empty path", i)
		}
		switch r.Auth {
		case AuthPublic, AuthAuthed:
		default:
			return nil, fmt.Errorf("routes[%d] (%s): invalid auth %q (want %q or %q)", i, r.Path, r.Auth, AuthPublic, AuthAuthed)
		}
		if r.Auth == AuthAuthed && r.AuthedRedirect != "" {
			return nil, fmt.Errorf("routes[%d] (%s): authedRedirect only valid on public routes", i, r.Path)
		}
		if _, dup := seen[r.Path]; dup {
			return nil, fmt.Errorf("routes[%d]: duplicate path %q", i, r.Path)
		}
		seen[r.Path] = struct{}{}
	}
	return rs, nil
}

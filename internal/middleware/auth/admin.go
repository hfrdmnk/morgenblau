package auth

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// Admins, unlike Allowlist, fails closed: an unset or empty ADMIN_DIDS admits nobody.
type Admins map[syntax.DID]struct{}

func LoadAdmins(getenv func(string) string) (Admins, error) {
	raw := getenv("ADMIN_DIDS")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	admins := make(Admins)
	for _, part := range strings.Split(raw, ",") {
		did, err := syntax.ParseDID(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("ADMIN_DIDS must be a comma-separated list of DIDs: %w", err)
		}
		admins[did] = struct{}{}
	}
	return admins, nil
}

func (a Admins) Includes(did syntax.DID) bool {
	_, ok := a[did]
	return ok
}

// IsAdminPath cleans first so a path ServeMux would redirect onto an admin route is gated too.
func IsAdminPath(p string) bool {
	p = path.Clean("/" + p)
	for _, prefix := range []string{"/admin", "/api/admin"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}

type adminKey struct{}

// WithAdmin is for the gate and tests only; TestOnlyTheGateMarksAdmins keeps every other caller out.
func WithAdmin(ctx context.Context) context.Context {
	return context.WithValue(ctx, adminKey{}, true)
}

// IsAdmin reports whether the gate admitted this request to an admin path.
func IsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(adminKey{}).(bool)
	return v
}

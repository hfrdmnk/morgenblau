package auth

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

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

func WithAdmin(ctx context.Context) context.Context {
	return context.WithValue(ctx, adminKey{}, true)
}

func IsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(adminKey{}).(bool)
	return v
}

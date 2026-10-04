package auth

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

// A nil Allowlist leaves admission unrestricted; an empty non-nil list denies everyone.
type Allowlist map[syntax.DID]struct{}

func LoadAllowlist(getenv func(string) string) (Allowlist, error) {
	raw := getenv("ALPHA_ENABLED")
	if raw == "" {
		return nil, nil
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("ALPHA_ENABLED: %w", err)
	}
	if !enabled {
		return nil, nil
	}
	allowed := make(Allowlist)
	for _, part := range strings.Split(getenv("ALPHA_ALLOWED_DIDS"), ",") {
		did, err := syntax.ParseDID(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("ALPHA_ALLOWED_DIDS must contain a non-empty comma-separated list of DIDs: %w", err)
		}
		allowed[did] = struct{}{}
	}
	return allowed, nil
}

func (a Allowlist) Allows(did syntax.DID) bool {
	if a == nil {
		return true
	}
	_, ok := a[did]
	return ok
}

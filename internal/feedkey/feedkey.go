// Package feedkey canonicalizes feed URLs only for comparison, never before a write: Tier-2 stores feed_url
// unnormalized (see internal/api/subscriptions_create.go). SPEC <discovery>, PRD module 4.
package feedkey

import (
	"net/url"
	"strings"
)

// Normalize returns raw unchanged if it doesn't parse as an absolute URL, rather than guessing at a canonical form.
func Normalize(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}

	u.Scheme = strings.ToLower(u.Scheme)

	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		u.Host = host + ":" + port
	} else {
		u.Host = host
	}

	u.Fragment = ""
	u.RawFragment = ""

	if strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
		u.RawPath = ""
	}

	return u.String()
}

// Kind derives a Tier-2 source kind from a canonical key's shape: at:// prefixed keys are standardfeed publication references, everything else is rss (SPEC <discovery>).
func Kind(key string) string {
	if strings.HasPrefix(key, "at://") {
		return "standardfeed"
	}
	return "rss"
}

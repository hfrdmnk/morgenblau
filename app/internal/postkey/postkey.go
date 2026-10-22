// Package postkey says when two links or two titles name the same post. Discovery, extraction and the stored feed all
// dedupe through it, so they never disagree about what counts as a new post.
package postkey

import (
	"net/url"
	"strings"
)

// URL keys a post's link: scheme, "www.", trailing slashes, the fragment and percent-encoding case do not make another post.
func URL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return FromURL(u)
}

// FromURL is URL for a link already parsed.
func FromURL(u *url.URL) string {
	path := (&url.URL{Path: u.Path}).EscapedPath()
	key := strings.TrimPrefix(strings.ToLower(u.Host), "www.") + strings.TrimRight(path, "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

// Title keys a post's title: case and whitespace do not make another title.
func Title(t string) string { return strings.ToLower(strings.Join(strings.Fields(t), " ")) }

// StripTracking drops query parameters that only say where a click came from, keeping the ones that pick content.
func StripTracking(u *url.URL) {
	if u.RawQuery == "" {
		return
	}
	q := u.Query()
	for k := range q {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "utm_") || lk == "ref" || lk == "fbclid" || lk == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
}

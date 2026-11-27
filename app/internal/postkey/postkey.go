package postkey

import (
	"net/url"
	"strings"
)

func URL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return FromURL(u)
}

func FromURL(u *url.URL) string {
	path := (&url.URL{Path: u.Path}).EscapedPath()
	key := strings.TrimPrefix(strings.ToLower(u.Host), "www.") + strings.TrimRight(path, "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

func Title(t string) string { return strings.ToLower(strings.Join(strings.Fields(t), " ")) }

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

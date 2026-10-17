// Package sitesection names the one-segment pages where sites keep their full list of posts, for setup and discovery.
package sitesection

import (
	"net/url"
	"strings"
)

var words = map[string]bool{
	"blog": true, "blogs": true, "writing": true, "writings": true, "posts": true, "post": true, "articles": true,
	"notes": true, "essays": true, "journal": true, "thoughts": true, "updates": true, "news": true, "changelog": true, "log": true,
}

// Name reports a one-segment path such as "blog" or "Writing" that sites use for their list of posts.
func Name(segment string) bool { return words[strings.ToLower(segment)] }

// SameSite reports whether a section link stays on the page's site: "www." aside, but another port is another origin.
func SameSite(a, b *url.URL) bool { return host(a) == host(b) }

func host(u *url.URL) string { return strings.TrimPrefix(strings.ToLower(u.Host), "www.") }

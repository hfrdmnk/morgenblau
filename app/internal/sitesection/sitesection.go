package sitesection

import (
	"net/url"
	"strings"
)

var words = map[string]bool{
	"blog": true, "blogs": true, "writing": true, "writings": true, "posts": true, "post": true, "articles": true,
	"notes": true, "essays": true, "journal": true, "thoughts": true, "updates": true, "news": true, "changelog": true, "log": true,
}

func Name(segment string) bool { return words[strings.ToLower(segment)] }

func SameSite(a, b *url.URL) bool { return host(a) == host(b) }

func host(u *url.URL) string { return strings.TrimPrefix(strings.ToLower(u.Host), "www.") }

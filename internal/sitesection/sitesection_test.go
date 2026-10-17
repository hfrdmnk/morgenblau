package sitesection

import (
	"net/url"
	"testing"
)

func TestName(t *testing.T) {
	for seg, want := range map[string]bool{"blog": true, "Writing": true, "UPDATES": true, "about": false, "blog/2024": false, "": false} {
		if got := Name(seg); got != want {
			t.Errorf("Name(%q) = %v, want %v", seg, got, want)
		}
	}
}

func TestSameSite(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://www.example.com/", "https://example.com/blog", true},
		{"https://Example.com/", "http://example.COM/blog", true},
		{"https://example.com:8443/", "https://example.com:8443/blog", true},
		{"https://example.com:8443/", "https://example.com/blog", false},
		{"https://example.com/", "https://blog.example.com/", false},
		{"https://example.com/", "https://other.example/", false},
	}
	for _, tc := range cases {
		a, _ := url.Parse(tc.a)
		b, _ := url.Parse(tc.b)
		if got := SameSite(a, b); got != tc.want {
			t.Errorf("SameSite(%s, %s) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

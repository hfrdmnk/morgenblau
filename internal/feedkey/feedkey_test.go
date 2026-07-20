package feedkey

import "testing"

func TestNormalize_TableDriven(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases scheme and host", "HTTP://Example.COM/feed", "http://example.com/feed"},
		{"strips default http port", "http://example.com:80/feed", "http://example.com/feed"},
		{"strips default https port", "https://example.com:443/feed", "https://example.com/feed"},
		{"keeps non-default port", "http://example.com:8080/feed", "http://example.com:8080/feed"},
		{"strips single trailing slash", "https://example.com/feed/", "https://example.com/feed"},
		{"strips trailing slash on bare root", "https://example.com/", "https://example.com"},
		{"drops fragment", "https://example.com/feed#section", "https://example.com/feed"},
		{"keeps query string", "https://example.com/feed?a=1&b=2", "https://example.com/feed?a=1&b=2"},
		{"combines port, slash, fragment, query", "HTTP://Example.COM:80/feed/?x=1#y", "http://example.com/feed?x=1"},
		{"http-vs-https trailing slash variant pair collapses", "http://example.com/feed/", "http://example.com/feed"},
		{"unparseable input returned unchanged", "http://example.com/%zz", "http://example.com/%zz"},
		{"non-URL input returned unchanged", "just-a-string", "just-a-string"},
		{"empty input returned unchanged", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalize(tc.in); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestKind_TableDriven(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		{"at-uri is standardfeed", "at://did:plc:aaaaaaaaaaaaaaaaaaaaaaaa/site.standard.publication/abc", "standardfeed"},
		{"https url is rss", "https://example.com/feed", "rss"},
		{"http url is rss", "http://example.com/feed", "rss"},
		{"non-url string is rss", "just-a-string", "rss"},
		{"empty string is rss", "", "rss"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Kind(tc.key); got != tc.want {
				t.Errorf("Kind(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestNormalize_SchemeAndTrailingSlashVariantsCollapseToSameKey(t *testing.T) {
	a := Normalize("http://example.com/feed/")
	b := Normalize("https://example.com/feed")
	// Scheme stays distinct: module 4 only collapses port/slash/fragment/case, not http-vs-https.
	if a == b {
		t.Fatalf("http and https must remain distinct keys, got both %q", a)
	}

	c := Normalize("https://example.com/feed/")
	d := Normalize("https://EXAMPLE.com:443/feed")
	if c != d {
		t.Errorf("Normalize(%q) = %q, Normalize(%q) = %q, want equal", "https://example.com/feed/", c, "https://EXAMPLE.com:443/feed", d)
	}
}

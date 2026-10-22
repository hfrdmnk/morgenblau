package webfeed

import "testing"

func TestURLTemplate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.Example.com/blog/my-post", "example.com/blog/*"},
		{"https://News.Example.com/posts/2024/05/hello/", "news.example.com/posts/#/#/*"},
		{"https://news.example.com/p/12345", "news.example.com/p/*"},
		{"https://news.example.com/2024/hello?utm_source=x#top", "news.example.com/#/*"},
		{"https://news.example.com/", "news.example.com/"},
		{"https://news.example.com", "news.example.com/"},
		{"https://www2.example.com/a/b", "www2.example.com/a/*"},
		{"http://127.0.0.1:8080/blog/x", "127.0.0.1:8080/blog/*"},
		{"https://news.example.com/pr/news/2026/20261010_1/index.html", "news.example.com/pr/news/#/*"},
		{"https://news.example.com/posts/hello/index.php", "news.example.com/posts/*"},
		{"https://news.example.com/index.html", "news.example.com/"},
		{"https://news.example.com/essays/greatwork.html", "news.example.com/essays/*"},
		{"mailto:someone@example.com", ""},
		{"::not a url", ""},
	}
	for _, c := range cases {
		if got := URLTemplate(c.in); got != c.want {
			t.Errorf("URLTemplate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFirstSegment(t *testing.T) {
	cases := map[string]string{
		"news.example.com/posts/*":  "posts",
		"news.example.com/*":        "",
		"news.example.com/":         "",
		"news.example.com/#/#/*":    "#",
		"news.example.com/blog/a/*": "blog",
	}
	for in, want := range cases {
		if got := templateFirstSegment(in); got != want {
			t.Errorf("templateFirstSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugTitleSkipsDefaultDocuments(t *testing.T) {
	for in, want := range map[string]string{
		"https://news.example.com/blog/hello-there-world/":           "Hello there world",
		"https://news.example.com/blog/hello-there-world/index.html": "Hello there world",
		"https://news.example.com/essays/great_work.html":            "Great work",
	} {
		if got := slugTitle(in); got != want {
			t.Errorf("slugTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGeneralizeTemplates(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"news.example.com/blog/money/tax/*", "news.example.com/blog/money/tax/*", "news.example.com/blog/start/ideas/*", "news.example.com/blog/money/cash/*"}, "news.example.com/blog/*/*/*"},
		{[]string{"news.example.com/jp/news/#/*", "news.example.com/jp/topics/#/*", "news.example.com/jp/news/#/*"}, "news.example.com/jp/*/#/*"},
		{[]string{"news.example.com/blog/a/*", "news.example.com/blog/*"}, ""},
		{[]string{"news.example.com/products/*", "news.example.com/solutions/*"}, ""},
		{[]string{"news.example.com/blog/*", "news.example.com/blog/*"}, ""},
		{[]string{"news.example.com/blog/x/*", "other.example.com/blog/y/*"}, ""},
	}
	for _, c := range cases {
		if got := generalizeTemplates(c.in); got != c.want {
			t.Errorf("generalizeTemplates(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchTemplate(t *testing.T) {
	cases := []struct {
		pattern, tmpl string
		want          bool
	}{
		{"news.example.com/blog/*", "news.example.com/blog/*", true},
		{"news.example.com/blog/*/*/*", "news.example.com/blog/money/tax/*", true},
		{"news.example.com/blog/*/*/*", "news.example.com/blog/money/*", false},
		{"news.example.com/jp/*/#/*", "news.example.com/jp/topics/#/*", true},
		{"news.example.com/jp/*/#/*", "news.example.com/jp/topics/2026a/*", false},
		{"news.example.com/blog/*", "other.example.com/blog/*", false},
	}
	for _, c := range cases {
		if got := matchTemplate(c.pattern, c.tmpl); got != c.want {
			t.Errorf("matchTemplate(%q, %q) = %v, want %v", c.pattern, c.tmpl, got, c.want)
		}
	}
}

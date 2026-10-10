package postkey

import (
	"net/url"
	"testing"
)

func TestURLNamesOnePostAcrossItsVariants(t *testing.T) {
	same := []string{
		"https://news.example.com/posts/first-post",
		"http://news.example.com/posts/first-post",
		"https://www.news.example.com/posts/first-post/",
		"https://NEWS.example.com/posts/first-post#comments",
		"https://news.example.com/posts/first-post//",
	}
	want := URL(same[0])
	if want != "news.example.com/posts/first-post" {
		t.Fatalf("URL(%q) = %q", same[0], want)
	}
	for _, raw := range same[1:] {
		if got := URL(raw); got != want {
			t.Errorf("URL(%q) = %q, want %q", raw, got, want)
		}
	}
	if URL("https://news.example.com/caf%c3%a9") != URL("https://news.example.com/café") {
		t.Error("percent-encoding case made another post")
	}
	for _, other := range []string{
		"https://news.example.com/posts/second-post",
		"https://news.example.com/posts/first-post?page=2",
		"https://news.example.com:8443/posts/first-post",
		"https://blog.example.com/posts/first-post",
	} {
		if URL(other) == want {
			t.Errorf("URL(%q) = %q, the same key as another post", other, want)
		}
	}
}

func TestFromURLMatchesURL(t *testing.T) {
	raw := "https://www.news.example.com/posts/first-post/?id=3"
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if FromURL(u) != URL(raw) {
		t.Fatalf("FromURL = %q, URL = %q", FromURL(u), URL(raw))
	}
}

func TestTitleIgnoresCaseAndSpacing(t *testing.T) {
	if Title("  A Quieter   way\nto follow ") != Title("a quieter way to follow") {
		t.Fatal("case or whitespace made another title")
	}
	if Title("A quieter way") == Title("A louder way") {
		t.Fatal("different titles share a key")
	}
}

func TestStripTrackingKeepsContentParameters(t *testing.T) {
	u, err := url.Parse("https://news.example.com/blog?utm_source=x&Ref=y&UTM_Medium=z&fbclid=1&gclid=2&section=world")
	if err != nil {
		t.Fatal(err)
	}
	StripTracking(u)
	if u.RawQuery != "section=world" {
		t.Fatalf("query = %q", u.RawQuery)
	}
}

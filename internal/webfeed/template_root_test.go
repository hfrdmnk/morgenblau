package webfeed

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestMatchTemplateStarNeedsASegment(t *testing.T) {
	for _, c := range []struct {
		pattern, tmpl string
		want          bool
	}{
		{"news.example.com/*", "news.example.com/", false},
		{"news.example.com/", "news.example.com/", true},
		{"news.example.com/*", "news.example.com/*", true},
		{"news.example.com/*/*", "news.example.com/blog/*", true},
		{"news.example.com/blog/*", "news.example.com/blog/*", true},
	} {
		if got := matchTemplate(c.pattern, c.tmpl); got != c.want {
			t.Errorf("matchTemplate(%q, %q) = %v, want %v", c.pattern, c.tmpl, got, c.want)
		}
	}
}

func TestRootSlugStreamSkipsItsPaginationLink(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><ul class="posts">`)
	for i := range 8 {
		title := postTitles[i]
		fmt.Fprintf(&b, `<li class="post"><h2><a href="/%s">%s</a></h2><time datetime="2024-03-%02d">March %d, 2024</time></li>`, slugify(title), title, 20-i, 20-i)
	}
	b.WriteString(`<li class="post"><h2><a href="/?page=2">Older posts</a></h2></li><li class="post"><h2><a href="/?page=9">Last page</a></h2></li></ul></main>`)
	s.set("/", page("Example Publication", b.String()))
	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if strings.Contains(it.URL, "page=") {
			t.Fatalf("setup served the pagination link as a post: %+v", it)
		}
	}
	if len(res.Items) != 8 {
		t.Fatalf("items %d, want 8; streams %+v", len(res.Items), res.Recipe.Streams)
	}
	body, _ := s.get(t, "/")
	items, err := apply(res.Recipe, body, s.url("/"))
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if strings.Contains(it.URL, "page=") {
			t.Fatalf("the poll served the pagination link as a post: %+v", it)
		}
	}
}

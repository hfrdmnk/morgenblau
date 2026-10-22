package webfeed

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestIsDateLikeOnlyWhenTheDateIsTheWholeTitle(t *testing.T) {
	for s, want := range map[string]bool{
		"May 3, 2024":                  true,
		"03.05.2024":                   true,
		"2024-05-01":                   true,
		"2 days ago":                   true,
		"Yesterday":                    true,
		"Monday, May 3, 2024":          true,
		"Published on May 3, 2024":     true,
		"3. Mai 2024":                  true,
		"May 2024":                     true,
		"Sept 2024":                    true,
		"May 3, 2024 at 10:30 AM":      true,
		"Monthly roundup: May 2024":    false,
		"Example Letter, October 2024": false,
		"Changelog 2024-05-01":         false,
		"Best gear set 2024":           false,
		"Out 2024 is the year we ship": false,
		"A quieter way to follow news": false,
	} {
		if got := isDateLike(s); got != want {
			t.Errorf("isDateLike(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestSetupKeepsListingTitlesThatMentionADate(t *testing.T) {
	titles := []string{
		"Monthly roundup: May 2024",
		"Example Letter, October 2024",
		"Changelog 2024-05-01",
		"Best gear set 2024",
		"Monthly roundup: April 2024",
		"Example Letter, September 2024",
	}
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><h1>Journal</h1><ul class="posts">`)
	for i, title := range titles {
		fmt.Fprintf(&b, `<li class="post"><h2><a href="/journal/%s">%s</a></h2><time datetime="2024-05-%02d">May %d, 2024</time></li>`, slugify(title), title, 20-i, 20-i)
	}
	b.WriteString(`</ul></main>`)
	s.set("/journal", page("Journal | Example Publication", siteNav+b.String()+siteFooter))
	res, err := Setup(context.Background(), s.url("/journal"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Healthy() {
		t.Fatalf("reasons %v, items %+v", res.Reasons, res.Items)
	}
	for i, it := range res.Items {
		if it.Title != titles[i] {
			t.Errorf("item %d title %q, want the listing's %q", i, it.Title, titles[i])
		}
	}
}

package webfeed

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// blogWithMostRead is a /blog listing of postTitles[0:8] beside an aside "Most read" list of older /blog posts.
func blogWithMostRead(asideHolds string) string {
	var b strings.Builder
	b.WriteString(blogList(8))
	b.WriteString(`<aside class="sidebar"><h2>Most read</h2><ul class="popular">`)
	for _, t := range append([]string{asideHolds}, postTitles[8:12]...) {
		if t == "" {
			continue
		}
		fmt.Fprintf(&b, `<li><a href="/blog/%s">%s</a></li>`, slugify(t), t)
	}
	b.WriteString(`</ul></aside>`)
	return page("Blog | Example Publication", siteNav+b.String()+siteFooter)
}

func TestSetupAnchoredLeavesTheSidebarOut(t *testing.T) {
	for name, tc := range map[string]struct {
		asideHolds string
		anchor     string
	}{
		"the main list holds the anchor":         {"", "/blog/" + slugify(postTitles[2])},
		"both lists hold the anchor":             {postTitles[2], "/blog/" + slugify(postTitles[2])},
		"no list holds it, both share its shape": {"", "/blog/a-post-this-page-never-links"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestSite(t)
			s.set("/blog", blogWithMostRead(tc.asideHolds))
			res, err := SetupAnchored(context.Background(), s.url("/blog"), s.url(tc.anchor), s.opts())
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range res.Items {
				for _, old := range postTitles[8:12] {
					if strings.HasSuffix(it.URL, "/"+slugify(old)) {
						t.Errorf("a most-read post joined the recipe: %s", it.URL)
					}
				}
			}
			if len(res.Items) != 8 {
				t.Errorf("items %d, want the main list's 8", len(res.Items))
			}
			for _, st := range res.Recipe.Streams {
				for _, h := range st.ContainerHints {
					if strings.Contains(h, "aside") || strings.Contains(h, "popular") {
						t.Errorf("stream %s keeps the sidebar's container hint %q", st.URLTemplate, h)
					}
				}
			}
		})
	}
}

func TestSetupAnchoredKeepsASidebarThatIsTheOnlyListHoldingThePost(t *testing.T) {
	s := newTestSite(t)
	var b strings.Builder
	b.WriteString(`<main><p>Welcome to the blog.</p></main><aside class="sidebar"><h2>Latest</h2><ul class="recent">`)
	for _, title := range postTitles[:5] {
		fmt.Fprintf(&b, `<li><a href="/blog/%s">%s</a></li>`, slugify(title), title)
	}
	b.WriteString(`</ul></aside>`)
	s.set("/blog", page("Blog | Example Publication", siteNav+b.String()+siteFooter))
	res, err := SetupAnchored(context.Background(), s.url("/blog"), s.url("/blog/"+slugify(postTitles[1])), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 5 {
		t.Fatalf("items %d, want the sidebar's 5 the reader pointed into", len(res.Items))
	}
}

package webfeed

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// jsonLDArticle is a post page whose publish date sits only in its embedded JSON-LD.
func jsonLDArticle(title, published string) string {
	ld := ""
	if published != "" {
		ld = `<script type="application/ld+json">{"@context":"https://schema.org","@type":"BlogPosting","headline":"` + title + `","datePublished":"` + published + `"}</script>`
	}
	return `<!doctype html><html lang="en"><head><title>` + title + ` | Example Publication</title>` + ld + `</head><body>` + siteNav +
		`<main><article><h1>` + title + `</h1><p>Body text of the post.</p></article></main>` + siteFooter + `</body></html>`
}

// undatedBlog serves /blog as cards without any date, each post dated only in its own page's JSON-LD; dates[i] "" leaves post i undated.
func undatedBlog(s *testSite, titles, dates []string) {
	var b strings.Builder
	b.WriteString(`<main><h1>Blog</h1><ul class="posts">`)
	for i, t := range titles {
		fmt.Fprintf(&b, `<li class="post"><h2><a href="/blog/%s">%s</a></h2><p>A short excerpt that explains the post in one or two sentences for readers.</p></li>`, slugify(t), t)
		s.set("/blog/"+slugify(t), jsonLDArticle(t, dates[i]))
	}
	b.WriteString(`</ul></main>`)
	s.set("/blog", page("Blog | Example Publication", siteNav+b.String()+siteFooter))
}

func TestPollDatesNewPostsFromTheirPagesLikeSetup(t *testing.T) {
	s := newTestSite(t)
	titles := postTitles[:8]
	dates := []string{"2024-03-20T09:00:00Z", "2024-03-19T09:00:00Z", "", "2024-03-17T09:00:00Z", "2024-03-16T09:00:00Z", "2024-03-15T09:00:00Z", "2024-03-14T09:00:00Z", "2024-03-13T09:00:00Z"}
	undatedBlog(s, titles, dates)
	res := setupRecipe(t, s, "/blog")
	if res.Recipe.DateSource != DateSourceArticle {
		t.Fatalf("date source = %s, want article", res.Recipe.DateSource)
	}

	newTitle := "A brand new post about calm reading"
	undatedBlog(s, append([]string{newTitle}, titles...), append([]string{"2024-03-22T09:00:00Z"}, dates...))
	before := map[string]int{}
	s.mu.Lock()
	for p, n := range s.hits {
		before[p] = n
	}
	s.mu.Unlock()

	got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res), Known: res.Items}, s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != PollOK || len(got.Items) != 9 {
		t.Fatalf("verdict %s %s, items %v", got.Verdict, got.Reason, itemURLs(got.Items))
	}
	if first := got.Items[0]; first.URL != s.url("/blog/"+slugify(newTitle)) || !first.Date.Equal(time.Date(2024, 3, 22, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("first item = %+v, want the new post dated from its page", first)
	}
	for _, it := range got.Items {
		if undated := it.URL == s.url("/blog/"+slugify(titles[2])); it.Date.IsZero() != undated {
			t.Fatalf("%s dated %v; only the post whose page states no date stays undated", it.URL, it.Date)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, title := range titles {
		if p := "/blog/" + slugify(title); s.hits[p] != before[p] {
			t.Errorf("%s fetched again on poll; known posts keep their stored date", p)
		}
	}
	if p := "/blog/" + slugify(newTitle); s.hits[p] != 1 {
		t.Errorf("new post fetched %d times, want 1", s.hits[p])
	}
}

func TestPollLeavesListingDatedRecipesToTheListing(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	if res.Recipe.DateSource != DateSourceListing {
		t.Fatalf("date source = %s, want listing", res.Recipe.DateSource)
	}
	s.set("/blog", page("Blog | Example Publication", siteNav+blogList(9)+siteFooter))
	s.set("/blog/"+slugify(postTitles[8]), article(postTitles[8], postTitles[8], "2024-03-12T09:00:00Z"))
	got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res), Known: res.Items}, s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != PollOK || len(got.Items) != 9 {
		t.Fatalf("verdict %s, items %d", got.Verdict, len(got.Items))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := s.hits["/blog/"+slugify(postTitles[8])]; n != 0 {
		t.Fatalf("new post page fetched %d times; its date is on the listing", n)
	}
}

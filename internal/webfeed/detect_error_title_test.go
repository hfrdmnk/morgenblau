package webfeed

import (
	"context"
	"testing"
)

func TestErrorTitleNeedsMoreThanALeadingNumber(t *testing.T) {
	cases := []struct {
		title string
		h1    string
		want  Reason
	}{
		{"404 Example Media", "404 Example Media", ""},
		{"500 Example Ventures Blog", "Latest from 500 Example Ventures", ""},
		{"Blog | 404 Example Media", "502 Example Weekly", ""},
		{"404", "", ReasonErrorPage},
		{"404 | Example Publication", "", ReasonErrorPage},
		{"404 – Example Publication", "", ReasonErrorPage},
		{"Error 404 Example Publication", "", ReasonErrorPage},
		{"Error: 500", "", ReasonErrorPage},
		{"410 Gone", "", ReasonErrorPage},
		{"503 Service Unavailable", "", ReasonErrorPage},
		{"Example Publication", "404", ReasonErrorPage},
		{"Example Publication", "404 — page missing", ReasonErrorPage},
	}
	for _, tc := range cases {
		t.Run(tc.title+"/"+tc.h1, func(t *testing.T) {
			body := page(tc.title, siteNav+`<main><h1>`+tc.h1+`</h1>`+blogList(8)+`</main>`+siteFooter)
			if got := pageTier0(mustPage(t, body, "https://news.example.com/blog")); got != tc.want {
				t.Fatalf("pageTier0 = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPollOfAListingNamedForAStatusCodeIsNotAnErrorPage(t *testing.T) {
	for _, name := range []string{"404 Example Media", "500 Example Ventures Blog"} {
		t.Run(name, func(t *testing.T) {
			s := newTestSite(t)
			s.addBlog(8)
			s.set("/blog", page(name, siteNav+`<main><h1>`+name+`</h1>`+blogList(8)+`</main>`+siteFooter))
			res := setupRecipe(t, s, "/blog")
			if !res.Healthy() {
				t.Fatalf("setup reasons %v", res.Reasons)
			}
			got, err := Poll(context.Background(), PollInput{Recipe: res.Recipe, History: historyFrom(res)}, s.opts())
			if err != nil || got.Verdict != PollOK {
				t.Fatalf("poll = verdict %q reason %q, %v", got.Verdict, got.Reason, err)
			}
		})
	}
}

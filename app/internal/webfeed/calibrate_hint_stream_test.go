package webfeed

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCalibratedTitleHintReachesAGeneralizedMinorityStream(t *testing.T) {
	s := newTestSite(t)
	paths := []string{}
	for i := range 6 {
		paths = append(paths, "/journal/"+slugify(postTitles[i]))
	}
	minority := []string{"/journal/articles/" + slugify(postTitles[6]), "/journal/articles/" + slugify(postTitles[7]),
		"/journal/series/" + slugify(postTitles[8]), "/journal/series/" + slugify(postTitles[9])}
	paths = append(paths, minority...)
	var b strings.Builder
	b.WriteString(`<main><div class="teasers">`)
	for i, p := range paths {
		fmt.Fprintf(&b, `<a class="teaser" href="%s"><p class="intro">A subtitle that sits above the headline %d</p><p class="big">%s</p><span>%d March 2024</span></a>`, p, i, postTitles[i], 20-i)
		s.set(p, article(postTitles[i], postTitles[i], ""))
	}
	b.WriteString(`</div></main>`)
	s.set("/journal", page("Journal | Example Publication", b.String()))
	res, err := Setup(context.Background(), s.url("/journal"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	minorTmpl := s.host() + "/journal/*/*"
	var found bool
	for _, st := range res.Recipe.Streams {
		if st.URLTemplate != minorTmpl {
			continue
		}
		found = true
		if st.TitleHints.Source != TitleCalibrated || st.TitleHints.Tag != "p.big" {
			t.Errorf("stream %s title hints = %+v, want the calibrated p.big", st.URLTemplate, st.TitleHints)
		}
	}
	if !found {
		t.Fatalf("no generalized minority stream; streams %+v", res.Recipe.Streams)
	}
	body, _ := s.get(t, "/journal")
	items, err := apply(res.Recipe, body, s.url("/journal"))
	if err != nil {
		t.Fatal(err)
	}
	byURL := map[string]string{}
	for _, it := range items {
		byURL[it.URL] = it.Title
	}
	for i, p := range minority {
		if got, want := byURL[s.url(p)], postTitles[6+i]; got != want {
			t.Errorf("poll title of %s = %q, want the setup's article title %q", p, got, want)
		}
	}
}

package websource

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/webfeed"
)

// jsonLDSite lists posts from..to-1 without any date on the listing; each post page states its date only in JSON-LD.
func jsonLDSite(web *fakeWeb, from, to int) {
	var b strings.Builder
	b.WriteString(`<main><h1>Blog</h1><ul class="posts">`)
	for i := from; i < to; i++ {
		t := driftTitles[i]
		fmt.Fprintf(&b, `<li class="post"><h2><a href="/blog/%s">%s</a></h2><p>A short teaser for post %d that tells readers what it is about.</p></li>`, driftSlug(t), t, i)
		web.set(driftURL(i, "blog"), `<!doctype html><html lang="en"><head><title>`+t+` | Example Publication</title>`+
			fmt.Sprintf(`<script type="application/ld+json">{"@type":"BlogPosting","headline":%q,"datePublished":"2026-09-%02dT09:00:00Z"}</script>`, t, 28-i)+
			`</head><body>`+driftNav+`<main><article><h1>`+t+`</h1><p>Body.</p></article></main>`+driftFooter+`</body></html>`)
	}
	b.WriteString(`</ul></main>`)
	web.set(driftListing, driftPage("Blog | Example Publication", b.String()))
}

func TestPollDueDatesANewPostTheWaySetupDid(t *testing.T) {
	env := newTestEnv(t)
	web := newFakeWeb()
	jsonLDSite(web, 2, 10)
	svc := NewService(env.reader, env.writer, NewWebfeedGenerator(webfeed.Options{Client: web.client()}),
		Config{PublicBaseURL: testBase, PollInterval: 30 * time.Minute, ModelDailyUSD: 1})
	svc.now = func() time.Time { return env.now }
	preview, err := svc.Create(context.Background(), testDID, driftListing, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source.Health != HealthOK {
		t.Fatalf("setup = %+v", preview.Source)
	}
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, preview.Source.FeedURL); err != nil {
		t.Fatal(err)
	}
	id := preview.Source.ID

	jsonLDSite(web, 1, 10)
	env.advance(31 * time.Minute)
	if n, err := svc.PollDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("PollDue = %d, %v", n, err)
	}
	fresh := storedItems(t, env, id)[driftURL(1, "blog")]
	if fresh.published == nil || *fresh.published != "2026-09-27T09:00:00Z" || fresh.history {
		t.Fatalf("new post = %+v (published %v), want its page's date as news", fresh, fresh.published)
	}
	var fracs []float64
	rows, err := env.reader.Query(`SELECT date_frac FROM web_source_polls WHERE source_id = ? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var f float64
		if err := rows.Scan(&f); err != nil {
			t.Fatal(err)
		}
		fracs = append(fracs, f)
	}
	if len(fracs) != 2 || fracs[0] != 1 || fracs[1] != 1 {
		t.Fatalf("history date_frac = %v, want setup's 1 then the poll's 1", fracs)
	}
	for i := 2; i < 10; i++ {
		if n := web.hitCount(driftURL(i, "blog")); n != 1 {
			t.Errorf("%s fetched %d times; a known post keeps its stored date", driftURL(i, "blog"), n)
		}
	}
}

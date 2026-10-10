package websource

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"morgenblau/internal/webfeed"
)

const (
	blockedListing = "https://news.example.com/news"
	newsSitemap    = "https://news.example.com/sitemap-news.xml"
)

func newsURL(i int) string { return "https://news.example.com/news/" + driftSlug(driftTitles[i]) }

func sitemapOf(from, to int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for i := from; i < to; i++ {
		fmt.Fprintf(&b, "<url><loc>%s</loc></url>", newsURL(i))
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// blockedSite refuses its listing and lists posts 2..5 in its sitemap.
func blockedSite() *fakeWeb {
	web := newFakeWeb()
	web.setPage(blockedListing, fakePage{status: http.StatusForbidden, body: "no"})
	web.setPage("https://news.example.com/robots.txt", fakePage{ctype: "text/plain", body: "User-agent: *\nAllow: /\nSitemap: " + newsSitemap + "\n"})
	web.setPage(newsSitemap, fakePage{ctype: "application/xml", body: sitemapOf(2, 6)})
	return web
}

func TestSitemapSourcesPollTheirSitemap(t *testing.T) {
	env := newTestEnv(t)
	web := blockedSite()
	svc := NewService(env.reader, env.writer, NewWebfeedGenerator(webfeed.Options{Client: web.client()}),
		Config{PublicBaseURL: testBase, PollInterval: 30 * time.Minute, ModelDailyUSD: 1})
	svc.now = func() time.Time { return env.now }
	preview, err := svc.Create(context.Background(), testDID, blockedListing, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source.Health != HealthOK || preview.ItemCount != 4 || !strings.Contains(readSource(t, env, preview.Source.ID).recipe, `"mode":"sitemap"`) {
		t.Fatalf("setup = %+v with %d items", preview.Source, preview.ItemCount)
	}
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, preview.Source.FeedURL); err != nil {
		t.Fatal(err)
	}
	id := preview.Source.ID
	poll := func() {
		t.Helper()
		env.advance(2 * time.Hour)
		if n, err := svc.PollDue(context.Background()); err != nil || n != 1 {
			t.Fatalf("PollDue = %d, %v", n, err)
		}
	}

	poll()
	if row := readSource(t, env, id); row.health != string(HealthOK) || row.hash == nil || len(storedItems(t, env, id)) != 4 {
		t.Fatalf("first poll: %+v, %d items", row, len(storedItems(t, env, id)))
	}
	hash := *readSource(t, env, id).hash
	polls := pollCount(t, env, id)
	poll()
	if row := readSource(t, env, id); row.health != string(HealthOK) || *row.hash != hash || pollCount(t, env, id) != polls {
		t.Fatalf("unchanged sitemap: %+v, good polls %d -> %d; want not modified", row, polls, pollCount(t, env, id))
	}

	web.setPage(newsSitemap, fakePage{ctype: "application/xml", body: sitemapOf(1, 6)})
	poll()
	fresh, ok := storedItems(t, env, id)[newsURL(1)]
	if !ok || fresh.history {
		t.Fatalf("new sitemap post = %+v, %v; want it published as news", fresh, ok)
	}
	if fresh.title != driftTitles[1] {
		t.Errorf("title = %q, want the slug read as a title", fresh.title)
	}

	web.setPage(newsSitemap, fakePage{status: http.StatusForbidden, body: "no"})
	poll()
	if row := readSource(t, env, id); row.health != string(HealthBlockedBySite) || row.failures != 1 {
		t.Fatalf("sitemap refused: %+v", row)
	}
}

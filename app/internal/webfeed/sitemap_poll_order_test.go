package webfeed

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

type datedLoc struct {
	loc     string
	lastmod time.Time
}

func datedSitemapXML(locs []datedLoc) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		fmt.Fprintf(&b, "<url><loc>%s</loc><lastmod>%s</lastmod></url>", l.loc, l.lastmod.Format(time.RFC3339))
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

// oldestFirstPosts are n /news posts listed oldest first, each edited a day after the one before.
func oldestFirstPosts(s *testSite, n int) []datedLoc {
	start := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	locs := make([]datedLoc, n)
	for i := range n {
		locs[i] = datedLoc{loc: s.url(fmt.Sprintf("/news/example-story-number-%03d", i)), lastmod: start.AddDate(0, 0, i)}
	}
	return locs
}

func TestSitemapPollKeepsSetupsNewestFirstCap(t *testing.T) {
	ctx := context.Background()
	s := newTestSite(t)
	s.mu.Lock()
	s.status["/news"] = 403
	s.mu.Unlock()
	s.set("/robots.txt", "User-agent: *\nAllow: /\nSitemap: "+s.url("/sitemap-news.xml")+"\n")
	locs := oldestFirstPosts(s, 300)
	s.set("/sitemap-news.xml", datedSitemapXML(locs))
	res := setupRecipe(t, s, "/news")
	if res.Recipe.Mode != ModeSitemap || len(res.Items) != MaxItems || res.Items[0].URL != locs[299].loc {
		t.Fatalf("setup mode %s items %d first %q, want the newest %d", res.Recipe.Mode, len(res.Items), res.Items[0].URL, MaxItems)
	}

	first, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res), Known: res.Items}, s.opts())
	if err != nil || first.Verdict != PollOK {
		t.Fatalf("first poll = %+v, %v", first.Verdict, err)
	}
	if got, want := itemURLs(first.Items), itemURLs(res.Items); !slices.Equal(got, want) {
		t.Fatalf("first poll serves %d items starting %q, want setup's newest %d starting %q", len(got), got[0], len(want), want[0])
	}

	fresh := datedLoc{loc: s.url("/news/a-story-published-after-setup"), lastmod: locs[299].lastmod.AddDate(0, 0, 1)}
	s.set("/sitemap-news.xml", datedSitemapXML(append(slices.Clone(locs), fresh)))
	got, err := Poll(ctx, PollInput{Recipe: res.Recipe, History: historyFrom(res), Known: res.Items, LinkSetHash: first.LinkSetHash}, s.opts())
	if err != nil || got.Verdict != PollOK || got.NotModified {
		t.Fatalf("poll after a new post = verdict %q not modified %v, %v", got.Verdict, got.NotModified, err)
	}
	if len(got.Items) == 0 || got.Items[0].URL != fresh.loc || len(got.Items) != MaxItems {
		t.Fatalf("poll after a new post = %d items starting %v, want %s first", len(got.Items), itemURLs(got.Items)[:min(3, len(got.Items))], fresh.loc)
	}
}

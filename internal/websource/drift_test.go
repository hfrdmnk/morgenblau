package websource

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"morgenblau/internal/webfeed"
)

// fakeWeb serves invented sites by full URL, so fixtures keep example.com hosts instead of a loopback address.
type fakeWeb struct {
	mu    sync.Mutex
	pages map[string]fakePage
	hits  map[string]int
}

type fakePage struct {
	status int
	ctype  string
	body   string
}

func newFakeWeb() *fakeWeb { return &fakeWeb{pages: map[string]fakePage{}, hits: map[string]int{}} }

func (f *fakeWeb) set(url, body string) { f.setPage(url, fakePage{body: body}) }

func (f *fakeWeb) setPage(url string, p fakePage) {
	f.mu.Lock()
	f.pages[url] = p
	f.mu.Unlock()
}

func (f *fakeWeb) hitCount(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[url]
}

func (f *fakeWeb) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.hits[r.URL.String()]++
	p, ok := f.pages[r.URL.String()]
	f.mu.Unlock()
	if !ok {
		p = fakePage{status: http.StatusNotFound, ctype: "text/plain", body: "not found"}
	}
	if p.status == 0 {
		p.status = http.StatusOK
	}
	if p.ctype == "" {
		p.ctype = "text/html; charset=utf-8"
	}
	return &http.Response{
		StatusCode: p.status, Header: http.Header{"Content-Type": {p.ctype}},
		Body: io.NopCloser(strings.NewReader(p.body)), Request: r,
	}, nil
}

func (f *fakeWeb) client() *http.Client { return &http.Client{Transport: f} }

const driftListing = "https://blog.example.com/blog"

var driftTitles = []string{
	"How we rebuilt the reading queue from scratch",
	"A quieter way to follow the news",
	"Shipping the offline mode for every reader",
	"What we learned from a year of daily digests",
	"Designing calm notifications that respect attention",
	"The case for fewer and better feeds",
	"Inside the new archive search engine",
	"Why our servers now sleep at night",
	"Small teams and slow software in practice",
	"Notes on typography for long reading sessions",
	"Building trust with transparent changelogs",
	"An honest look at our infrastructure costs",
}

func driftSlug(t string) string { return strings.ToLower(strings.ReplaceAll(t, " ", "-")) }

const driftNav = `<header><a href="/">Example Publication</a><nav><a href="/about">About</a><a href="/blog">Blog</a>
<a href="/pricing">Pricing</a><a href="/careers">Careers</a><a href="/docs">Docs</a><a href="/contact">Contact</a>
<a href="/login">Log in</a><a href="/signup">Sign up</a><a href="/changelog">Changelog</a><a href="/status">Status</a></nav></header>`

const driftFooter = `<footer><ul><li><a href="https://social.example.com/example">Social</a></li><li><a href="/privacy">Privacy policy</a></li>
<li><a href="/terms">Terms of service</a></li><li><a href="/imprint">Imprint</a></li></ul></footer>`

func driftPage(title, body string) string {
	return `<!doctype html><html lang="en"><head><title>` + title + `</title></head><body>` + driftNav + body + driftFooter + `</body></html>`
}

// driftPost is post i (0 is the newest) as one list entry under prefix.
func driftPost(i int, prefix string) string {
	t := driftTitles[i]
	return fmt.Sprintf(`<li class="post"><h2><a href="/%s/%s">%s</a></h2><time datetime="2026-09-%02d">September %d, 2026</time>`+
		`<p>A short teaser for post %d that tells readers what it is about.</p></li>`, prefix, driftSlug(t), t, 28-i, 28-i, i)
}

// driftList renders posts from..to-1 in a list under prefix.
func driftList(from, to int, prefix string) string {
	var b strings.Builder
	b.WriteString(`<main><h1>Blog</h1><ul class="posts">`)
	for i := from; i < to; i++ {
		b.WriteString(driftPost(i, prefix))
	}
	b.WriteString(`</ul></main>`)
	return b.String()
}

func driftURL(i int, prefix string) string {
	return "https://blog.example.com/" + prefix + "/" + driftSlug(driftTitles[i])
}

// driftSite is the publication at setup: posts 2..9 under /blog, each with an article page.
func driftSite() *fakeWeb {
	web := newFakeWeb()
	web.set(driftListing, driftPage("Blog | Example Publication", driftList(2, 10, "blog")))
	for i, t := range driftTitles {
		for _, prefix := range []string{"blog", "posts"} {
			web.set(driftURL(i, prefix), `<!doctype html><html lang="en"><head><title>`+t+` | Example Publication</title>`+
				`<meta property="og:title" content="`+t+`"><meta property="og:type" content="article">`+
				fmt.Sprintf(`<meta property="article:published_time" content="2026-09-%02dT09:00:00Z">`, 28-i)+
				`</head><body>`+driftNav+`<main><article><h1>`+t+`</h1><p>Body.</p></article></main>`+driftFooter+`</body></html>`)
		}
	}
	return web
}

// driftEnv is a service over the real webfeed generator, with one followed source set up from driftSite.
type driftEnv struct {
	*testEnv
	web     *fakeWeb
	service *Service
	id      string
}

func newDriftEnv(t *testing.T, model webfeed.Model) *driftEnv {
	t.Helper()
	env := newTestEnv(t)
	web := driftSite()
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Model: model})
	svc := NewService(env.reader, env.writer, gen, Config{PublicBaseURL: testBase, PollInterval: 30 * time.Minute, ModelDailyUSD: 1})
	svc.now = func() time.Time { return env.now }
	preview, err := svc.Create(context.Background(), testDID, driftListing, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source.Health != HealthOK || preview.ItemCount != 8 {
		t.Fatalf("setup = %+v with %d items", preview.Source, preview.ItemCount)
	}
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, preview.Source.FeedURL); err != nil {
		t.Fatal(err)
	}
	return &driftEnv{testEnv: env, web: web, service: svc, id: preview.Source.ID}
}

func (d *driftEnv) poll(t *testing.T) {
	t.Helper()
	d.advance(31 * time.Minute)
	if n, err := d.service.PollDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("PollDue = %d, %v", n, err)
	}
}

// served is the item links GET /web/<id> serves, in feed order.
func (d *driftEnv) served(t *testing.T) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	d.service.FeedHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/web/"+d.id, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /web/%s = %d", d.id, rec.Code)
	}
	var doc rssDoc
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range doc.Channel.Items {
		out = append(out, it.Link)
	}
	return out
}

func urlsOf(from, to int, prefix string) []string {
	var out []string
	for i := from; i < to; i++ {
		out = append(out, driftURL(i, prefix))
	}
	return out
}

func rehashClasses(html string) string {
	return strings.NewReplacer(`class="posts"`, `class="_x81kq posts_list__a9Z"`, `class="post"`, `class="_p71ka entry_card__Q2x"`).Replace(html)
}

func popularAside() string {
	var b strings.Builder
	b.WriteString(`<aside class="sidebar"><h3>Popular</h3><ol class="popular">`)
	for _, i := range []int{10, 11} {
		fmt.Fprintf(&b, `<li><a href="/blog/%s">%s</a></li>`, driftSlug(driftTitles[i]), driftTitles[i])
	}
	b.WriteString(`</ol></aside>`)
	return b.String()
}

func TestDriftBench(t *testing.T) {
	setupItems := urlsOf(2, 10, "blog")
	withNew := append(urlsOf(0, 2, "blog"), setupItems...)
	cases := []struct {
		name    string
		page    fakePage
		robots  string
		health  Health
		reason  string
		served  []string
		repair  *repairRow
		history []string
	}{
		{name: "new posts added", page: fakePage{body: driftPage("Blog", driftList(0, 10, "blog"))},
			health: HealthOK, served: withNew},
		{name: "class names rehashed", page: fakePage{body: rehashClasses(driftPage("Blog", driftList(0, 10, "blog")))},
			health: HealthOK, served: withNew},
		{name: "decoy popular list with the same url shape", page: fakePage{body: driftPage("Blog", `<div class="layout">`+driftList(0, 10, "blog")+popularAside()+`</div>`)},
			health: HealthOK, served: withNew},
		{name: "wrapper elements added", page: fakePage{body: driftPage("Blog", strings.Replace(driftList(0, 10, "blog"), `<main>`, `<main><div class="container"><section class="inner">`, 1))},
			health: HealthOK, served: withNew},
		{name: "list cut to three", page: fakePage{body: driftPage("Blog", driftList(2, 5, "blog"))},
			health: HealthOK, served: setupItems,
			repair: &repairRow{cause: "detector", outcome: "accepted", step: "known_urls", rule: "known_url", reason: "count_ratio"}},
		{name: "url scheme changed", page: fakePage{body: driftPage("Blog", driftList(2, 10, "posts"))},
			health: HealthOK, served: append(urlsOf(2, 10, "posts"), setupItems...), history: urlsOf(2, 10, "posts"),
			repair: &repairRow{cause: "detector", outcome: "accepted", step: "heuristic", rule: "titles", reason: "empty"}},
		{name: "only topic links left", page: fakePage{body: driftPage("Blog", `<main><h1>Topics</h1><div class="topics"><a href="/tags/design">Design</a><a href="/tags/engineering">Engineering</a><a href="/tags/culture">Culture</a><a href="/tags/product">Product</a></div></main>`)},
			health: HealthNeedsAttention, reason: "the page changed and the post list could not be found again", served: setupItems,
			repair: &repairRow{cause: "detector", outcome: "unresolved", step: "heuristic", reason: "empty"}},
		{name: "challenge page", page: fakePage{body: `<!doctype html><html><head><title>Just a moment...</title></head><body><p>Checking your browser.</p></body></html>`},
			health: HealthBlockedBySite, reason: "the site showed a bot check instead of the page", served: setupItems},
		{name: "cookie wall", page: fakePage{body: driftPage("Blog", `<div class="consent"><h2>Before you continue</h2><p>We and our partners use cookies.</p><button>Accept all</button></div>`)},
			health: HealthBroken, reason: "the site showed a cookie wall instead of the page", served: setupItems},
		{name: "error page", page: fakePage{body: driftPage("Page not found | Example Publication", `<main><h1>Page not found</h1></main>`)},
			health: HealthBroken, reason: "the site showed an error page", served: setupItems},
		{name: "server error", page: fakePage{status: http.StatusInternalServerError, body: "oops"},
			health: HealthBroken, reason: "the site returned an error", served: setupItems},
		{name: "robots.txt now disallows the listing", page: fakePage{body: driftPage("Blog", driftList(0, 10, "blog"))}, robots: "User-agent: *\nDisallow: /blog\n",
			health: HealthBlockedBySite, reason: "this site asks feed readers not to read this page", served: setupItems},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newDriftEnv(t, nil)
			if got := d.served(t); !slices.Equal(got, setupItems) {
				t.Fatalf("served after setup = %v", got)
			}
			d.web.setPage(driftListing, tc.page)
			if tc.robots != "" {
				d.web.setPage("https://blog.example.com/robots.txt", fakePage{ctype: "text/plain", body: tc.robots})
			}
			d.poll(t)

			row := readSource(t, d.testEnv, d.id)
			if Health(row.health) != tc.health || row.reason != tc.reason {
				t.Fatalf("health %s %q, want %s %q", row.health, row.reason, tc.health, tc.reason)
			}
			if got := d.served(t); !sameSet(got, tc.served) {
				t.Errorf("served = %v\nwant     %v", got, tc.served)
			}
			repairs := repairRows(t, d.testEnv, d.id)
			switch {
			case tc.repair == nil && len(repairs) != 0:
				t.Errorf("repairs = %+v, want none", repairs)
			case tc.repair != nil && (len(repairs) != 1 || repairs[0] != *tc.repair):
				t.Errorf("repairs = %+v, want %+v", repairs, *tc.repair)
			}
			items := storedItems(t, d.testEnv, d.id)
			for _, u := range tc.history {
				if !items[u].history {
					t.Errorf("%s was stored as a new post; a moved post belongs to history", u)
				}
			}
			for _, u := range withNew[:2] {
				if it, ok := items[u]; ok && (it.history || it.excerpt == "") {
					t.Errorf("new post %s = %+v, want a fresh item with its teaser", u, it)
				}
			}

			// The next poll of the same page is quiet: a repair stored a recipe that holds.
			if tc.health == HealthOK {
				before := len(repairs)
				d.poll(t)
				if row := readSource(t, d.testEnv, d.id); row.health != string(HealthOK) || len(repairRows(t, d.testEnv, d.id)) != before {
					t.Errorf("second poll: health %s, repairs %d -> %d", row.health, before, len(repairRows(t, d.testEnv, d.id)))
				}
			}
		})
	}
}

func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

func TestDriftTierZeroSpendsNothing(t *testing.T) {
	d := newDriftEnv(t, nil)
	d.web.set(driftListing, `<!doctype html><html><head><title>Just a moment...</title></head><body></body></html>`)
	before := d.web.hitCount(driftURL(2, "blog"))
	d.poll(t)
	if d.web.hitCount(driftURL(2, "blog")) != before {
		t.Error("a broken poll fetched article pages")
	}
	if got := d.web.hitCount(driftListing); got != 2 {
		t.Errorf("listing fetched %d times, want setup plus one poll", got)
	}
}

func TestWeeklyCheckRunsOnceAWeekAndItsAlarmGoesThroughTheLadder(t *testing.T) {
	w := &scriptedModel{}
	d := newDriftEnv(t, w)
	d.poll(t)
	if w.count(webfeed.EffortLow) != 0 {
		t.Fatalf("watcher called %d times before the week was up", w.count(webfeed.EffortLow))
	}
	d.advance(7 * 24 * time.Hour)
	d.poll(t)
	if w.count(webfeed.EffortLow) != 1 {
		t.Fatalf("watcher calls = %d, want 1", w.count(webfeed.EffortLow))
	}
	d.poll(t)
	if w.count(webfeed.EffortLow) != 1 {
		t.Fatalf("watcher called again right after the check")
	}
	if len(repairRows(t, d.testEnv, d.id)) != 0 {
		t.Fatal("a healthy check started a repair")
	}

	w.setBroken(true)
	d.advance(7 * 24 * time.Hour)
	d.poll(t)
	rows := repairRows(t, d.testEnv, d.id)
	if len(rows) != 1 || rows[0].cause != "weekly_check" || rows[0].outcome != "unresolved" {
		t.Fatalf("repairs = %+v, want the alarm left standing when re-finding only reproduces the doubted posts", rows)
	}
	if row := readSource(t, d.testEnv, d.id); row.health != string(HealthNeedsAttention) {
		t.Errorf("health after the alarm = %s", row.health)
	}

	d.web.set(driftListing, driftPage("Blog", driftList(0, 10, "blog")))
	d.poll(t)
	if row := readSource(t, d.testEnv, d.id); row.health != string(HealthNeedsAttention) {
		t.Errorf("an ordinary poll closed the weekly alarm: health = %s", row.health)
	}
	if got := d.served(t); slices.Contains(got, driftURL(0, "blog")) {
		t.Errorf("published new posts while the weekly alarm stands: %v", got)
	}
	rows = repairRows(t, d.testEnv, d.id)
	// The first repair's setup rung asked no model, so the day's model repair is still unspent and the retry climbs to it.
	if len(rows) != 2 || rows[1].cause != "weekly_check" || rows[1].outcome != "unresolved" || rows[1].step != "judge" || rows[0].usedModel || rows[1].usedModel {
		t.Errorf("repairs = %+v, want the open alarm to retry the whole ladder on the next poll without spending a model call", rows)
	}
	if w.count(webfeed.EffortLow) != 2 {
		t.Errorf("watcher calls = %d, want the next check to wait for its day", w.count(webfeed.EffortLow))
	}

	d.advance(24 * time.Hour)
	d.poll(t)
	if w.count(webfeed.EffortLow) != 3 {
		t.Errorf("watcher calls = %d, want the check back a day after its alarm", w.count(webfeed.EffortLow))
	}
	if row := readSource(t, d.testEnv, d.id); row.health != string(HealthNeedsAttention) {
		t.Errorf("a check that still doubts the list closed the alarm: health = %s", row.health)
	}

	w.setBroken(false)
	d.advance(24 * time.Hour)
	d.poll(t)
	if row := readSource(t, d.testEnv, d.id); row.health != string(HealthOK) {
		t.Errorf("a healthy weekly check left health = %s", row.health)
	}
	if got := d.served(t); !slices.Contains(got, driftURL(0, "blog")) {
		t.Errorf("new posts missing after the alarm closed: %v", got)
	}
}

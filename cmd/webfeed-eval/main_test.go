package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseOptionsDefaultsAndValidation(t *testing.T) {
	opts, err := parseOptions([]string{"sites.json"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.input != "sites.json" || opts.judge != "off" || opts.concurrency != 4 || opts.timeout <= 0 {
		t.Fatalf("defaults = %+v", opts)
	}
	if opts, err := parseOptions([]string{"-skip-feed", "sites.json"}); err != nil || !opts.skipFeed {
		t.Fatalf("-skip-feed = %+v, %v", opts, err)
	}
	if opts, err := parseOptions([]string{"-judge", "haiku", "-listing-snapshot-field", "sites.json"}); err != nil || opts.judge != "haiku" || !opts.snapshotField {
		t.Fatalf("-judge haiku -listing-snapshot-field = %+v, %v", opts, err)
	}
	for _, bad := range [][]string{{}, {"-judge", "sonnet", "sites.json"}, {"-concurrency", "0", "sites.json"}, {"a.json", "b.json"}} {
		if _, err := parseOptions(bad); err == nil {
			t.Errorf("parseOptions(%v) accepted", bad)
		}
	}
}

const feedXML = `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Publication</title>
<item><title>First example post</title><link>%[1]s/blog/first-example-post</link><pubDate>Tue, 05 Mar 2024 10:00:00 GMT</pubDate></item>
<item><title>Second example post</title><link>%[1]s/blog/second-example-post</link></item>
</channel></rss>`

func listingHTML(feedLink string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><title>Blog</title>` + feedLink + `</head><body><main><ul class="posts">`)
	for i, t := range []string{"How we rebuilt the reading queue", "A quieter way to follow the news", "Shipping the offline mode for readers", "What we learned from daily digests"} {
		fmt.Fprintf(&b, `<li><h2><a href="/blog/post-%d-%s">%s</a></h2><time datetime="2024-03-%02d"></time></li>`, i, strings.ReplaceAll(strings.ToLower(t), " ", "-"), t, 10-i)
	}
	b.WriteString(`</ul></main></body></html>`)
	return b.String()
}

func newSite(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/with-feed":
			fmt.Fprint(w, listingHTML(`<link rel="alternate" type="application/rss+xml" href="/feed.xml">`))
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprintf(w, feedXML, srv.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEvalReportsFeedStreamsAndBlockedSites(t *testing.T) {
	srv := newSite(t)
	// Feed discovery probes common paths at the site root, so sites without any feed live on their own host.
	bare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/no-feed":
			fmt.Fprint(w, listingHTML(""))
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(bare.Close)
	sites := []site{
		{ID: "feed", ListingURL: srv.URL + "/with-feed"},
		{ID: "dom", ListingURL: bare.URL + "/no-feed"},
		{ID: "blocked", ListingURL: bare.URL + "/forbidden"},
		{ID: "gone", ListingURL: bare.URL + "/missing"},
	}
	var out bytes.Buffer
	err := run(context.Background(), sites, options{judge: "off", concurrency: 2, timeout: 10 * time.Second}, env{client: srv.Client()}, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Results []result `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	byID := map[string]result{}
	for _, r := range got.Results {
		byID[r.ID] = r
	}
	if len(got.Results) != 4 || got.Results[0].ID != "feed" {
		t.Fatalf("results keep input order: %+v", got.Results)
	}
	feed := byID["feed"]
	if feed.Mode != "feed" || feed.FeedURL != srv.URL+"/feed.xml" || feed.Health != "ok" || len(feed.Items) != 2 || feed.Items[0].Date != "2024-03-05T10:00:00Z" || feed.Items[1].Date != "" {
		t.Fatalf("feed = %+v", feed)
	}
	dom := byID["dom"]
	if dom.Mode != "streams" || dom.Health != "ok" || len(dom.Items) != 4 || dom.Items[0].Title != "How we rebuilt the reading queue" || dom.JudgeCalls != 0 || dom.CostUSD != 0 {
		t.Fatalf("dom = %+v", dom)
	}
	if b := byID["blocked"]; b.Health != "blocked_by_site" || b.Mode != "none" {
		t.Fatalf("blocked = %+v", b)
	}
	if g := byID["gone"]; g.Health != "failed" || g.Error == "" || g.Mode != "none" {
		t.Fatalf("gone = %+v", g)
	}
}

func TestSkipFeedRunsTheDOMPathOnFeedSites(t *testing.T) {
	srv := newSite(t)
	var out bytes.Buffer
	err := run(context.Background(), []site{{ID: "feed", ListingURL: srv.URL + "/with-feed"}}, options{judge: "off", concurrency: 1, timeout: 10 * time.Second, skipFeed: true}, env{client: srv.Client()}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"mode": "streams"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestOutputUsesTheScorerFieldNames(t *testing.T) {
	raw, err := json.Marshal(output{Results: []result{{ID: "x", Items: []item{{URL: "u"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{`"results"`, `"id"`, `"listing_url"`, `"mode"`, `"feed_url"`, `"health"`, `"items"`, `"url"`, `"title"`, `"date"`, `"judge_calls"`, `"induce_calls"`, `"cost_usd"`, `"error"`, `"recipe_listing_url"`} {
		if !strings.Contains(string(raw), f) {
			t.Errorf("output %s lacks %s", raw, f)
		}
	}
}

func TestReadSites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sites.json")
	if err := os.WriteFile(path, []byte(`[{"id":"a","listing_url":"https://news.example.com/blog"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	sites, err := readSites(path)
	if err != nil || len(sites) != 1 || sites[0].ListingURL != "https://news.example.com/blog" || sites[0].Snapshot != "" {
		t.Fatalf("sites %+v err %v", sites, err)
	}
}

func TestEvalReportsTheListingTheRecipeEndedUpOn(t *testing.T) {
	posts := []string{"How we rebuilt the reading queue", "A quieter way to follow the news", "Shipping the offline mode for readers", "What we learned from daily digests", "Designing calm notifications for readers"}
	list := func(n int) string {
		var b strings.Builder
		b.WriteString(`<!doctype html><html lang="en"><head><title>Example Publication</title></head><body><nav><a href="/about">About</a><a href="/blog">Blog</a></nav><main><ul class="posts">`)
		for i, t := range posts[:n] {
			fmt.Fprintf(&b, `<li><h2><a href="/blog/%s">%s</a></h2><time datetime="2024-03-%02d"></time></li>`, strings.ReplaceAll(strings.ToLower(t), " ", "-"), t, 10-i)
		}
		b.WriteString(`</ul></main></body></html>`)
		return b.String()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, list(3))
		case "/blog":
			fmt.Fprint(w, list(5))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	var out bytes.Buffer
	err := run(context.Background(), []site{{ID: "home", ListingURL: srv.URL + "/"}}, options{judge: "off", concurrency: 1, timeout: 10 * time.Second}, env{client: srv.Client()}, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if r := got.Results[0]; r.ListingURL != srv.URL+"/" || r.RecipeListingURL != srv.URL+"/blog" || len(r.Items) != 5 {
		t.Fatalf("result = %+v", r)
	}
}

func TestCountingDoerTalliesRequestsAndBodyBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", 100))
	}))
	t.Cleanup(srv.Close)
	c := &countingDoer{next: srv.Client()}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, srv.URL+"/page", nil)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if c.requests.Load() != 2 || c.posts.Load() != 1 || c.bytes.Load() != 200 {
		t.Fatalf("requests %d, posts %d, bytes %d", c.requests.Load(), c.posts.Load(), c.bytes.Load())
	}
}

func TestEvalLoadsLivewireLazyListsThroughThePostClient(t *testing.T) {
	posts := []string{"How we rebuilt the reading queue", "A quieter way to follow the news", "Shipping the offline mode for readers", "What we learned from daily digests", "Designing calm notifications for readers"}
	var list strings.Builder
	list.WriteString(`<ul class="posts">`)
	for i, p := range posts {
		fmt.Fprintf(&list, `<li><h2><a href="/blog/%s">%s</a></h2><time datetime="2024-03-%02d"></time></li>`, strings.ReplaceAll(strings.ToLower(p), " ", "-"), p, 10-i)
	}
	list.WriteString(`</ul>`)
	shell := `<!doctype html><html lang="en"><head><title>Blog</title></head><body><main>` +
		`<section wire:snapshot="{&quot;memo&quot;:{&quot;id&quot;:&quot;c1&quot;}}" x-init="$wire.__lazyLoad('eyJsaXN0IjoxfQ')"><span>Loading</span></section>` +
		`</main><script src="/livewire/livewire.min.js" data-csrf="token-123" data-update-uri="/livewire/update"></script></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/blog":
			fmt.Fprint(w, shell)
		case r.URL.Path == "/livewire/update" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"components": []any{map[string]any{"effects": map[string]string{"html": "<section>" + list.String() + "</section>"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	var out bytes.Buffer
	opts := options{judge: "off", concurrency: 1, timeout: 10 * time.Second, skipFeed: true}
	if err := run(context.Background(), []site{{ID: "lazy", ListingURL: srv.URL + "/blog"}}, opts, env{client: srv.Client(), post: srv.Client()}, &out); err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if r := got.Results[0]; len(r.Items) != len(posts) {
		t.Fatalf("result = %+v, want the lazy list's %d posts", r, len(posts))
	}
}

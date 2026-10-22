// Command webfeed-eval runs feed discovery and webfeed setup over a list of listing pages and prints what a reader would get, for an external scorer.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/atidentity"
	"morgenblau/internal/claude"
	"morgenblau/internal/feedfinder"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/standardfeed"
	"morgenblau/internal/webfeed"
	"morgenblau/internal/websource"
)

type options struct {
	input       string
	judge       string
	concurrency int
	timeout     time.Duration
	explain     bool
	skipFeed    bool
	cacheDir    string
	// snapshotField serves each site's listing page from its "snapshot" file; every other request stays live.
	snapshotField bool
}

func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("webfeed-eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := options{}
	fs.StringVar(&opts.judge, "judge", "off", "model for unsure setups and recipe induction: off or haiku (reads ANTHROPIC_API_KEY)")
	fs.IntVar(&opts.concurrency, "concurrency", 4, "sites evaluated in parallel")
	fs.DurationVar(&opts.timeout, "timeout", 90*time.Second, "time budget per site")
	fs.BoolVar(&opts.explain, "explain", false, "log each site's candidates and setup record to stderr")
	fs.StringVar(&opts.cacheDir, "cache", "", "replay responses from this directory and record new ones (tuning only)")
	fs.BoolVar(&opts.skipFeed, "skip-feed", false, "skip feed discovery to measure the DOM path alone (tuning only)")
	fs.BoolVar(&opts.snapshotField, "listing-snapshot-field", false, `serve each listing page from the site's "snapshot" file; feed probes, robots, sitemaps and articles stay live`)
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 1 {
		return options{}, errors.New("usage: webfeed-eval [-judge off|haiku] [-listing-snapshot-field] [-concurrency N] [-timeout D] sites.json")
	}
	opts.input = fs.Arg(0)
	if opts.judge != "off" && opts.judge != "haiku" {
		return options{}, fmt.Errorf("-judge %q: use off or haiku", opts.judge)
	}
	if opts.concurrency < 1 {
		return options{}, errors.New("-concurrency must be at least 1")
	}
	if opts.timeout <= 0 {
		return options{}, errors.New("-timeout must be positive")
	}
	return opts, nil
}

type site struct {
	ID         string `json:"id"`
	ListingURL string `json:"listing_url"`
	Snapshot   string `json:"snapshot"`
}

type item struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Date  string `json:"date"`
}

type result struct {
	ID         string `json:"id"`
	ListingURL string `json:"listing_url"`
	// RecipeListingURL is the page polls read, a section page when setup moved off the pasted one.
	RecipeListingURL string  `json:"recipe_listing_url"`
	Mode             string  `json:"mode"`
	FeedURL          string  `json:"feed_url"`
	Health           string  `json:"health"`
	Items            []item  `json:"items"`
	JudgeCalls       int     `json:"judge_calls"`
	InduceCalls      int     `json:"induce_calls"`
	CostUSD          float64 `json:"cost_usd"`
	Error            string  `json:"error"`
}

type output struct {
	Results []result `json:"results"`
}

func readSites(path string) ([]site, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sites []site
	if err := json.Unmarshal(raw, &sites); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return sites, nil
}

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	sites, err := readSites(opts.input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	safe := safehttp.NewClient(opts.timeout, 5)
	// Discovery is composed as in internal/server: the same finder with the Standardfeed probes.
	e := env{client: safe, post: safehttp.NewClient(opts.timeout, 0), standard: standardfeed.NewClient(atidentity.Guarded(safe), safe)}
	if opts.cacheDir != "" {
		if err := os.MkdirAll(opts.cacheDir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		e.client = cachingDoer{dir: opts.cacheDir, next: e.client}
		e.post = cachingDoer{dir: opts.cacheDir, next: e.post}
	}
	if opts.judge == "haiku" {
		model, err := newModel(os.Getenv("ANTHROPIC_API_KEY"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		e.model = model
	}
	if err := run(context.Background(), sites, opts, e, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// env holds what run fetches and asks through; a nil model or standard resolver turns that step off.
type env struct {
	client webfeed.Doer
	// post sends Livewire's lazy-list POST and must not follow redirects; nil leaves lazy lists unloaded.
	post     webfeed.Doer
	standard feedfinder.StandardResolver
	model    webfeed.Model
}

// newModel builds the Haiku client; the key comes from the environment and is never printed.
func newModel(apiKey string) (webfeed.Model, error) {
	if apiKey == "" {
		return nil, errors.New("-judge haiku needs ANTHROPIC_API_KEY in the environment")
	}
	c, err := claude.New(claude.Config{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	return claudeModel{c}, nil
}

// claudeModel runs webfeed's prompts on Claude, as internal/server wires it.
type claudeModel struct{ c *claude.Client }

func (m claudeModel) Complete(ctx context.Context, p webfeed.Prompt) (string, webfeed.Usage, error) {
	text, u, err := m.c.Complete(ctx, p.System, p.User, string(p.Effort))
	return text, webfeed.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, USD: u.USD}, err
}

// run evaluates sites in parallel and writes one JSON document with results in input order.
func run(ctx context.Context, sites []site, opts options, e env, w io.Writer) error {
	results := make([]result, len(sites))
	sem := make(chan struct{}, opts.concurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(ctx, opts.timeout)
			defer cancel()
			results[i] = evalSite(sctx, s, opts, e)
		})
	}
	wg.Wait()
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(output{Results: results})
}

func evalSite(ctx context.Context, s site, opts options, e env) result {
	r := result{ID: s.ID, ListingURL: s.ListingURL, Mode: "none", Items: []item{}}
	client := e.client
	if opts.snapshotField && s.Snapshot != "" {
		body, err := os.ReadFile(s.Snapshot)
		if err != nil {
			r.Health, r.Error = "failed", "snapshot: "+err.Error()
			return r
		}
		client = snapshotDoer{listingURL: s.ListingURL, body: body, next: client}
	}
	post := e.post
	if opts.explain {
		gets, posts := &countingDoer{next: client}, &countingDoer{next: post}
		client = gets
		if post != nil {
			post = posts
		}
		defer func() {
			slog.Info("fetches", "id", s.ID, "requests", gets.requests.Load()+posts.requests.Load(),
				"posts", gets.posts.Load()+posts.posts.Load(), "bytes", gets.bytes.Load()+posts.bytes.Load())
		}()
	}
	if !opts.skipFeed {
		finder := feedfinder.New(client)
		if e.standard != nil {
			finder = finder.WithStandardResolver(e.standard)
		}
		cands, err := finder.Resolve(ctx, s.ListingURL)
		if err != nil {
			r.Health, r.Error = "failed", "discovery: "+err.Error()
			return r
		}
		// The reader is offered generation only when discovery finds nothing at all.
		if len(cands) > 0 {
			return feedRoute(ctx, client, r, cands)
		}
	}
	res, err := webfeed.Setup(ctx, s.ListingURL, webfeed.Options{Client: client, PostClient: post, Model: e.model})
	if err != nil {
		r.Health, r.Error = "failed", err.Error()
		return r
	}
	if opts.explain {
		explain(s, res)
	}
	r.Health = string(websource.SetupHealth(res))
	r.RecipeListingURL = res.Recipe.ListingURL
	u := res.Recipe.Setup.ModelUsage()
	r.JudgeCalls, r.InduceCalls, r.CostUSD = u.JudgeCalls, u.Calls-u.JudgeCalls, u.USD
	for _, it := range res.Items {
		r.Items = append(r.Items, item{URL: it.URL, Title: it.Title, Date: formatDate(it.Date)})
	}
	if len(r.Items) > 0 {
		r.Mode = string(res.Recipe.Mode)
		if res.Recipe.Mode == webfeed.ModeSitemap {
			r.FeedURL = res.Recipe.FeedURL
		}
	}
	return r
}

// feedRoute reports the subscription the reader would get: the first RSS candidate with items, else the first candidate as offered.
func feedRoute(ctx context.Context, client webfeed.Doer, r result, cands []feedfinder.Candidate) result {
	var firstErr error
	for _, c := range cands {
		if c.Kind != "" || c.FeedURL == "" {
			continue
		}
		items, err := feedItems(ctx, client, c.FeedURL)
		if err == nil && len(items) > 0 {
			r.Mode, r.FeedURL, r.Health, r.Items = "feed", c.FeedURL, string(websource.HealthOK), items
			return r
		}
		if firstErr == nil {
			firstErr = err
			if err == nil {
				firstErr = errors.New("feed has no items")
			}
		}
	}
	first := cands[0]
	if first.Kind == "standardfeed" {
		r.Mode, r.FeedURL, r.Health = "standardfeed", first.Publication, string(websource.HealthOK)
		return r
	}
	if firstErr == nil {
		firstErr = errors.New("no readable feed among the candidates")
	}
	r.Mode, r.FeedURL, r.Health, r.Error = "feed", first.FeedURL, "failed", firstErr.Error()
	return r
}

// snapshotDoer serves the exact listing URL from a saved page and passes every other request through.
type snapshotDoer struct {
	listingURL string
	body       []byte
	next       webfeed.Doer
}

func (d snapshotDoer) Do(req *http.Request) (*http.Response, error) {
	if req.URL.String() != d.listingURL {
		return d.next.Do(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader(d.body)),
		Request:    req,
	}, nil
}

// countingDoer tallies one site's requests and the response bytes its readers consumed, for -explain.
type countingDoer struct {
	next     webfeed.Doer
	requests atomic.Int64
	posts    atomic.Int64
	bytes    atomic.Int64
}

func (c *countingDoer) Do(req *http.Request) (*http.Response, error) {
	c.requests.Add(1)
	if req.Method == http.MethodPost {
		c.posts.Add(1)
	}
	resp, err := c.next.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body = countingBody{ReadCloser: resp.Body, n: &c.bytes}
	return resp, nil
}

type countingBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (b countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n.Add(int64(n))
	return n, err
}

func explain(s site, res *webfeed.Result) {
	rec := res.Recipe.Setup
	slog.Info("setup", "id", s.ID, "blocked", res.Blocked, "reasons", res.Reasons, "stages", rec.Stages,
		"confidence", rec.Confidence, "margin", rec.Margin, "judge_wanted", rec.JudgeWanted, "judge_reasons", rec.JudgeReasons)
	for i, c := range res.Candidates {
		var sample []string
		for _, e := range c.Entries[:min(3, len(c.Entries))] {
			sample = append(sample, e.Title)
		}
		slog.Info("candidate", "id", s.ID, "i", i, "score", fmt.Sprintf("%.2f", c.Score), "template", c.Template, "n", len(c.Entries),
			"dated", c.DateFrac, "nav", c.InNav, "hint", c.ContainerHint, "sample", strings.Join(sample, " / "))
	}
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// feedItems fetches and parses a discovered feed, keeping every item.
func feedItems(ctx context.Context, client webfeed.Doer, feedURL string) ([]item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", safehttp.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	feed, err := gofeed.NewParser().Parse(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(feedURL)
	var items []item
	for _, fi := range feed.Items {
		link := strings.TrimSpace(fi.Link)
		if link == "" {
			continue
		}
		if base != nil {
			if u, err := base.Parse(link); err == nil {
				link = u.String()
			}
		}
		var d time.Time
		switch {
		case fi.PublishedParsed != nil:
			d = *fi.PublishedParsed
		case fi.UpdatedParsed != nil:
			d = *fi.UpdatedParsed
		}
		items = append(items, item{URL: link, Title: strings.TrimSpace(fi.Title), Date: formatDate(d)})
	}
	return items, nil
}

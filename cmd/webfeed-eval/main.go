// Command webfeed-eval runs feed discovery and webfeed setup over a list of listing pages and prints what a reader would get, for an external scorer.
package main

import (
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
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/feedfinder"
	"morgenblau/internal/safehttp"
	"morgenblau/internal/webfeed"
)

type options struct {
	input       string
	judge       string
	concurrency int
	timeout     time.Duration
	explain     bool
	skipFeed    bool
	cacheDir    string
}

func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("webfeed-eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := options{}
	fs.StringVar(&opts.judge, "judge", "off", "judge for unsure setups: off")
	fs.IntVar(&opts.concurrency, "concurrency", 4, "sites evaluated in parallel")
	fs.DurationVar(&opts.timeout, "timeout", 90*time.Second, "time budget per site")
	fs.BoolVar(&opts.explain, "explain", false, "log each site's candidates and setup record to stderr")
	fs.StringVar(&opts.cacheDir, "cache", "", "replay responses from this directory and record new ones (tuning only)")
	fs.BoolVar(&opts.skipFeed, "skip-feed", false, "skip feed discovery to measure the DOM path alone (tuning only)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 1 {
		return options{}, errors.New("usage: webfeed-eval [-judge off] [-concurrency N] [-timeout D] sites.json")
	}
	opts.input = fs.Arg(0)
	if opts.judge != "off" {
		return options{}, fmt.Errorf("-judge %q is not available yet; use off", opts.judge)
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
}

type item struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Date  string `json:"date"`
}

type result struct {
	ID         string  `json:"id"`
	ListingURL string  `json:"listing_url"`
	Mode       string  `json:"mode"`
	FeedURL    string  `json:"feed_url"`
	Health     string  `json:"health"`
	Items      []item  `json:"items"`
	JudgeCalls int     `json:"judge_calls"`
	CostUSD    float64 `json:"cost_usd"`
	Error      string  `json:"error"`
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
	var client webfeed.Doer = safehttp.NewClient(opts.timeout, 5)
	if opts.cacheDir != "" {
		if err := os.MkdirAll(opts.cacheDir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		client = cachingDoer{dir: opts.cacheDir, next: client}
	}
	if err := run(context.Background(), sites, opts, client, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run evaluates sites in parallel and writes one JSON document with results in input order.
func run(ctx context.Context, sites []site, opts options, client webfeed.Doer, w io.Writer) error {
	results := make([]result, len(sites))
	sem := make(chan struct{}, opts.concurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(ctx, opts.timeout)
			defer cancel()
			results[i] = evalSite(sctx, s, opts, client)
		})
	}
	wg.Wait()
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(output{Results: results})
}

func evalSite(ctx context.Context, s site, opts options, client webfeed.Doer) result {
	r := result{ID: s.ID, ListingURL: s.ListingURL, Mode: "none", Items: []item{}}
	if !opts.skipFeed {
		if feed, items := discoverFeed(ctx, client, s.ListingURL); feed != "" {
			r.Mode, r.FeedURL, r.Health, r.Items = "feed", feed, string(webfeed.HealthOK), items
			return r
		}
	}
	res, err := webfeed.Setup(ctx, s.ListingURL, webfeed.Options{Client: client})
	if err != nil {
		r.Health, r.Error = "failed", err.Error()
		return r
	}
	if opts.explain {
		explain(s, res)
	}
	r.Health = string(res.Health)
	if res.Recipe.Setup.Judge != nil {
		r.JudgeCalls, r.CostUSD = res.Recipe.Setup.Judge.Calls, res.Recipe.Setup.Judge.USD
	}
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

// discoverFeed returns the first discovered RSS candidate that parses with items.
func discoverFeed(ctx context.Context, client webfeed.Doer, listingURL string) (string, []item) {
	cands, err := feedfinder.New(client).Resolve(ctx, listingURL)
	if err != nil {
		return "", nil
	}
	for _, c := range cands {
		if c.Kind != "" || c.FeedURL == "" {
			continue
		}
		if items, err := feedItems(ctx, client, c.FeedURL); err == nil && len(items) > 0 {
			return c.FeedURL, items
		}
	}
	return "", nil
}

func explain(s site, res *webfeed.Result) {
	rec := res.Recipe.Setup
	slog.Info("setup", "id", s.ID, "health", res.Health, "reasons", res.HealthReasons, "stages", rec.Stages,
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
		if base != nil {
			if u, err := base.Parse(link); err == nil {
				link = u.String()
			}
		}
		if link == "" {
			continue
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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"morgenblau/internal/standardfeed"
	"morgenblau/internal/webfeed"
)

func evalOne(t *testing.T, s site, opts options, e env) result {
	t.Helper()
	opts.concurrency, opts.timeout = 1, 10*time.Second
	if opts.judge == "" {
		opts.judge = "off"
	}
	var out bytes.Buffer
	if err := run(context.Background(), []site{s}, opts, e, &out); err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got.Results) != 1 {
		t.Fatalf("output %s: %v", out.String(), err)
	}
	return got.Results[0]
}

// tonedListing defeats the extractor: each post link has its own class and category path.
func tonedListing() string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><title>Blog</title></head><body><main><div class="feed">`)
	for i, tone := range []string{"ocean", "forest", "desert", "glacier", "meadow", "canyon"} {
		fmt.Fprintf(&b, `<article class="post"><h2><a class="tone-%s" href="/blog/%s/story-number-%d">Story number %d from the example desk</a></h2><time datetime="2024-03-%02d"></time></article>`, tone, tone, i, i, 20-i)
	}
	b.WriteString(`</div></main></body></html>`)
	return b.String()
}

type stubJudge struct{ calls atomic.Int32 }

func (j *stubJudge) Judge(ctx context.Context, in webfeed.JudgeInput) (webfeed.Verdict, webfeed.Usage, error) {
	j.calls.Add(1)
	return webfeed.Verdict{Pick: []int{0}, Order: webfeed.OrderDate}, webfeed.Usage{InputTokens: 2500, OutputTokens: 600, USD: 0.00055}, nil
}

type stubInducer struct{ calls atomic.Int32 }

func (i *stubInducer) Induce(ctx context.Context, in webfeed.InduceInput) (webfeed.CSSRecipe, webfeed.Usage, error) {
	i.calls.Add(1)
	return webfeed.CSSRecipe{Item: "article.post", Link: "h2 a", Date: "time"}, webfeed.Usage{InputTokens: 20000, OutputTokens: 900, USD: 0.00245}, nil
}

func TestModelStepsAreWiredAndTheirCostReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blog" {
			fmt.Fprint(w, tonedListing())
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	j, ind := &stubJudge{}, &stubInducer{}
	r := evalOne(t, site{ID: "toned", ListingURL: srv.URL + "/blog"}, options{judge: "haiku"}, env{client: srv.Client(), judge: j, inducer: ind})
	if r.Mode != "css" || len(r.Items) != 6 || r.Health != "ok" {
		t.Fatalf("result = %+v", r)
	}
	if r.JudgeCalls != int(j.calls.Load()) || r.InduceCalls != 1 || ind.calls.Load() != 1 {
		t.Fatalf("judge calls %d (made %d), induce calls %d", r.JudgeCalls, j.calls.Load(), r.InduceCalls)
	}
	if want := float64(r.JudgeCalls)*0.00055 + 0.00245; math.Abs(r.CostUSD-want) > 1e-9 {
		t.Fatalf("cost_usd = %v, want %v", r.CostUSD, want)
	}
}

func TestNewModelNeedsTheKeyAndNeverEchoesIt(t *testing.T) {
	if _, err := newModel(""); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	if c, err := newModel("sk-test-not-a-real-key"); err != nil || c == nil {
		t.Fatalf("newModel = %v, %v", c, err)
	}
}

func TestListingSnapshotReplacesOnlyTheListing(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		if r.URL.Path == "/no-feed" {
			fmt.Fprint(w, listingHTML(""))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	snap := filepath.Join(t.TempDir(), "listing.html")
	if err := os.WriteFile(snap, []byte(tonedListing()), 0o600); err != nil {
		t.Fatal(err)
	}
	s := site{ID: "snap", ListingURL: srv.URL + "/no-feed", Snapshot: snap}

	r := evalOne(t, s, options{snapshotField: true}, env{client: srv.Client(), inducer: &stubInducer{}})
	mu.Lock()
	listingHits, otherHits := hits["/no-feed"], len(hits)
	mu.Unlock()
	if r.Mode != "css" || len(r.Items) != 6 || !strings.HasPrefix(r.Items[0].Title, "Story number") {
		t.Fatalf("snapshot result = %+v", r)
	}
	if listingHits != 0 || otherHits == 0 {
		t.Fatalf("listing fetched live %d times; %d other paths fetched live", listingHits, otherHits)
	}

	r = evalOne(t, s, options{}, env{client: srv.Client()})
	if r.Mode != "streams" || r.Items[0].Title != "How we rebuilt the reading queue" {
		t.Fatalf("without the flag the live page is used: %+v", r)
	}

	r = evalOne(t, site{ID: "missing", ListingURL: s.ListingURL, Snapshot: filepath.Join(t.TempDir(), "absent.html")}, options{snapshotField: true}, env{client: srv.Client()})
	if r.Health != "failed" || !strings.Contains(r.Error, "snapshot") {
		t.Fatalf("missing snapshot = %+v", r)
	}
}

func TestAdvertisedFeedWinsEvenWhenBrokenLikeOnTheServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blog" {
			fmt.Fprint(w, listingHTML(`<link rel="alternate" type="application/rss+xml" href="/broken.xml">`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	r := evalOne(t, site{ID: "broken-feed", ListingURL: srv.URL + "/blog"}, options{}, env{client: srv.Client()})
	if r.Mode != "feed" || r.FeedURL != srv.URL+"/broken.xml" || r.Health != "failed" || r.Error == "" {
		t.Fatalf("result = %+v; the server offers the advertised feed and never generation", r)
	}
}

// flakyDoer fails the first request so discovery errors while setup would succeed.
type flakyDoer struct {
	once sync.Once
	next *http.Client
}

func (d *flakyDoer) Do(req *http.Request) (*http.Response, error) {
	failed := false
	d.once.Do(func() { failed = true })
	if failed {
		return nil, errors.New("connection reset")
	}
	return d.next.Do(req)
}

func TestDiscoveryFailureIsReportedNotGenerated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blog" {
			fmt.Fprint(w, listingHTML(""))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	r := evalOne(t, site{ID: "flaky", ListingURL: srv.URL + "/blog"}, options{}, env{client: &flakyDoer{next: srv.Client()}})
	if r.Health != "failed" || r.Mode != "none" || !strings.Contains(r.Error, "discovery") {
		t.Fatalf("result = %+v; the server answers 502 here and offers no generation", r)
	}
}

type stubStandard struct{}

func (stubStandard) GetPublication(ctx context.Context, uri string) (*standardfeed.Publication, error) {
	return &standardfeed.Publication{URI: uri, Name: "Example Publication", URL: "https://pub.example.com"}, nil
}

func (stubStandard) GetDocument(ctx context.Context, uri string) (*standardfeed.Document, error) {
	return nil, errors.New("not found")
}

func (stubStandard) FetchWellKnown(ctx context.Context, siteURL string) (string, error) {
	return "at://did:plc:example/site.standard.publication/self", nil
}

func TestStandardPublicationIsAFeedRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blog" {
			fmt.Fprint(w, listingHTML(""))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	r := evalOne(t, site{ID: "std", ListingURL: srv.URL + "/blog"}, options{}, env{client: srv.Client(), standard: stubStandard{}})
	if r.Mode != "standardfeed" || r.FeedURL != "at://did:plc:example/site.standard.publication/self" || r.Health != "ok" {
		t.Fatalf("result = %+v", r)
	}
}

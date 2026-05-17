package feedfinder

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// roundTripperFunc lets us hand-roll responses without spinning up a server.
type roundTripperFunc func(*http.Request) *http.Response

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r), nil
}

func resp(body, contentType string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

const htmlWithFeeds = `<!doctype html><html><head>
<link rel="alternate" type="application/rss+xml" title="Posts" href="/feed.xml">
<link rel="alternate" type="application/atom+xml" title="Comments" href="/comments.atom">
<link rel="alternate" type="application/json" href="https://other.example/feed.json">
</head><body></body></html>`

func TestResolve_LinkRelAlternate_MultipleCandidates(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if r.URL.Host == "example.test" {
			return resp(htmlWithFeeds, "text/html; charset=utf-8")
		}
		t.Fatalf("unexpected host: %s", r.URL.Host)
		return nil
	})})

	cands, err := finder.Resolve(context.Background(), "https://example.test/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 3 {
		t.Fatalf("got %d candidates, want 3: %+v", len(cands), cands)
	}
	if cands[0].FeedURL != "https://example.test/feed.xml" {
		t.Errorf("cand0.FeedURL = %q", cands[0].FeedURL)
	}
	if cands[1].FeedURL != "https://example.test/comments.atom" {
		t.Errorf("cand1.FeedURL = %q", cands[1].FeedURL)
	}
	if cands[2].FeedURL != "https://other.example/feed.json" {
		t.Errorf("cand2.FeedURL = %q", cands[2].FeedURL)
	}
}

func TestResolve_PassthroughDirectFeedURL(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		return resp(`<?xml version="1.0"?><rss></rss>`, "application/rss+xml")
	})})

	cands, err := finder.Resolve(context.Background(), "https://lobste.rs/rss")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 || cands[0].FeedURL != "https://lobste.rs/rss" {
		t.Errorf("passthrough mis-resolved: %+v", cands)
	}
}

func TestResolve_PassthroughExtractsCanonicalTitle(t *testing.T) {
	// Mastodon-style direct RSS feed — body parse should yield <channel><title>.
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>dominikhofer</title>
    <link>https://social.lol/@dominikhofer</link>
    <description>Posts</description>
  </channel>
</rss>`
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		return resp(body, "application/rss+xml")
	})})

	cands, err := finder.Resolve(context.Background(), "https://social.lol/@dominikhofer.rss")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("len = %d", len(cands))
	}
	if cands[0].Title != "dominikhofer" {
		t.Errorf("Title = %q, want %q", cands[0].Title, "dominikhofer")
	}
}

func TestResolve_YouTube_ChannelDirect(t *testing.T) {
	// /channel/UC... should resolve WITHOUT hitting HTTP.
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		t.Fatal("HTTP should not be called for /channel/<id>")
		return nil
	})})
	cands, err := finder.Resolve(context.Background(), "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("len = %d", len(cands))
	}
	if !strings.Contains(cands[0].FeedURL, "channel_id=UCabcdefghijklmnopqrstuv") {
		t.Errorf("FeedURL = %q", cands[0].FeedURL)
	}
}

func TestResolve_YouTube_HandlePath(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		// Real YouTube pages embed channelId in JSON; mimic that.
		return resp(`<html><body><script>var x = {"channelId":"UCwxyzABCDEFGHIJKLMNopq","other":1}</script></body></html>`, "text/html")
	})})
	cands, err := finder.Resolve(context.Background(), "https://www.youtube.com/@somecreator")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || !strings.Contains(cands[0].FeedURL, "channel_id=UCwxyzABCDEFGHIJKLMNopq") {
		t.Errorf("cands = %+v", cands)
	}
}

func TestResolve_ApplePodcasts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/lookup", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "1234567" {
			t.Errorf("id = %q", r.URL.Query().Get("id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"feedUrl":"https://feeds.example/show.rss","collectionName":"Some Show"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	finder := New(http.DefaultClient).WithITunesBase(srv.URL + "/lookup")
	cands, err := finder.Resolve(context.Background(), "https://podcasts.apple.com/us/podcast/some-show/id1234567")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("len = %d", len(cands))
	}
	if cands[0].FeedURL != "https://feeds.example/show.rss" {
		t.Errorf("FeedURL = %q", cands[0].FeedURL)
	}
	if cands[0].Title != "Some Show" {
		t.Errorf("Title = %q", cands[0].Title)
	}
}

func TestResolve_NoFeedsFound_EmptyList(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		return resp(`<html><head></head><body>no feeds here</body></html>`, "text/html")
	})})
	cands, err := finder.Resolve(context.Background(), "https://barren.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 0 {
		t.Errorf("expected empty, got %+v", cands)
	}
}

package feedfinder

import (
	"context"
	"io"
	"net/http"
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
<link rel="alternate" type="application/json" href="https://alternate.example.com/feed.json">
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
	if cands[2].FeedURL != "https://alternate.example.com/feed.json" {
		t.Errorf("cand2.FeedURL = %q", cands[2].FeedURL)
	}
}

func TestResolve_PassthroughDirectFeedURL(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		return resp(`<?xml version="1.0"?><rss></rss>`, "application/rss+xml")
	})})

	cands, err := finder.Resolve(context.Background(), "https://example.com/rss")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 || cands[0].FeedURL != "https://example.com/rss" {
		t.Errorf("passthrough mis-resolved: %+v", cands)
	}
}

func TestResolve_PassthroughExtractsCanonicalTitle(t *testing.T) {
	// Mastodon-style direct RSS feed; body parse should yield <channel><title>.
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Author</title>
    <link>https://social.example.com/@example-user</link>
    <description>Posts</description>
  </channel>
</rss>`
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		return resp(body, "application/rss+xml")
	})})

	cands, err := finder.Resolve(context.Background(), "https://social.example.com/@example-user.rss")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("len = %d", len(cands))
	}
	if cands[0].Title != "Example Author" {
		t.Errorf("Title = %q, want %q", cands[0].Title, "Example Author")
	}
}

const ytAtomFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Vollmar Cant</title>
  <link href="https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv"/>
</feed>`

func TestResolve_YouTube_ChannelDirect(t *testing.T) {
	// /channel/<id> skips the HTML scrape but still fetches the feed to fill in the title.
	var feedHits int
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if strings.Contains(r.URL.RawQuery, "channel_id=UCabcdefghijklmnopqrstuv") {
			feedHits++
			return resp(ytAtomFeed, "application/atom+xml")
		}
		t.Fatalf("unexpected request: %s", r.URL.String())
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
	if cands[0].Title != "Vollmar Cant" {
		t.Errorf("Title = %q, want %q", cands[0].Title, "Vollmar Cant")
	}
	if feedHits != 1 {
		t.Errorf("feed fetches = %d, want 1", feedHits)
	}
}

func TestResolve_YouTube_HandlePath_ConsentBypass(t *testing.T) {
	// YouTube's consent-bypass page (no cookies) embeds the channel ID only in canonical/og:url/feed-link attrs, not the JS blob.
	const channelHTML = `<html><head>
<link rel="canonical" href="https://www.youtube.com/channel/UCwxyzABCDEFGHIJKLMNopq">
<link rel="alternate" type="application/rss+xml" title="RSS" href="https://www.youtube.com/feeds/videos.xml?channel_id=UCwxyzABCDEFGHIJKLMNopq">
</head></html>`
	const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Aaron Francis</title></feed>`
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if strings.HasPrefix(r.URL.Path, "/feeds/videos.xml") {
			return resp(feedXML, "application/atom+xml")
		}
		return resp(channelHTML, "text/html")
	})})
	cands, err := finder.Resolve(context.Background(), "https://www.youtube.com/@aarondfrancis")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || !strings.Contains(cands[0].FeedURL, "channel_id=UCwxyzABCDEFGHIJKLMNopq") {
		t.Fatalf("cands = %+v", cands)
	}
	if cands[0].Title != "Aaron Francis" {
		t.Errorf("Title = %q, want %q", cands[0].Title, "Aaron Francis")
	}
}

func TestResolve_YouTube_HandlePath(t *testing.T) {
	const channelHTML = `<html><body><script>var x = {"channelId":"UCwxyzABCDEFGHIJKLMNopq","other":1}</script></body></html>`
	const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>example creator</title></feed>`
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if strings.HasPrefix(r.URL.Path, "/feeds/videos.xml") {
			return resp(feedXML, "application/atom+xml")
		}
		// Real YouTube pages embed channelId in JSON; mimic that.
		return resp(channelHTML, "text/html")
	})})
	cands, err := finder.Resolve(context.Background(), "https://www.youtube.com/@example-creator")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || !strings.Contains(cands[0].FeedURL, "channel_id=UCwxyzABCDEFGHIJKLMNopq") {
		t.Errorf("cands = %+v", cands)
	}
	if cands[0].Title != "example creator" {
		t.Errorf("Title = %q, want %q", cands[0].Title, "example creator")
	}
}

func TestResolve_YouTube_FeedFetchFailure_StillReturnsCandidate(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if strings.HasPrefix(r.URL.Path, "/feeds/videos.xml") {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("boom"))}
		}
		t.Fatalf("unexpected request: %s", r.URL.String())
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
	if cands[0].Title != "" {
		t.Errorf("Title = %q, want empty", cands[0].Title)
	}
}

func TestResolve_YouTube_HandlePath_FirstMatchWins(t *testing.T) {
	// Pins the regex contract: canonical link/og:url/feed link precede user content like an embed pointing at another channel.
	const channelHTML = `<html><head>
<link rel="canonical" href="https://www.youtube.com/channel/UCcanonicalAAAAAAAAAAAA">
<link rel="alternate" type="application/rss+xml" href="https://www.youtube.com/feeds/videos.xml?channel_id=UCcanonicalAAAAAAAAAAAA">
</head><body>
<a href="/channel/UCotherChannelBBBBBBBBBB">someone else</a>
</body></html>`
	const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Canonical Channel</title></feed>`
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if strings.HasPrefix(r.URL.Path, "/feeds/videos.xml") {
			if !strings.Contains(r.URL.RawQuery, "channel_id=UCcanonicalAAAAAAAAAAAA") {
				t.Fatalf("wrong channel picked: %s", r.URL.RawQuery)
			}
			return resp(feedXML, "application/atom+xml")
		}
		return resp(channelHTML, "text/html")
	})})
	cands, err := finder.Resolve(context.Background(), "https://www.youtube.com/@someone")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || !strings.Contains(cands[0].FeedURL, "channel_id=UCcanonicalAAAAAAAAAAAA") {
		t.Fatalf("cands = %+v", cands)
	}
}

func TestResolve_YouTube_HandlePath_NonOK_NoCandidate(t *testing.T) {
	// A non-2xx response must not feed the regex; falls through to generic resolution.
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		// Error page happens to mention a UC… string but must be ignored.
		return &http.Response{
			StatusCode: 404,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader(`<html>not found /channel/UCshouldNotBePickedXXXXX</html>`)),
		}
	})})
	cands, err := finder.Resolve(context.Background(), "https://www.youtube.com/@nope")
	if err != nil {
		t.Fatal(err)
	}
	// Falls through to generic link-rel-alternate path, which finds nothing.
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %+v", cands)
	}
}

func TestResolve_SendsMorgenblauUserAgent(t *testing.T) {
	var gotUA string
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		gotUA = r.Header.Get("User-Agent")
		return resp(htmlWithFeeds, "text/html; charset=utf-8")
	})})

	if _, err := finder.Resolve(context.Background(), "https://example.test/blog"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.Contains(gotUA, "Morgenblau") {
		t.Errorf("User-Agent = %q, want it to contain Morgenblau", gotUA)
	}
}

func TestResolve_YouTube_SendsMorgenblauUserAgent(t *testing.T) {
	// Handle-path resolution hits both the channel-scrape request and the feed-title fetch.
	var gotUAs []string
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		gotUAs = append(gotUAs, r.Header.Get("User-Agent"))
		if strings.HasPrefix(r.URL.Path, "/feeds/videos.xml") {
			return resp(ytAtomFeed, "application/atom+xml")
		}
		return resp(`<html><body><script>var x = {"channelId":"UCabcdefghijklmnopqrstuv"}</script></body></html>`, "text/html")
	})})

	if _, err := finder.Resolve(context.Background(), "https://www.youtube.com/@example-creator"); err != nil {
		t.Fatal(err)
	}
	if len(gotUAs) != 2 {
		t.Fatalf("requests made = %d, want 2", len(gotUAs))
	}
	for _, ua := range gotUAs {
		if !strings.Contains(ua, "Morgenblau") {
			t.Errorf("User-Agent = %q, want it to contain Morgenblau", ua)
		}
	}
}

func TestResolve_NoFeedsFound_EmptyList(t *testing.T) {
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
		return resp(`<html><head></head><body>no feeds here</body></html>`, "text/html")
	})})
	cands, err := finder.Resolve(context.Background(), "https://empty.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 0 {
		t.Errorf("expected empty, got %+v", cands)
	}
}

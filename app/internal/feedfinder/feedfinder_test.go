package feedfinder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

func TestResolve_PassthroughSiteURLComesFromTheFeedLink(t *testing.T) {
	cases := []struct {
		name, contentType, body, want string
	}{
		{
			name:        "rss channel link",
			contentType: "application/rss+xml",
			body: `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Blog</title>
<link>https://blog.example.com/</link>
<atom:link xmlns:atom="http://www.w3.org/2005/Atom" href="https://blog.example.com/rss" rel="self"/>
</channel></rss>`,
			want: "https://blog.example.com/",
		},
		{
			name:        "atom alternate link, not self",
			contentType: "application/atom+xml",
			body: `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Example Blog</title>
<link rel="self" href="https://blog.example.com/rss"/>
<link rel="alternate" href="https://blog.example.com/"/>
</feed>`,
			want: "https://blog.example.com/",
		},
		{
			name:        "relative link resolves against the feed",
			contentType: "application/atom+xml",
			body:        `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Example Blog</title><link href="/"/></feed>`,
			want:        "https://blog.example.com/",
		},
		{
			name:        "no site link stays empty, since the feed URL is no site",
			contentType: "application/rss+xml",
			body:        `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Blog</title></channel></rss>`,
			want:        "",
		},
		{
			name:        "non-http link stays empty",
			contentType: "application/rss+xml",
			body:        `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Blog</title><link>javascript:alert(1)</link></channel></rss>`,
			want:        "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
				return resp(tc.body, tc.contentType)
			})})
			cands, err := finder.Resolve(context.Background(), "https://blog.example.com/rss")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if len(cands) != 1 {
				t.Fatalf("len = %d", len(cands))
			}
			if cands[0].SiteURL != tc.want {
				t.Errorf("SiteURL = %q, want %q", cands[0].SiteURL, tc.want)
			}
		})
	}
}

const ytAtomFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Vollmar Cant</title>
  <link href="https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv"/>
</feed>`

func TestResolve_YouTube_ChannelDirect(t *testing.T) {
	var feedHits int
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		if strings.Contains(r.URL.RawQuery, "channel_id=UCabcdefghijklmnopqrstuv") {
			feedHits++
			return resp(ytAtomFeed, "application/atom+xml")
		}
		t.Fatalf("unexpected request %s: a /channel/<id> URL names its feed, so only the feed is fetched, for its title", r.URL.String())
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

func TestSameYouTubeFeedURLs_NameTheChannelFeedAndItsLongFormFeed(t *testing.T) {
	const all = "https://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv"
	const longForm = "https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv"
	both := fmt.Sprint([]string{all, longForm})
	for raw, want := range map[string]string{
		all:      both,
		longForm: both,
		"https://youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv":        both,
		"https://www.youtube.com/feeds/videos.xml?playlist_id=UUSHabcdefghijklmnopqrstuv": "[]",
		"https://www.youtube.com/feeds/videos.xml?playlist_id=PLabcdefghijklmnopqrstuv":   "[]",
		"https://www.youtube.com/feeds/videos.xml?channel_id=UCshort":                     "[]",
		"https://blog.example.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv":   "[]",
		"https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv":                        "[]",
	} {
		if got := fmt.Sprint(SameYouTubeFeedURLs(raw)); got != want {
			t.Errorf("SameYouTubeFeedURLs(%q) = %s, want %s: only the UULF uploads playlist, which leaves out Shorts and live streams, is the same channel; UUSH and other playlists are different feeds", raw, got, want)
		}
	}
}

func TestYouTubeChannelURL_NamesTheChannelBehindEitherFeed(t *testing.T) {
	const channel = "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv"
	for raw, want := range map[string]string{
		"https://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv":    channel,
		"https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv": channel,
		"https://www.youtube.com/feeds/videos.xml?playlist_id=PLabcdefghijklmnopqrstuv":   "",
		"https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv":                        channel,
		"https://www.youtube.com/@example":                                                "",
		"https://blog.example.com/feed.xml":                                               "",
	} {
		if got := YouTubeChannelURL(raw); got != want {
			t.Errorf("YouTubeChannelURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestYouTubeHandleURL_ReadsTheChannelsOwnHandle(t *testing.T) {
	const page = `<script>{"canonicalBaseUrl":"/@featured"}{"channelMetadataRenderer":{"vanityChannelUrl":"http://www.youtube.com/@example","ownerUrls":["http://www.youtube.com/@example"]}}</script>`
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"handle":     {200, page, "https://www.youtube.com/@example"},
		"no handle":  {200, `{"canonicalBaseUrl":"/@featured"}`, ""},
		"error page": {404, page, ""},
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
				if r.URL.String() != "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv" {
					t.Fatalf("unexpected request: %s", r.URL)
				}
				res := resp(tc.body, "text/html")
				res.StatusCode = tc.status
				return res
			})}
			got, err := YouTubeHandleURL(context.Background(), client, "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv")
			if err != nil || got != tc.want {
				t.Errorf("YouTubeHandleURL = %q, %v, want %q: the handle is the channel's own vanityChannelUrl, since featured channels appear on the page as canonicalBaseUrl before it", got, err, tc.want)
			}
		})
	}
}

func TestYouTubeChannelGone_ReadsTheChannelPagesOwnVerdict(t *testing.T) {
	const gone = `<script>{"alerts":[{"alertRenderer":{"type":"ERROR","text":{"simpleText":"This channel does not exist."}}}]}</script>`
	const live = `<script>{"alerts":[{"alertRenderer":{"type":"ERROR","text":{"simpleText":"Something went wrong."}}}]}{"channelMetadataRenderer":{"vanityChannelUrl":"http://www.youtube.com/@example"}}</script>`
	for name, tc := range map[string]struct {
		status int
		body   string
		want   bool
	}{
		"channel does not exist":        {200, gone, true},
		"channel page with metadata":    {200, live, false},
		"error status says nothing":     {404, gone, false},
		"page without alert or channel": {200, `<html></html>`, false},
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
				if r.URL.String() != "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv" {
					t.Fatalf("unexpected request: %s", r.URL)
				}
				res := resp(tc.body, "text/html")
				res.StatusCode = tc.status
				return res
			})}
			got, err := YouTubeChannelGone(context.Background(), client, "https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv")
			if err != nil || got != tc.want {
				t.Errorf("YouTubeChannelGone = %v, %v, want %v: YouTube answers a channel that does not exist with 200, an error alert and no channel metadata, so an error status proves nothing", got, err, tc.want)
			}
		})
	}
}

func TestResolve_YouTube_HandlePath_ConsentBypass(t *testing.T) {
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
		t.Fatalf("cands = %+v, want the channel's feed: YouTube's cookie-less consent-bypass page names the channel only in its canonical and feed links, not in channelId JSON", cands)
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
				t.Fatalf("wrong channel picked: %s; the first channel ID on the page is the channel's own, since its canonical and feed links precede user content such as links to other channels", r.URL.RawQuery)
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
	finder := New(&http.Client{Transport: roundTripperFunc(func(_ *http.Request) *http.Response {
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
	if len(cands) != 0 {
		t.Errorf("cands = %+v, want none: an error page can mention a UC… string, so only a 2xx channel page is read for the channel ID", cands)
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
		t.Fatalf("requests made = %d, want 2: the channel page and the feed for its title", len(gotUAs))
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

func TestIsYouTubeFeedURL_MatchesTheOutageCount(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv":   true,
		"https://YouTube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv":    true,
		"http://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv":    false,
		"https://www.youtube.com/feeds/videos.xml":                                       false,
		"https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv":                       false,
		"https://publication.example.com/www.youtube.com/feeds/videos.xml?channel_id=UC": false,
	} {
		if got := IsYouTubeFeedURL(raw); got != want {
			t.Errorf("IsYouTubeFeedURL(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestYouTubeFeedsDown_WhenAtLeastHalfAndMoreThanAFewFail(t *testing.T) {
	for _, tc := range []struct {
		total, failing int64
		want           bool
	}{
		{total: 10, failing: 5, want: true},
		{total: 10, failing: 4, want: false},
		{total: 3, failing: 3, want: true},
		{total: 2, failing: 2, want: false},
	} {
		if got := YouTubeFeedsDown(tc.total, tc.failing); got != tc.want {
			t.Errorf("YouTubeFeedsDown(%d, %d) = %v, want %v: an outage fails at least half the followed YouTube feeds at once, and a few dead channels on a small server are no outage", tc.total, tc.failing, got, tc.want)
		}
	}
}

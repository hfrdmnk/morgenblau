package fetcher

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"morgenblau/internal/safehttp"
)

const localRSS = `<?xml version="1.0"?><rss version="2.0"><channel><title>Example Publication</title><link>https://news.example.com/</link><description>d</description>` +
	`<item><title>First post</title><link>https://news.example.com/posts/first</link><guid>https://news.example.com/posts/first</guid></item></channel></rss>`

// onOrigin claims the /feed/ feeds of one origin, the way the server's generated feeds are recognized.
func onOrigin(origin string) func(*url.URL) bool {
	return func(u *url.URL) bool {
		return u.Scheme+"://"+u.Host == origin && strings.HasPrefix(u.Path, "/feed/")
	}
}

func TestLocalFeedsAreAnsweredInProcess(t *testing.T) {
	var hits atomic.Int32
	feeds := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/feed/abc" {
			t.Errorf("in-process path = %q", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = io.WriteString(w, localRSS)
	})
	// Nothing listens on port 1, so only the in-process path can answer.
	f := New(WithLocalFeeds(onOrigin("http://127.0.0.1:1"), feeds))

	res, err := f.Fetch(context.Background(), "http://127.0.0.1:1/feed/abc", FeedState{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Feed == nil || len(res.Feed.Items) != 1 || res.ETag != `"v1"` || res.StatusCode != http.StatusOK {
		t.Fatalf("result = %+v", res)
	}
	res, err = f.Fetch(context.Background(), "http://127.0.0.1:1/feed/abc", FeedState{ETag: `"v1"`})
	if err != nil || !res.NotModified {
		t.Fatalf("conditional fetch = %+v, %v; want 304", res, err)
	}
	if hits.Load() != 2 {
		t.Errorf("handler hits = %d, want 2", hits.Load())
	}
}

func TestEverythingElseStillGoesThroughTheSafeTransport(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, localRSS)
	}))
	defer upstream.Close()
	var hits atomic.Int32
	feeds := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, localRSS)
	})
	f := New(WithLocalFeeds(onOrigin(upstream.URL), feeds))

	for _, raw := range []string{
		upstream.URL + "/feed.xml",
		upstream.URL + "/api/health",
		upstream.URL + "/webfeed",
		strings.Replace(upstream.URL, "http://", "https://", 1) + "/feed/abc",
	} {
		if _, err := f.Fetch(context.Background(), raw, FeedState{}); !errors.Is(err, safehttp.ErrBlockedAddress) {
			t.Errorf("%s: err = %v, want the safe transport's loopback block", raw, err)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("in-process handler served %d requests it does not own", hits.Load())
	}
	if _, err := f.Fetch(context.Background(), upstream.URL+"/feed/abc", FeedState{}); err != nil || hits.Load() != 1 {
		t.Errorf("local feed: err = %v, hits = %d", err, hits.Load())
	}
}

func TestLocalFeedErrorsKeepTheirStatus(t *testing.T) {
	f := New(WithLocalFeeds(onOrigin("http://127.0.0.1:1"), http.NotFoundHandler()))
	_, err := f.Fetch(context.Background(), "http://127.0.0.1:1/feed/missing", FeedState{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
}

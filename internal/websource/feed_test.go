package websource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/database"
)

func serveFeed(t *testing.T, env *testEnv, id string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	return serveFeedPath(t, env, "/feed/"+id, header)
}

func serveFeedPath(t *testing.T, env *testEnv, path string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rr := httptest.NewRecorder()
	env.service.FeedHandler().ServeHTTP(rr, req)
	return rr
}

func TestFeedServesRSSThatGofeedParses(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Items[0].Excerpt = `Fish & <chips> "today"`
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{{URL: "https://news.example.com/posts/fresh", Title: "Fresh"}}}}
	pollDue(t, env)

	rr := serveFeed(t, env, id, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/rss+xml; charset=utf-8" {
		t.Errorf("content type = %q", got)
	}
	feed, err := gofeed.NewParser().ParseString(rr.Body.String())
	if err != nil {
		t.Fatalf("gofeed: %v\n%s", err, rr.Body)
	}
	if feed.FeedType != "rss" || feed.Title != "Example Publication" || feed.Link != "https://news.example.com/" {
		t.Errorf("channel = %s %q %q", feed.FeedType, feed.Title, feed.Link)
	}
	var got []string
	for _, item := range feed.Items {
		date := "-"
		if item.PublishedParsed != nil {
			date = item.PublishedParsed.UTC().Format(time.RFC3339)
		}
		if item.GUID != item.Link {
			t.Errorf("%s: guid = %q, want the link", item.Link, item.GUID)
		}
		got = append(got, fmt.Sprintf("%s|%s|%s", item.Title, item.Link, date))
	}
	want := []string{
		"Fresh|https://news.example.com/posts/fresh|2026-10-10T09:31:00Z",
		"Third post|https://news.example.com/posts/third|2026-10-08T08:00:00Z",
		"First post|https://news.example.com/posts/first|2026-10-06T08:00:00Z",
		"Second post|https://news.example.com/posts/second|-",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("items:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if feed.Items[1].Description != `Fish & <chips> "today"` {
		t.Errorf("description = %q, want the excerpt as escaped plain text", feed.Items[1].Description)
	}
	if !strings.Contains(rr.Body.String(), "Fish &amp; &lt;chips&gt;") {
		t.Errorf("excerpt not escaped in the XML:\n%s", rr.Body)
	}
}

func TestFeedServesTheNewestTwenty(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Items = nil
	for i := range 25 {
		env.gen.generated.Items = append(env.gen.generated.Items, Item{
			URL: fmt.Sprintf("https://news.example.com/posts/%02d", i), Title: fmt.Sprintf("Post %02d", i), Date: day(1).Add(time.Duration(i) * time.Hour),
		})
	}
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != previewItems || preview.ItemCount != 25 {
		t.Errorf("preview has %d items of %d", len(preview.Items), preview.ItemCount)
	}
	feed, err := gofeed.NewParser().ParseString(serveFeed(t, env, preview.Source.ID, nil).Body.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != feedItems || feed.Items[0].Title != "Post 24" || feed.Items[19].Title != "Post 05" {
		t.Errorf("feed has %d items, first %q, last %q", len(feed.Items), feed.Items[0].Title, feed.Items[len(feed.Items)-1].Title)
	}
}

func TestFeedAnswersConditionalGET(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	first := serveFeed(t, env, id, nil)
	etag, lastModified := first.Header().Get("ETag"), first.Header().Get("Last-Modified")
	if etag == "" || lastModified != "Sat, 10 Oct 2026 09:00:00 GMT" {
		t.Fatalf("ETag = %q, Last-Modified = %q", etag, lastModified)
	}
	if rr := serveFeed(t, env, id, http.Header{"If-None-Match": {etag}}); rr.Code != http.StatusNotModified || rr.Body.Len() != 0 {
		t.Errorf("If-None-Match status = %d", rr.Code)
	}
	if rr := serveFeed(t, env, id, http.Header{"If-Modified-Since": {lastModified}}); rr.Code != http.StatusNotModified {
		t.Errorf("If-Modified-Since status = %d", rr.Code)
	}
	if rr := serveFeed(t, env, id, http.Header{"If-None-Match": {`"stale"`}, "If-Modified-Since": {lastModified}}); rr.Code != http.StatusOK {
		t.Errorf("a stale ETag must win over a matching date: status = %d", rr.Code)
	}

	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{{URL: "https://news.example.com/posts/fresh", Title: "Fresh"}}}}
	pollDue(t, env)
	if rr := serveFeed(t, env, id, http.Header{"If-None-Match": {etag}}); rr.Code != http.StatusOK || rr.Header().Get("ETag") == etag {
		t.Errorf("after a new item: status = %d, ETag unchanged = %v", rr.Code, rr.Header().Get("ETag") == etag)
	}
}

func TestFeedAnswersUnknownIDsWithNotFound(t *testing.T) {
	env := newTestEnv(t)
	for _, id := range []string{"aaaaaaaaaaaaaaaaaaaaaaaaaa", "", "a/b"} {
		for _, path := range []string{"/feed/" + id, "/web/" + id} {
			if rr := serveFeedPath(t, env, path, nil); rr.Code != http.StatusNotFound {
				t.Errorf("%s status = %d, want 404", path, rr.Code)
			}
		}
	}
	id := followed(t, env)
	for _, path := range []string{"/" + id, "/api/" + id, "/feeds/" + id, "/webfeed/" + id} {
		if rr := serveFeedPath(t, env, path, nil); rr.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, rr.Code)
		}
	}
}

// Readers who subscribed before generated feeds moved to /feed/ still hold /web/ URLs in their PDS records.
func TestFeedServesTheSameFeedAtTheLegacyPath(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)

	current, legacy := serveFeed(t, env, id, nil), serveFeedPath(t, env, "/web/"+id, nil)
	if current.Code != http.StatusOK || legacy.Code != http.StatusOK {
		t.Fatalf("status /feed/ = %d, /web/ = %d", current.Code, legacy.Code)
	}
	if legacy.Body.String() != current.Body.String() || legacy.Header().Get("ETag") != current.Header().Get("ETag") {
		t.Errorf("/web/%s serves a different feed than /feed/%s", id, id)
	}
}

func TestSameFeedURLsNameBothPathsOfOneOfOurFeeds(t *testing.T) {
	svc := NewService(nil, nil, nil, Config{PublicBaseURL: "https://app.example.com"})
	both := "[https://app.example.com/feed/abc https://app.example.com/web/abc]"
	for raw, want := range map[string]string{
		"https://app.example.com/feed/abc":                        both,
		"https://app.example.com/web/abc":                         both,
		"https://APP.example.com/web/abc":                         both,
		"https://other.example.com/feed/abc":                      "[]",
		"https://app.example.com/feeds/abc":                       "[]",
		"at://did:plc:example/site.standard.publication/3example": "[]",
	} {
		if got := fmt.Sprint(svc.SameFeedURLs(raw)); got != want {
			t.Errorf("SameFeedURLs(%q) = %s, want %s", raw, got, want)
		}
	}
}

func TestFeedURLsBelongToThePublicBase(t *testing.T) {
	svc := NewService(nil, nil, nil, Config{PublicBaseURL: "https://app.example.com/"})
	if got := svc.FeedURL("abc"); got != "https://app.example.com/feed/abc" {
		t.Fatalf("FeedURL = %q", got)
	}
	if got, want := svc.FeedURLPrefixes(), (FeedPrefixes{Current: "https://app.example.com/feed/", Legacy: "https://app.example.com/web/"}); got != want {
		t.Fatalf("FeedURLPrefixes = %+v, want %+v", got, want)
	}
	for raw, want := range map[string]string{
		"https://app.example.com/feed/abc":     "abc",
		"https://APP.example.com/feed/abc":     "abc",
		"http://app.example.com/feed/abc":      "",
		"https://app.example.com/feed/":        "",
		"https://app.example.com/feed/abc/rss": "",
		"https://app.example.com/feeds/abc":    "",
		"https://other.example.com/feed/abc":   "",
		"https://app.example.com/web/abc":      "abc",
		"https://APP.example.com/web/abc":      "abc",
		"HTTPS://app.example.com/web/abc":      "abc",
		"http://app.example.com/web/abc":       "",
		"https://app.example.com/web/":         "",
		"https://app.example.com/web/abc/rss":  "",
		"https://app.example.com/webfeed":      "",
		"https://app.example.com/api/health":   "",
		"https://other.example.com/web/abc":    "",
		"https://app.example.com:8443/web/abc": "",
		"::not a url":                          "",
	} {
		id, ok := svc.IDFromFeedURL(raw)
		if id != want || ok != (want != "") || svc.IsFeedURL(raw) != ok {
			t.Errorf("IDFromFeedURL(%q) = %q, %v; want %q", raw, id, ok, want)
		}
	}
}

func TestAnOutsideFeedReaderKeepsAGeneratedFeedPolledAndKept(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	preview, err := env.service.Create(ctx, testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	id := preview.Source.ID
	read := func(h http.Handler) {
		t.Helper()
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/web/"+id, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
	}
	sweep := func() {
		t.Helper()
		if _, err := database.SweepUnfollowedFeeds(ctx, env.writer, env.now); err != nil {
			t.Fatal(err)
		}
	}

	// Long after the offer: from here only the outside reader can keep the source.
	env.advance(database.FeedSweepGrace + 24*time.Hour)
	read(env.service.FeedHandler())
	if n := pollDue(t, env); n != 0 {
		t.Fatalf("polled %d: Morgenblau's own in-process fetch is not a reader", n)
	}
	read(env.service.PublicFeedHandler())
	if n := pollDue(t, env); n != 1 {
		t.Fatalf("polled %d, want the source an outside feed reader follows", n)
	}

	env.advance(database.FeedSweepGrace - time.Hour)
	sweep()
	if _, err := env.service.read.GetWebSource(ctx, id); err != nil {
		t.Fatalf("a source an outside reader fetched within the grace was swept: %v", err)
	}
	env.advance(2 * time.Hour)
	sweep()
	if _, err := env.service.read.GetWebSource(ctx, id); err == nil {
		t.Error("a source nobody has read for the whole grace survived the sweep")
	}
}

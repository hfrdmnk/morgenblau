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
)

func serveFeed(t *testing.T, env *testEnv, id string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/web/"+id, nil)
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
		if rr := serveFeed(t, env, id, nil); rr.Code != http.StatusNotFound {
			t.Errorf("/web/%s status = %d, want 404", id, rr.Code)
		}
	}
}

package sync

import (
	"reflect"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"morgenblau/internal/database/db"
)

func TestChooseGUID(t *testing.T) {
	cases := []struct {
		name string
		item *gofeed.Item
		want string
	}{
		{name: "guid set", item: &gofeed.Item{GUID: " guid-1 "}, want: "guid-1"},
		{name: "link fallback", item: &gofeed.Item{Link: " https://example.com/post "}, want: "https://example.com/post"},
		{name: "guid wins", item: &gofeed.Item{GUID: "guid-1", Link: "https://example.com/post"}, want: "guid-1"},
		{name: "neither", item: &gofeed.Item{}, want: ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseGUID(tt.item); got != tt.want {
				t.Errorf("chooseGUID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChooseTime(t *testing.T) {
	published := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	fallback := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	// Bluesky's RSS pubDate lacks a weekday prefix and seconds; gofeed can't parse it, see chooseTime's manual fallback.
	bskyOffset := time.Date(2026, 5, 24, 18, 2, 0, 0, time.UTC) // 20:02 +0200
	bskyUTC := time.Date(2026, 5, 25, 14, 43, 0, 0, time.UTC)
	cases := []struct {
		name string
		item *gofeed.Item
		want time.Time
	}{
		{name: "published parsed", item: &gofeed.Item{PublishedParsed: &published, UpdatedParsed: &updated}, want: published},
		{name: "updated fallback", item: &gofeed.Item{UpdatedParsed: &updated}, want: updated},
		{name: "neither", item: &gofeed.Item{}, want: fallback},
		{name: "raw bluesky offset", item: &gofeed.Item{Published: "24 May 2026 20:02 +0200"}, want: bskyOffset},
		{name: "raw bluesky utc", item: &gofeed.Item{Published: "25 May 2026 14:43 +0000"}, want: bskyUTC},
		{name: "raw updated fallback", item: &gofeed.Item{Updated: "24 May 2026 20:02 +0200"}, want: bskyOffset},
		{name: "raw malformed", item: &gofeed.Item{Published: "not a date"}, want: fallback},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseTime(tt.item, fallback); !got.Equal(tt.want) {
				t.Errorf("chooseTime = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestChooseBody(t *testing.T) {
	cases := []struct {
		name string
		item *gofeed.Item
		want string
	}{
		{name: "content set", item: &gofeed.Item{Content: " <p>content</p> ", Description: "desc"}, want: "<p>content</p>"},
		{name: "description fallback", item: &gofeed.Item{Description: " desc "}, want: "desc"},
		{name: "empty", item: &gofeed.Item{}, want: ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseBody(tt.item); got != tt.want {
				t.Errorf("chooseBody = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyContentType(t *testing.T) {
	cases := []struct {
		name    string
		feedURL string
		item    *gofeed.Item
		want    string
	}{
		{name: "youtube host", feedURL: "https://www.youtube.com/feeds/videos.xml", item: &gofeed.Item{Title: "Video"}, want: "video"},
		{name: "youtu.be host", feedURL: "https://youtu.be/feed", item: &gofeed.Item{Title: "Video"}, want: "video"},
		{name: "video enclosure", feedURL: "https://example.com/feed", item: &gofeed.Item{Title: "Video", Enclosures: []*gofeed.Enclosure{{Type: "video/mp4"}}}, want: "video"},
		{name: "empty title", feedURL: "https://example.com/feed", item: &gofeed.Item{}, want: "microblog"},
		{name: "default", feedURL: "https://example.com/feed", item: &gofeed.Item{Title: "Post"}, want: "blogpost"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyContentType(tt.feedURL, tt.item); got != tt.want {
				t.Errorf("classifyContentType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildMetadata(t *testing.T) {
	cases := []struct {
		name string
		item *gofeed.Item
		want map[string]any
	}{
		{name: "empty", item: &gofeed.Item{}, want: map[string]any{}},
		{name: "author and image", item: &gofeed.Item{
			Authors: []*gofeed.Person{{Name: "Example Author"}},
			Image:   &gofeed.Image{URL: "https://example.com/image.jpg"},
		}, want: map[string]any{"author": "Example Author", "image": "https://example.com/image.jpg"}},
		{name: "enclosure", item: &gofeed.Item{
			Enclosures: []*gofeed.Enclosure{{URL: "https://example.com/audio.mp3", Type: "audio/mpeg"}},
		}, want: map[string]any{"enclosure": map[string]any{"url": "https://example.com/audio.mp3", "type": "audio/mpeg"}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildMetadata(tt.item); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildMetadata = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestShouldDiscoverIcon(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	fresh := now.Add(-iconRefreshAfter + time.Hour).Format(time.RFC3339)
	stale := now.Add(-iconRefreshAfter - time.Hour).Format(time.RFC3339)
	icon := "https://example.com/favicon.ico"
	empty := ""
	malformed := "not-a-time"
	cases := []struct {
		name string
		feed db.Feed
		want bool
	}{
		{name: "nil url", feed: db.Feed{}, want: true},
		{name: "empty url", feed: db.Feed{IconUrl: &empty}, want: true},
		{name: "nil fetched at", feed: db.Feed{IconUrl: &icon}, want: true},
		{name: "malformed fetched at", feed: db.Feed{IconUrl: &icon, IconFetchedAt: &malformed}, want: true},
		{name: "fresh", feed: db.Feed{IconUrl: &icon, IconFetchedAt: &fresh}, want: false},
		{name: "stale", feed: db.Feed{IconUrl: &icon, IconFetchedAt: &stale}, want: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldDiscoverIcon(tt.feed, now); got != tt.want {
				t.Errorf("shouldDiscoverIcon = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNilIfEmpty(t *testing.T) {
	if got := nilIfEmpty(""); got != nil {
		t.Errorf("nilIfEmpty(empty) = %v, want nil", got)
	}
	if got := nilIfEmpty("x"); got == nil || *got != "x" {
		t.Errorf("nilIfEmpty(non-empty) = %v, want x", got)
	}
}

func TestEntrySlug(t *testing.T) {
	if got := EntrySlug("https://example.com/feed", "guid-1"); got != "WZw5GD1LIL" {
		t.Errorf("EntrySlug = %q, want frozen vector", got)
	}
}

package feedkey

import (
	"context"
	"errors"
	"testing"
)

type fakeReactionResolver struct {
	byGuid    map[string]string
	byItemURL map[string]string
}

func (f fakeReactionResolver) GetFeedURLByGuid(_ context.Context, guid string) (string, error) {
	if fu, ok := f.byGuid[guid]; ok {
		return fu, nil
	}
	return "", errors.New("not found")
}

func (f fakeReactionResolver) GetFeedURLByItemURL(_ context.Context, url string) (string, error) {
	if fu, ok := f.byItemURL[url]; ok {
		return fu, nil
	}
	return "", errors.New("not found")
}

func TestResolveReactionKey_TableDriven(t *testing.T) {
	resolver := fakeReactionResolver{
		byGuid:    map[string]string{"guid-1": "https://example.com/feed/"},
		byItemURL: map[string]string{"https://example.com/posts/1": "https://example.com/other-feed"},
	}

	cases := []struct {
		name     string
		feedURL  string
		document string
		itemURL  string
		wantKey  string
		wantOK   bool
	}{
		{
			name:     "feedUrl provenance wins over document and itemUrl, normalized",
			feedURL:  "HTTP://Example.COM:80/feed/",
			document: "guid-1",
			itemURL:  "https://example.com/posts/1",
			wantKey:  "http://example.com/feed",
			wantOK:   true,
		},
		{
			name:     "document lookup wins over itemUrl fallback, normalized",
			document: "guid-1",
			itemURL:  "https://example.com/posts/1",
			wantKey:  "https://example.com/feed",
			wantOK:   true,
		},
		{
			name:    "itemUrl lookup used when document is empty, normalized",
			itemURL: "https://example.com/posts/1",
			wantKey: "https://example.com/other-feed",
			wantOK:  true,
		},
		{
			name:     "itemUrl fallback used when document lookup misses, normalized",
			document: "missing-guid",
			itemURL:  "https://example.com/posts/1",
			wantKey:  "https://example.com/other-feed",
			wantOK:   true,
		},
		{
			name:     "nothing resolves",
			document: "missing-guid",
			itemURL:  "https://example.com/posts/unknown",
			wantOK:   false,
		},
		{
			name:   "all inputs empty",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := ResolveReactionKey(context.Background(), resolver, tc.feedURL, tc.document, tc.itemURL)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if key != tc.wantKey {
				t.Errorf("key = %q, want %q", key, tc.wantKey)
			}
		})
	}
}

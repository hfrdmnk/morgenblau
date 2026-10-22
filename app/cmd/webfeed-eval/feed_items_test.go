package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFeedItemsDropsItemsWithoutALink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Example Publication</title>
<item><title>Linked</title><link>/posts/linked</link></item>
<item><title>No link</title></item>
<item><title>Blank link</title><link>  </link></item>
</channel></rss>`))
	}))
	defer srv.Close()
	items, err := feedItems(context.Background(), srv.Client(), srv.URL+"/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].URL != srv.URL+"/posts/linked" {
		t.Errorf("items = %+v, want only the linked post", items)
	}
}

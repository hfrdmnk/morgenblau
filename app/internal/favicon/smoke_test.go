package favicon

import (
	"context"
	"os"
	"testing"
)

func TestDiscoverFixtureSite(t *testing.T) {
	body, err := os.ReadFile("testdata/sample-site.html")
	if err != nil {
		t.Fatal(err)
	}
	site := newFakeSite(map[string]route{
		"/":                {contentType: "text/html; charset=utf-8", body: string(body)},
		"/assets/icon.svg": {contentType: "image/svg+xml", body: "<svg/>"},
		"/favicon.ico":     {status: 404},
	})
	defer site.Close()

	got, err := Discover(context.Background(), site.Client(), site.URL)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := site.URL + "/assets/icon.svg"
	if got != want {
		t.Fatalf("icon = %q, want %q", got, want)
	}
}

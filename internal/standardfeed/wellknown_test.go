package standardfeed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchWellKnown(t *testing.T) {
	pubURI := "at://did:plc:pub123/site.standard.publication/3abc"
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"hit", http.StatusOK, pubURI, pubURI},
		{"hit with whitespace", http.StatusOK, "  " + pubURI + "\n", pubURI},
		{"404 miss", http.StatusNotFound, "not found", ""},
		{"html garbage", http.StatusOK, "<!doctype html><html></html>", ""},
		{"wrong collection", http.StatusOK, "at://did:plc:pub123/app.bsky.feed.post/3abc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client := NewClient(&fakeResolver{}, srv.Client())
			got, err := client.FetchWellKnown(context.Background(), srv.URL+"/some/article")
			if err != nil {
				t.Fatalf("FetchWellKnown: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if gotPath != "/.well-known/site.standard.publication" {
				t.Fatalf("probe path = %q", gotPath)
			}
		})
	}
}

func TestFetchWellKnown_UnparsableURL(t *testing.T) {
	client := NewClient(&fakeResolver{}, nil)
	got, err := client.FetchWellKnown(context.Background(), "://not-a-url")
	if err != nil || got != "" {
		t.Fatalf("got (%q, %v), want empty miss", got, err)
	}
}

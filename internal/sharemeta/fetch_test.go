package sharemeta

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"morgenblau/internal/standardfeed"
)

const (
	testDocumentURI    = "at://did:plc:publisher/site.standard.document/3doc"
	testPublicationURI = "at://did:plc:publisher/site.standard.publication/self"
)

type fakeStandardfeedClient struct {
	document    *standardfeed.Document
	publication *standardfeed.Publication
	docErr      error
	pubErr      error
}

func (f *fakeStandardfeedClient) GetDocument(context.Context, string) (*standardfeed.Document, error) {
	return f.document, f.docErr
}

func (f *fakeStandardfeedClient) GetPublication(context.Context, string) (*standardfeed.Publication, error) {
	return f.publication, f.pubErr
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetcher_WebPagePrefersOpenGraphTitleAndUsesFinalURL(t *testing.T) {
	finalURL, _ := url.Parse("https://example.com/final")
	doer := httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Accept") != "text/html,application/xhtml+xml" {
			t.Errorf("Accept = %q", req.Header.Get("Accept"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body: io.NopCloser(strings.NewReader(`
				<html><head>
					<title>Document title</title>
					<meta name="twitter:title" content="Twitter title">
					<meta property="og:title" content="  The   Open Graph
					Title  ">
				</head></html>
			`)),
			Request: &http.Request{URL: finalURL},
		}, nil
	})

	got, err := NewFetcher(nil, doer).Fetch(context.Background(), Target{ItemURL: "https://example.com/original"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Title != "The Open Graph Title" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.TargetURL != finalURL.String() {
		t.Errorf("TargetURL = %q, want %q", got.TargetURL, finalURL)
	}
}

func TestFetcher_WebPageTitleFallbackOrder(t *testing.T) {
	tests := []struct {
		name string
		head string
		want string
	}{
		{
			name: "twitter before document title",
			head: `<title>Document title</title><meta name="twitter:title" content="Twitter title">`,
			want: "Twitter title",
		},
		{
			name: "document title",
			head: `<title>  Document
				title  </title>`,
			want: "Document title",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("<html><head>" + tt.head + "</head></html>")),
				}, nil
			})
			got, err := NewFetcher(nil, doer).Fetch(context.Background(), Target{ItemURL: "https://example.com/post"})
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if got.Title != tt.want {
				t.Errorf("Title = %q, want %q", got.Title, tt.want)
			}
		})
	}
}

func TestFetcher_WebPageBoundsHTMLAndNormalizedTitle(t *testing.T) {
	longTitle := strings.Repeat("é", maxTitleRunes+20)
	doer := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader(`<title>` + longTitle + `</title>`)),
		}, nil
	})
	got, err := NewFetcher(nil, doer).Fetch(context.Background(), Target{ItemURL: "https://example.com/post"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len([]rune(got.Title)) != maxTitleRunes {
		t.Errorf("title runes = %d, want %d", len([]rune(got.Title)), maxTitleRunes)
	}

	oversized := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxHTMLBytes+1))),
		}, nil
	})
	if _, err := NewFetcher(nil, oversized).Fetch(context.Background(), Target{ItemURL: "https://example.com/post"}); err == nil {
		t.Fatal("expected oversized HTML error")
	}
}

func TestFetcher_StandardfeedDocumentResolvesPublicationURL(t *testing.T) {
	client := &fakeStandardfeedClient{
		document: &standardfeed.Document{
			URI:   testDocumentURI,
			Site:  testPublicationURI,
			Title: "A Standardfeed Post",
			Path:  "posts/hello",
		},
		publication: &standardfeed.Publication{
			URI: testPublicationURI,
			URL: "https://publication.example/",
		},
	}

	got, err := NewFetcher(client, nil).Fetch(context.Background(), Target{
		Document: testDocumentURI,
		ItemURL:  "https://sidecar.example/ignored",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Title != "A Standardfeed Post" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.TargetURL != "https://publication.example/posts/hello" {
		t.Errorf("TargetURL = %q", got.TargetURL)
	}
}

func TestFetcher_PathlessDocumentStillReturnsTitle(t *testing.T) {
	client := &fakeStandardfeedClient{
		document: &standardfeed.Document{
			URI:   testDocumentURI,
			Site:  testPublicationURI,
			Title: "Pathless",
		},
		publication: &standardfeed.Publication{URI: testPublicationURI, URL: "https://publication.example"},
	}

	got, err := NewFetcher(client, nil).Fetch(context.Background(), Target{Document: testDocumentURI})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Title != "Pathless" || got.TargetURL != "" {
		t.Errorf("got = %+v, want title with no target URL", got)
	}
}

func TestFetcher_RejectsNonHTTPItemURL(t *testing.T) {
	_, err := NewFetcher(nil, nil).Fetch(context.Background(), Target{ItemURL: "file:///etc/passwd"})
	if err == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestFetcher_PropagatesStandardfeedFailure(t *testing.T) {
	client := &fakeStandardfeedClient{docErr: errors.New("upstream unavailable")}
	_, err := NewFetcher(client, nil).Fetch(context.Background(), Target{Document: testDocumentURI})
	if err == nil {
		t.Fatal("expected upstream error")
	}
}

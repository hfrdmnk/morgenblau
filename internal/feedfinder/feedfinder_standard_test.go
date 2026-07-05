package feedfinder

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"morgenblau/internal/standardfeed"
)

const (
	pubURI    = "at://did:plc:publisher/site.standard.publication/3pub"
	docURI    = "at://did:plc:publisher/site.standard.document/3doc"
	handlePub = "at://blog.example.test/site.standard.publication/3pub"
)

// fakeStandard is a canned StandardResolver. Publications are keyed by the
// REQUESTED uri (handle- or DID-form) and return the DID-normalized shape,
// mirroring the real client's normalization.
type fakeStandard struct {
	pubs         map[string]*standardfeed.Publication
	docs         map[string]*standardfeed.Document
	wellKnown    map[string]string // origin → publication at-uri
	wellKnownErr error
}

func (f *fakeStandard) GetPublication(_ context.Context, uri string) (*standardfeed.Publication, error) {
	if pub, ok := f.pubs[uri]; ok {
		return pub, nil
	}
	return nil, errors.New("publication not found")
}

func (f *fakeStandard) GetDocument(_ context.Context, uri string) (*standardfeed.Document, error) {
	if doc, ok := f.docs[uri]; ok {
		return doc, nil
	}
	return nil, errors.New("document not found")
}

func (f *fakeStandard) FetchWellKnown(_ context.Context, siteURL string) (string, error) {
	if f.wellKnownErr != nil {
		return "", f.wellKnownErr
	}
	return f.wellKnown[siteURL], nil
}

func normalizedPub() *standardfeed.Publication {
	return &standardfeed.Publication{
		URI:  pubURI,
		DID:  "did:plc:publisher",
		Name: "Example Publication",
		URL:  "https://blog.example.test",
	}
}

func htmlTransport(t *testing.T, body string) HTTPDoer {
	t.Helper()
	return &http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		return resp(body, "text/html; charset=utf-8")
	})}
}

func TestResolve_WellKnownHit_MergesWithLinkRels(t *testing.T) {
	std := &fakeStandard{
		pubs:      map[string]*standardfeed.Publication{pubURI: normalizedPub()},
		wellKnown: map[string]string{"https://example.test": pubURI},
	}
	finder := New(htmlTransport(t, htmlWithFeeds)).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), "https://example.test/blog")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 4 {
		t.Fatalf("got %d candidates, want 3 rss + 1 standardfeed: %+v", len(cands), cands)
	}
	std1 := cands[3]
	if std1.Kind != "standardfeed" || std1.Publication != pubURI {
		t.Errorf("standard candidate = %+v", std1)
	}
	if std1.Title != "Example Publication" || std1.SiteURL != "https://blog.example.test" {
		t.Errorf("standard candidate metadata = %+v", std1)
	}
	if std1.FeedURL != "" {
		t.Errorf("standard candidate must not carry a feedUrl: %q", std1.FeedURL)
	}
}

func TestResolve_WellKnownMissOrError_RSSOnly(t *testing.T) {
	cases := map[string]*fakeStandard{
		"miss":            {wellKnown: map[string]string{}},
		"transport error": {wellKnownErr: errors.New("timeout")},
	}
	for name, std := range cases {
		t.Run(name, func(t *testing.T) {
			finder := New(htmlTransport(t, htmlWithFeeds)).WithStandardResolver(std)
			cands, err := finder.Resolve(context.Background(), "https://example.test/blog")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if len(cands) != 3 {
				t.Fatalf("got %d candidates, want rss-only: %+v", len(cands), cands)
			}
			for _, c := range cands {
				if c.Kind == "standardfeed" {
					t.Errorf("unexpected standardfeed candidate: %+v", c)
				}
			}
		})
	}
}

func TestResolve_ATURIPassthrough_Publication(t *testing.T) {
	std := &fakeStandard{pubs: map[string]*standardfeed.Publication{pubURI: normalizedPub()}}
	// Any HTTP request would be a bug — at-uris resolve via the PDS client.
	finder := New(&http.Client{Transport: roundTripperFunc(func(r *http.Request) *http.Response {
		t.Fatalf("unexpected HTTP request: %s", r.URL)
		return nil
	})}).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 || cands[0].Publication != pubURI || cands[0].Kind != "standardfeed" {
		t.Fatalf("candidates = %+v", cands)
	}
}

func TestResolve_ATURIPassthrough_Document(t *testing.T) {
	std := &fakeStandard{
		pubs: map[string]*standardfeed.Publication{pubURI: normalizedPub()},
		docs: map[string]*standardfeed.Document{docURI: {URI: docURI, Site: pubURI, Title: "Post"}},
	}
	finder := New(http.DefaultClient).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), docURI)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 1 || cands[0].Publication != pubURI {
		t.Fatalf("candidates = %+v", cands)
	}
}

func TestResolve_ATURIPassthrough_LooseDocumentNoCandidate(t *testing.T) {
	std := &fakeStandard{
		docs: map[string]*standardfeed.Document{docURI: {URI: docURI, Site: "https://loose.example.test", Title: "Post"}},
	}
	finder := New(http.DefaultClient).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), docURI)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("loose document must yield no candidates: %+v", cands)
	}
}

func TestResolve_ATURIUnknownCollection_NoCandidates(t *testing.T) {
	std := &fakeStandard{}
	finder := New(http.DefaultClient).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), "at://did:plc:x/app.bsky.feed.post/3abc")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("candidates = %+v", cands)
	}
}

func TestResolve_ATURIWithNilResolver_Empty(t *testing.T) {
	finder := New(http.DefaultClient)
	cands, err := finder.Resolve(context.Background(), pubURI)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("candidates = %+v", cands)
	}
}

const htmlWithDocLink = `<!doctype html><html><head>
<link rel="alternate" type="application/rss+xml" title="Posts" href="/feed.xml">
<link rel="site.standard.document" href="` + docURI + `">
</head><body></body></html>`

func TestResolve_ArticleDocLinkTag_ResolvesPublication(t *testing.T) {
	std := &fakeStandard{
		pubs: map[string]*standardfeed.Publication{pubURI: normalizedPub()},
		docs: map[string]*standardfeed.Document{docURI: {URI: docURI, Site: pubURI, Title: "Post"}},
	}
	finder := New(htmlTransport(t, htmlWithDocLink)).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), "https://blog.example.test/posts/hello")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want rss + standardfeed: %+v", len(cands), cands)
	}
	if cands[1].Kind != "standardfeed" || cands[1].Publication != pubURI {
		t.Errorf("standard candidate = %+v", cands[1])
	}
}

func TestResolve_WellKnownAndDocLink_DedupeByNormalizedURI(t *testing.T) {
	// Well-known returns the handle-form uri, the document's site is the
	// DID-form — both normalize to the same publication.
	std := &fakeStandard{
		pubs: map[string]*standardfeed.Publication{
			pubURI:    normalizedPub(),
			handlePub: normalizedPub(),
		},
		docs:      map[string]*standardfeed.Document{docURI: {URI: docURI, Site: pubURI, Title: "Post"}},
		wellKnown: map[string]string{"https://blog.example.test": handlePub},
	}
	finder := New(htmlTransport(t, htmlWithDocLink)).WithStandardResolver(std)

	cands, err := finder.Resolve(context.Background(), "https://blog.example.test/posts/hello")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	standard := 0
	for _, c := range cands {
		if c.Kind == "standardfeed" {
			standard++
		}
	}
	if standard != 1 {
		t.Fatalf("standardfeed candidates = %d, want deduped 1: %+v", standard, cands)
	}
}

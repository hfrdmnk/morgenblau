package webfeed

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// appShell is a client-rendered listing: an empty mount point and the script that fills it.
func appShell() string {
	return `<!doctype html><html lang="en"><head><title>Example Publication</title></head><body><div id="root"></div>` +
		`<script type="module" src="/assets/index-4f2a.js"></script></body></html>`
}

func TestSetupReportsAnAppShellAsBrowserBuiltWithoutAskingTheModel(t *testing.T) {
	s := newTestSite(t)
	s.set("/", appShell())
	m := &fakeModel{}
	opts := s.opts()
	opts.Model = m

	res, err := Setup(context.Background(), s.url("/"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.BrowserBuilt() || len(res.Items) != 0 {
		t.Fatalf("browser built %v reasons %v items %v", res.BrowserBuilt(), res.Reasons, itemURLs(res.Items))
	}
	if len(m.prompts) != 0 {
		t.Fatalf("model asked %d times", len(m.prompts))
	}
}

func TestSetupReportsAnUnloadedLazyListAsBrowserBuiltBeforeTheJudge(t *testing.T) {
	l := lazyBlog(t, 6)
	l.postStatus = http.StatusInternalServerError
	m := &fakeModel{replies: map[Effort][]string{EffortHigh: {`{"pick":[0],"order":"page"}`}}}
	opts := l.opts()
	opts.Model = m

	res, err := Setup(context.Background(), l.url("/blog"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.BrowserBuilt() || len(res.Items) != 0 {
		t.Fatalf("browser built %v reasons %v items %v", res.BrowserBuilt(), res.Reasons, itemURLs(res.Items))
	}
	if len(m.prompts) != 0 {
		t.Fatalf("judge asked %d times about a list the page never loaded", len(m.prompts))
	}
}

func TestSetupKeepsAServedListBesideALazyWidget(t *testing.T) {
	l := newLazySite(t)
	l.addBlog(6)
	l.postStatus = http.StatusInternalServerError
	widget := placeholder("x-intersect", lazyParam)
	script := `<script src="/livewire/livewire.min.js?id=1a2b" data-csrf="" data-update-uri="/livewire/update"></script>`
	l.set("/blog", page("Blog | Example Publication", siteNav+blogList(6)+`<aside>`+widget+`</aside>`+siteFooter+script))

	res, err := Setup(context.Background(), l.url("/blog"), l.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.BrowserBuilt() || !res.Healthy() || len(res.Items) != 6 {
		t.Fatalf("browser built %v reasons %v items %v", res.BrowserBuilt(), res.Reasons, itemURLs(res.Items))
	}
}

func TestSetupKeepsAShortServedListInsideAnAppMountPoint(t *testing.T) {
	s := newTestSite(t)
	list := ""
	for i, slug := range []string{"boxes", "vets", "links", "notes"} {
		list += fmt.Sprintf(`<li><a href="/blog/%s">On %s</a> <time datetime="2026-0%d-02">2026-0%d-02</time></li>`, slug, slug, i+1, i+1)
	}
	s.set("/blog", `<!doctype html><html lang="en"><head><title>Blog</title></head><body><div id="app"><ul>`+list+`</ul></div></body></html>`)

	res, err := Setup(context.Background(), s.url("/blog"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.BrowserBuilt() || len(res.Items) != 4 {
		t.Fatalf("browser built %v reasons %v items %v", res.BrowserBuilt(), res.Reasons, itemURLs(res.Items))
	}
}

func TestSetupLeavesAWeakListInAnAppRootUnsureRatherThanBrowserBuilt(t *testing.T) {
	s := newTestSite(t)
	s.set("/", `<!doctype html><html lang="en"><head><title>Example Person</title></head><body><div id="__next"><nav>`+
		`<a href="/projects/lamp">Lamp</a> <a href="/projects/kite">Kite</a></nav></div></body></html>`)

	res, err := Setup(context.Background(), s.url("/"), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.BrowserBuilt() {
		t.Fatalf("a page that serves its links was called browser built: reasons %v", res.Reasons)
	}
}

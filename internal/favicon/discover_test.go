package favicon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSite is a minimal multi-route httptest server: GET /<path> returns the
// configured (status, content-type, body). Missing routes 404.
type fakeSite struct {
	*httptest.Server
	routes map[string]route
}

type route struct {
	status      int
	contentType string
	body        string
	location    string // for redirects
}

func newFakeSite(routes map[string]route) *fakeSite {
	mux := http.NewServeMux()
	s := &fakeSite{routes: routes}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rt, ok := s.routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if rt.location != "" {
			w.Header().Set("Location", rt.location)
		}
		if rt.contentType != "" {
			w.Header().Set("Content-Type", rt.contentType)
		}
		status := rt.status
		if status == 0 {
			status = 200
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(rt.body))
	})
	s.Server = httptest.NewServer(mux)
	return s
}

func htmlWithHead(head string) string {
	return "<!doctype html><html><head>" + head + "</head><body>hi</body></html>"
}

func TestDiscover(t *testing.T) {
	const pngCT = "image/png"
	const svgCT = "image/svg+xml"
	const icoCT = "image/x-icon"
	const octet = "application/octet-stream"

	cases := []struct {
		name      string
		routes    map[string]route
		wantSlug  string // substring expected in returned URL
		wantError bool
	}{
		{
			name: "basic rel=icon",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="/favicon.ico">`)},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "shortcut icon",
			routes: map[string]route{
				"/":             {contentType: "text/html", body: htmlWithHead(`<link rel="shortcut icon" href="/short.ico">`)},
				"/short.ico":    {contentType: icoCT, body: "ICO"},
				"/favicon.ico":  {status: 404},
			},
			wantSlug: "/short.ico",
		},
		{
			name: "apple-touch-icon wins over plain icon",
			routes: map[string]route{
				"/":          {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="/i.png"><link rel="apple-touch-icon" href="/a.png">`)},
				"/a.png":     {contentType: pngCT, body: "PNG"},
				"/i.png":     {contentType: pngCT, body: "PNG"},
			},
			wantSlug: "/a.png",
		},
		{
			name: "precomposed wins over apple-touch-icon",
			routes: map[string]route{
				"/":          {contentType: "text/html", body: htmlWithHead(`<link rel="apple-touch-icon" href="/a.png"><link rel="apple-touch-icon-precomposed" href="/p.png">`)},
				"/a.png":     {contentType: pngCT, body: "PNG"},
				"/p.png":     {contentType: pngCT, body: "PNG"},
			},
			wantSlug: "/p.png",
		},
		{
			name: "SVG beats everything",
			routes: map[string]route{
				"/":         {contentType: "text/html", body: htmlWithHead(`<link rel="icon" type="image/svg+xml" href="/i.svg"><link rel="apple-touch-icon" href="/a.png">`)},
				"/i.svg":    {contentType: svgCT, body: "<svg/>"},
				"/a.png":    {contentType: pngCT, body: "PNG"},
			},
			wantSlug: "/i.svg",
		},
		{
			name: "largest sizes wins among same rel",
			routes: map[string]route{
				"/":         {contentType: "text/html", body: htmlWithHead(`<link rel="icon" sizes="32x32" href="/32.png"><link rel="icon" sizes="192x192" href="/192.png">`)},
				"/192.png":  {contentType: pngCT, body: "PNG"},
				"/32.png":   {contentType: pngCT, body: "PNG"},
			},
			wantSlug: "/192.png",
		},
		{
			name: "relative href resolves against site",
			routes: map[string]route{
				"/":           {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="assets/icon.png">`)},
				"/assets/icon.png": {contentType: pngCT, body: "PNG"},
			},
			wantSlug: "/assets/icon.png",
		},
		{
			name: "no rel=icon falls back to /favicon.ico",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(``)},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "site 404 still tries /favicon.ico",
			routes: map[string]route{
				"/":            {status: 404},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "octet-stream content-type accepted",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="/favicon.ico">`)},
				"/favicon.ico": {contentType: octet, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "data: href is skipped",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="data:image/png;base64,AAA">`)},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "empty href is skipped",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="">`)},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "candidate 404 then fallback validation 404 returns error",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="/missing.ico">`)},
				"/missing.ico": {status: 404},
				"/favicon.ico": {status: 404},
			},
			wantError: true,
		},
		{
			name: "candidate 404 falls back to /favicon.ico",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="/missing.ico">`)},
				"/missing.ico": {status: 404},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "mask-icon is ignored (Safari pinned-tab, would render black)",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="mask-icon" href="/safari.svg" color="#000"><link rel="icon" href="/real.png">`)},
				"/safari.svg":  {contentType: svgCT, body: "<svg/>"},
				"/real.png":    {contentType: pngCT, body: "PNG"},
			},
			wantSlug: "/real.png",
		},
		{
			name: "mask-icon only → falls back to /favicon.ico, not the mask-icon",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="mask-icon" href="/safari.svg" color="#000">`)},
				"/safari.svg":  {contentType: svgCT, body: "<svg/>"},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "case-insensitive rel match",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link REL="ICON" HREF="/favicon.ico">`)},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
		{
			name: "wrong content-type on candidate rejects it, falls back",
			routes: map[string]route{
				"/":            {contentType: "text/html", body: htmlWithHead(`<link rel="icon" href="/notimg.html">`)},
				"/notimg.html": {contentType: "text/html", body: "<p>nope</p>"},
				"/favicon.ico": {contentType: icoCT, body: "ICO"},
			},
			wantSlug: "/favicon.ico",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			site := newFakeSite(tc.routes)
			defer site.Close()

			got, err := Discover(context.Background(), site.Client(), site.URL)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got URL %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(got, tc.wantSlug) {
				t.Fatalf("got %q, want substring %q", got, tc.wantSlug)
			}
			if !strings.HasPrefix(got, "http://") && !strings.HasPrefix(got, "https://") {
				t.Fatalf("returned URL not absolute: %q", got)
			}
		})
	}
}

func TestDiscoverInvalidSiteURL(t *testing.T) {
	_, err := Discover(context.Background(), http.DefaultClient, "not a url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	_, err = Discover(context.Background(), http.DefaultClient, "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

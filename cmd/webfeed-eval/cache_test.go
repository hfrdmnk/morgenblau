package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCachingDoerServesRepeatsFromDisk(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		hits++
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<p>hello "+r.Header.Get("Accept-Language")+"</p>")
	}))
	t.Cleanup(srv.Close)
	d := cachingDoer{dir: t.TempDir(), next: srv.Client()}
	get := func(lang string) (string, string) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/old", nil)
		req.Header.Set("Accept-Language", lang)
		resp, err := d.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), resp.Request.URL.Path
	}
	first, _ := get("en")
	second, final := get("en")
	if hits != 1 || first != second || final != "/new" {
		t.Fatalf("hits %d, bodies %q %q, final %q", hits, first, second, final)
	}
	if body, _ := get("de"); hits != 2 || body != "<p>hello de</p>" {
		t.Fatalf("a different Accept-Language must miss the cache: hits %d body %q", hits, body)
	}
}

func TestParseOptionsCacheDir(t *testing.T) {
	opts, err := parseOptions([]string{"-cache", "/tmp/x", "sites.json"})
	if err != nil || opts.cacheDir != "/tmp/x" {
		t.Fatalf("opts %+v err %v", opts, err)
	}
}

func TestCachingDoerDoesNotPinTransientRefusals(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	d := cachingDoer{dir: t.TempDir(), next: srv.Client()}
	for range 2 {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		resp, err := d.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if hits != 2 {
		t.Fatalf("403 was cached: %d upstream hits", hits)
	}
}

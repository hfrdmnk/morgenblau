package server

import (
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"morgenblau/frontend"
)

const (
	hashedAssetCache = "public, max-age=31536000, immutable"
	staticFileCache  = "public, max-age=3600"
	// no-store, unlike no-cache, keeps the page out of the back/forward cache.
	// https://web.dev/articles/bfcache#minimize-use-of-cache-control-no-store
	shellCache = "no-cache"
)

func spaHandler() http.Handler {
	if os.Getenv("APP_ENV") == "local" {
		return viteProxyHandler()
	}
	return embeddedDistHandler()
}

func viteProxyHandler() http.Handler {
	target := os.Getenv("VITE_URL")
	if target == "" {
		target = "http://localhost:5173"
	}
	u, err := url.Parse(target)
	if err != nil {
		panic("invalid VITE_URL: " + err.Error())
	}
	return httputil.NewSingleHostReverseProxy(u)
}

func embeddedDistHandler() http.Handler {
	dist, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		panic("frontend/dist not embedded: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")

		if clean != "" {
			if _, err := fs.Stat(dist, clean); err == nil {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					w.Header().Set("Cache-Control", hashedAssetCache)
				} else {
					w.Header().Set("Cache-Control", staticFileCache)
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Cache-Control", shellCache)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

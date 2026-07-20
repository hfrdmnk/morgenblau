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

// spaHandler reverse-proxies to Vite in local dev (for HMR); otherwise serves the embedded build.
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

		// Vite content-hashes /assets/* so those cache forever; other files get a modest TTL.
		if clean != "" {
			if _, err := fs.Stat(dist, clean); err == nil {
				if strings.HasPrefix(r.URL.Path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "public, max-age=3600")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Unknown paths fall back to index.html; no-cache (not no-store) forces revalidation for new deploys while keeping the page bfcache-eligible.
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

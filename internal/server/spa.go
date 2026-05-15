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

// spaHandler serves the React frontend. In local dev it reverse-proxies to the
// Vite dev server so HMR works; in any other env it serves the embedded build.
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
		if clean == "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// If the requested file exists in dist, serve it; otherwise fall back
		// to index.html so client-side routes resolve.
		if _, err := fs.Stat(dist, clean); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

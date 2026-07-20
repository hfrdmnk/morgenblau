package api

import (
	"context"
	"log/slog"
	"net/http"

	"morgenblau/internal/discoverposts"
)

// DiscoverSourcePostsReader is the narrow slice of *discoverposts.CachedFetcher the handler needs.
type DiscoverSourcePostsReader interface {
	FetchPosts(ctx context.Context, key string) ([]discoverposts.Post, error)
}

// DiscoverPostWire is one post preview item.
type DiscoverPostWire struct {
	Title       string `json:"title"`
	PublishedAt string `json:"publishedAt,omitempty"`
	URL         string `json:"url,omitempty"`
	Key         string `json:"key"`
}

// DiscoverSourcePostsHandler serves a candidate's newest-posts preview. A fetch failure degrades to an
// empty list rather than an error status: a preview must never fail a discover card. SPEC <discovery>.
func DiscoverSourcePostsHandler(reader DiscoverSourcePostsReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := requireSession(w, r)
		if !ok {
			return
		}

		key := r.URL.Query().Get("key")
		if key == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "key is required")
			return
		}

		posts, err := reader.FetchPosts(r.Context(), key)
		if err != nil {
			slog.Warn("/api/discover/sources/posts: fetch failed", "key", key, "err", err)
			writeJSON(w, []DiscoverPostWire{})
			return
		}

		out := make([]DiscoverPostWire, 0, len(posts))
		for _, p := range posts {
			out = append(out, DiscoverPostWire{
				Title:       p.Title,
				PublishedAt: p.PublishedAt,
				URL:         p.URL,
				Key:         p.Key,
			})
		}
		writeJSON(w, out)
	})
}

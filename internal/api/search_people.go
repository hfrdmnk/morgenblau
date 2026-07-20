package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"morgenblau/internal/personsearch"
)

// PersonSearcher is the search seam; *personsearch.Searcher satisfies it.
type PersonSearcher interface {
	Search(ctx context.Context, q string) ([]personsearch.Result, error)
}

// SearchPersonWire is one whole-network search hit, badged with reader-network presence. SPEC <discovery> People "Search".
type SearchPersonWire struct {
	DID             string   `json:"did"`
	Handle          string   `json:"handle"`
	DisplayName     string   `json:"displayName"`
	Avatar          string   `json:"avatar"`
	InReaderNetwork bool     `json:"inReaderNetwork"`
	TasteHint       []string `json:"tasteHint,omitempty"`
}

// SearchPeopleHandler runs a whole-network person search (SPEC <discovery> People "Search"). An empty q is a client error; an upstream typeahead failure is a gateway error, since there's no local fallback to degrade to.
func SearchPeopleHandler(searcher PersonSearcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireSession(w, r); !ok {
			return
		}

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "q is required")
			return
		}

		results, err := searcher.Search(r.Context(), q)
		if err != nil {
			slog.Warn("/api/search/people: search failed", "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "search upstream failed")
			return
		}

		out := make([]SearchPersonWire, 0, len(results))
		for _, res := range results {
			out = append(out, SearchPersonWire{
				DID:             res.DID,
				Handle:          res.Handle,
				DisplayName:     res.DisplayName,
				Avatar:          res.Avatar,
				InReaderNetwork: res.InReaderNetwork,
				TasteHint:       res.TasteHint,
			})
		}
		writeJSON(w, out)
	})
}

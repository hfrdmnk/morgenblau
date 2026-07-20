package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/sharemeta"
)

// LibraryFollowsReader is the slice of *db.Queries the network-shares handler reads the follow graph from.
type LibraryFollowsReader interface {
	ListUserFollows(ctx context.Context, did string) ([]db.UserFollow, error)
}

// NetworkShareCrawler is the slice of *discovercrawl.CachedShareCrawler the handler depends on.
type NetworkShareCrawler interface {
	FetchShares(ctx context.Context, did syntax.DID) ([]discovercrawl.Share, error)
}

// NetworkShareWire is one followed person's share; SharerDID is the only identity field, resolved live via /api/profiles/{did}. SPEC <authentication>.
type NetworkShareWire struct {
	SharerDID string `json:"sharerDid"`
	Kind      string `json:"kind"`
	ItemURL   string `json:"itemUrl,omitempty"`
	Document  string `json:"document,omitempty"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"createdAt"`
	Title     string `json:"title,omitempty"`
	TargetURL string `json:"targetUrl,omitempty"`
	EntrySlug string `json:"entrySlug,omitempty"`
}

// LibraryNetworkSharesHandler returns followed people's shares, newest first; a follow with no crawl result degrades silently rather than failing the whole response.
func LibraryNetworkSharesHandler(follows LibraryFollowsReader, crawler NetworkShareCrawler, metadata ShareMetadataResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		didStr := sess.Data.AccountDID.String()

		followRows, err := follows.ListUserFollows(r.Context(), didStr)
		if err != nil {
			slog.Warn("/api/library/network-shares: list follows failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		if len(followRows) == 0 {
			writeJSON(w, []NetworkShareWire{})
			return
		}

		// Each crawl runs in its own goroutine (bounded by discoverCrawlFanoutLimit);
		// slots[i] is written only by goroutine i, so no lock is needed.
		slots := make([][]NetworkShareWire, len(followRows))
		g, gctx := errgroup.WithContext(r.Context())
		g.SetLimit(discoverCrawlFanoutLimit)
		for i, f := range followRows {
			i, f := i, f
			g.Go(func() error {
				followedDID, err := syntax.ParseDID(f.SubjectDid)
				if err != nil {
					slog.Warn("/api/library/network-shares: malformed followed did", "did", f.SubjectDid, "err", err)
					return nil
				}
				shares, err := crawler.FetchShares(gctx, followedDID)
				if err != nil {
					// Swallow the error: one broken repo degrades only that person's contribution.
					slog.Warn("/api/library/network-shares: crawl failed", "did", f.SubjectDid, "err", err)
					return nil
				}
				wires := make([]NetworkShareWire, 0, len(shares))
				for _, s := range shares {
					wires = append(wires, NetworkShareWire{
						SharerDID: f.SubjectDid,
						Kind:      s.Kind,
						ItemURL:   s.ItemURL,
						Document:  s.Document,
						Comment:   s.Comment,
						CreatedAt: s.CreatedAt,
					})
				}
				slots[i] = wires
				return nil // never an error, so one failed crawl can't abort the whole group
			})
		}
		_ = g.Wait()

		out := make([]NetworkShareWire, 0, len(followRows))
		for _, wires := range slots {
			out = append(out, wires...)
		}

		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
		enrichNetworkShareMetadata(r.Context(), metadata, out)
		writeJSON(w, out)
	})
}

func enrichNetworkShareMetadata(ctx context.Context, resolver ShareMetadataResolver, shares []NetworkShareWire) {
	targets := make([]sharemeta.Target, len(shares))
	for i, share := range shares {
		targets[i] = sharemeta.Target{ItemURL: share.ItemURL, Document: share.Document}
	}
	resolved := resolveShareMetadata(ctx, resolver, targets)
	for i, metadata := range resolved {
		shares[i].Title = metadata.Title
		shares[i].TargetURL = metadata.TargetURL
		shares[i].EntrySlug = metadata.EntrySlug
	}
}

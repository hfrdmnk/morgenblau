package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"morgenblau/internal/database/db"
	"morgenblau/internal/feedfinder"
)

// --- POST /api/subscriptions/resolve ---

type resolveRequest struct {
	URL string `json:"url"`
}

// candidateWire adds the sibling annotation: set when the user already subscribes to the same site under the other kind.
type candidateWire struct {
	feedfinder.Candidate
	SubscribedVia *subscribedVia `json:"subscribedVia,omitempty"`
}

type subscribedVia struct {
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
}

type resolveResponse struct {
	Candidates            []candidateWire   `json:"candidates"`
	ExistingSubscriptions []existingSubMeta `json:"existingSubscriptions"`
}

// existingSubMeta flags an already-subscribed candidate; FeedURL carries the catalog key (publication at-uri for standardfeed) so the picker can match against publication ?? feedUrl.
type existingSubMeta struct {
	FeedURL string  `json:"feedUrl"`
	Title   *string `json:"title"`
}

// ResolveReader is the read slice the resolve handler needs: the per-candidate dedupe probe plus the sibling-guard join.
type ResolveReader interface {
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
	ListUserSubscriptionsWithSiteURL(ctx context.Context, did string) ([]db.ListUserSubscriptionsWithSiteURLRow, error)
}

// FeedFinder is the slice of *feedfinder.Finder we depend on.
type FeedFinder interface {
	Resolve(ctx context.Context, url string) ([]feedfinder.Candidate, error)
}

// SubscriptionsResolveHandler turns a pasted URL into feed candidates, flags ones the user already subscribes to, and annotates cross-kind siblings.
func SubscriptionsResolveHandler(reader ResolveReader, finder FeedFinder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		var body resolveRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.URL = strings.TrimSpace(body.URL)
		if body.URL == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "url is required")
			return
		}

		cands, err := finder.Resolve(r.Context(), body.URL)
		if err != nil {
			slog.Warn("/api/subscriptions/resolve: finder failed", "url", body.URL, "err", err)
			writeError(w, http.StatusBadGateway, codeUpstreamError, "Couldn't reach that URL")
			return
		}

		// Sibling annotation is best-effort UX sugar; a failed join never fails the resolve.
		siblings := map[string][]subscribedVia{}
		if subs, err := reader.ListUserSubscriptionsWithSiteURL(r.Context(), sess.Data.AccountDID.String()); err != nil {
			slog.Warn("/api/subscriptions/resolve: sibling join failed", "err", err)
		} else {
			for _, s := range subs {
				key := subscriptionSiblingKey(s)
				if key == "" {
					continue
				}
				title := ""
				if s.Title != nil && *s.Title != "" {
					title = *s.Title
				} else if s.CatalogTitle != nil {
					title = *s.CatalogTitle
				}
				siblings[key] = append(siblings[key], subscribedVia{Kind: wireKind(s.Kind), Title: title})
			}
		}

		existing := make([]existingSubMeta, 0)
		out := make([]candidateWire, 0, len(cands))
		for _, c := range cands {
			// Candidate identity is publication at-uri for standardfeed or feed URL for rss, the same key the create path dedupes on.
			probeKey := c.FeedURL
			if c.Publication != "" {
				probeKey = c.Publication
			}
			row, err := reader.GetUserSubscriptionByFeedURL(r.Context(), db.GetUserSubscriptionByFeedURLParams{
				Did:     sess.Data.AccountDID.String(),
				FeedUrl: probeKey,
			})
			switch {
			case err == nil:
				existing = append(existing, existingSubMeta{FeedURL: row.FeedUrl, Title: row.Title})
			case !errors.Is(err, sql.ErrNoRows):
				slog.Warn("/api/subscriptions/resolve: index probe failed", "err", err)
			}

			wire := candidateWire{Candidate: c}
			key, kind := candidateSiblingKey(c)
			if key != "" {
				for _, via := range siblings[key] {
					if via.Kind != kind {
						v := via
						wire.SubscribedVia = &v
						break
					}
				}
			}
			out = append(out, wire)
		}
		writeJSON(w, resolveResponse{Candidates: out, ExistingSubscriptions: existing})
	})
}

func subscriptionSiblingKey(row db.ListUserSubscriptionsWithSiteURLRow) string {
	siteURL := ""
	if row.SiteUrl != nil {
		siteURL = *row.SiteUrl
	}
	if row.Kind == "standardfeed" {
		return siblingKey(siteURL)
	}
	return rssSiblingKey(siteURL, row.FeedUrl)
}

func candidateSiblingKey(c feedfinder.Candidate) (string, string) {
	kind := wireKind(c.Kind)
	if kind == "standardfeed" {
		return siblingKey(c.SiteURL), kind
	}
	return rssSiblingKey(c.SiteURL, c.FeedURL), kind
}

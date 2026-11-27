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

type resolveRequest struct {
	URL string `json:"url"`
}

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

type existingSubMeta struct {
	FeedURL string  `json:"feedUrl"`
	Rkey    string  `json:"rkey"`
	Title   *string `json:"title"`
}

type ResolveReader interface {
	GetUserSubscriptionByFeedURL(ctx context.Context, arg db.GetUserSubscriptionByFeedURLParams) (db.UserSubscription, error)
	ListUserSubscriptionsWithSiteURL(ctx context.Context, did string) ([]db.ListUserSubscriptionsWithSiteURLRow, error)
}

type FeedFinder interface {
	Resolve(ctx context.Context, url string) ([]feedfinder.Candidate, error)
}

func SubscriptionsResolveHandler(reader ResolveReader, finder FeedFinder, aliases FeedAliases) http.Handler {
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
			probeKey := c.FeedURL
			if c.Publication != "" {
				probeKey = c.Publication
			}
			row, err := findSubscription(r.Context(), reader, sess.Data.AccountDID.String(), probeKey, aliases)
			switch {
			case err == nil:
				existing = append(existing, existingSubMeta{FeedURL: probeKey, Rkey: row.Rkey, Title: row.Title})
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

func rejectCrossKindSite(w http.ResponseWriter, r *http.Request, reader IndexReader, did, kind, siteKey, op string) bool {
	if siteKey == "" {
		return false
	}
	subs, err := reader.ListUserSubscriptionsWithSiteURL(r.Context(), did)
	if err != nil {
		slog.Warn(op+": site sibling probe failed", "err", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
		return true
	}
	for _, s := range subs {
		if wireKind(s.Kind) != kind && subscriptionSiblingKey(s) == siteKey {
			writeError(w, http.StatusConflict, codeConflict, "Pick either the RSS feed or the ATProto publication for a site, not both")
			return true
		}
	}
	return false
}

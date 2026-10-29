package api

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/cache/profiles"
	"morgenblau/internal/database/db"
	"morgenblau/internal/middleware/auth"
	"morgenblau/internal/websource"
)

type AdminUsageReader interface {
	CountSubscriptionsByReader(ctx context.Context, arg db.CountSubscriptionsByReaderParams) ([]db.CountSubscriptionsByReaderRow, error)
	ListAccountsLastActive(ctx context.Context) ([]db.ListAccountsLastActiveRow, error)
}

// AdminProfileSource resolves profiles for a caller that may give up; *profiles.Cache satisfies it.
type AdminProfileSource interface {
	Lookup(ctx context.Context, did syntax.DID) (profiles.Profile, error)
}

type NewsletterUsageCounter interface {
	CountActiveSourcesByOwner(ctx context.Context) (map[string]int64, error)
}

type adminUsageTotals struct {
	People     int64 `json:"people"`
	Regular    int64 `json:"regular"`
	Generated  int64 `json:"generated"`
	Newsletter int64 `json:"newsletter"`
}

type adminPerson struct {
	DID          string  `json:"did"`
	Handle       string  `json:"handle"`
	DisplayName  *string `json:"displayName"`
	Avatar       *string `json:"avatar"`
	LastActiveAt *string `json:"lastActiveAt"`
	Regular      int64   `json:"regular"`
	Generated    int64   `json:"generated"`
	Newsletter   int64   `json:"newsletter"`
}

type adminUsageResponse struct {
	Totals adminUsageTotals `json:"totals"`
	People []adminPerson    `json:"people"`
}

// profileLookups bounds concurrent identity resolution so one admin page load cannot fan out to every reader's PDS at once.
const profileLookups = 8

// adminProfileBudget stops new profile lookups after this long; with identity lookups capped at 10s the page answers well inside the server's write timeout, and unresolved accounts show their DID.
const adminProfileBudget = 3 * time.Second

// requireAdmin refuses unless the auth gate admitted this request to an admin path, so a handler mounted anywhere else reads nothing.
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !auth.IsAdmin(r.Context()) {
		NotFound(w, r)
		return false
	}
	return true
}

// AdminUsageHandler lists every account the server knows with counts of its regular, generated and newsletter sources; newsletters may be nil when delivery is off.
func AdminUsageHandler(reader AdminUsageReader, newsletters NewsletterUsageCounter, src AdminProfileSource, webFeeds websource.FeedPrefixes) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
		ctx := r.Context()
		subscriptions, err := reader.CountSubscriptionsByReader(ctx, db.CountSubscriptionsByReaderParams{WebFeedPrefix: webFeeds.Current, LegacyWebFeedPrefix: webFeeds.Legacy})
		if err != nil {
			slog.Warn("/api/admin/usage: count subscriptions failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		activity, err := reader.ListAccountsLastActive(ctx)
		if err != nil {
			slog.Warn("/api/admin/usage: list activity failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		var newsletterCounts map[string]int64
		if newsletters != nil {
			if newsletterCounts, err = newsletters.CountActiveSourcesByOwner(ctx); err != nil {
				slog.Warn("/api/admin/usage: count newsletters failed", "err", err)
				writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
				return
			}
		}

		people := map[string]*adminPerson{}
		person := func(did string) *adminPerson {
			if p, ok := people[did]; ok {
				return p
			}
			p := &adminPerson{DID: did}
			people[did] = p
			return p
		}
		for _, row := range subscriptions {
			p := person(row.Did)
			p.Regular, p.Generated = row.TotalCount-row.GeneratedCount, row.GeneratedCount
		}
		for _, row := range activity {
			person(row.Did).LastActiveAt = &row.LastActiveAt
		}
		for did, n := range newsletterCounts {
			person(did).Newsletter = n
		}

		out := adminUsageResponse{People: make([]adminPerson, 0, len(people))}
		for _, p := range people {
			out.People = append(out.People, *p)
		}
		resolveProfiles(ctx, src, out.People)
		slices.SortFunc(out.People, func(a, b adminPerson) int {
			return cmp.Or(cmp.Compare(derefStr(b.LastActiveAt), derefStr(a.LastActiveAt)), cmp.Compare(a.DID, b.DID))
		})
		for _, p := range out.People {
			out.Totals.People++
			out.Totals.Regular += p.Regular
			out.Totals.Generated += p.Generated
			out.Totals.Newsletter += p.Newsletter
		}
		writeJSON(w, out)
	})
}

// resolveProfiles leaves an account it cannot resolve with an empty handle, so the page falls back to its DID.
func resolveProfiles(ctx context.Context, src AdminProfileSource, people []adminPerson) {
	ctx, cancel := context.WithTimeout(ctx, adminProfileBudget)
	defer cancel()
	sem := make(chan struct{}, profileLookups)
	var wg sync.WaitGroup
	for i := range people {
		did, err := syntax.ParseDID(people[i].DID)
		if err != nil {
			continue
		}
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			profile, err := src.Lookup(ctx, did)
			if err != nil {
				return
			}
			people[i].Handle, people[i].DisplayName, people[i].Avatar = profile.Handle, profile.DisplayName, avatarPath(profile)
		})
	}
	wg.Wait()
}

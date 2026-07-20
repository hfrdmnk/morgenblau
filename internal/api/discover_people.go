package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverhide"
	"morgenblau/internal/discoverrank"
	"morgenblau/internal/feedkey"
)

// maxPersonCandidateDIDs bounds candidate count before per-DID crawls fan out, same posture as maxAdjacentCrawlDIDs.
const maxPersonCandidateDIDs = 200

// ReaderNetworkFollowCrawler builds the one-hop candidate class. SPEC <discovery> People.
type ReaderNetworkFollowCrawler interface {
	FetchReaderNetworkFollows(ctx context.Context, did syntax.DID) ([]discovercrawl.ReaderNetworkFollow, error)
}

// DiscoverTrendingFollowsReader reads the daily batch's follower aggregate table, wired to the reader pool (this is a suggestion read, not a batch write).
type DiscoverTrendingFollowsReader interface {
	ListDiscoverTrendingFollowsAboveBar(ctx context.Context, minDistinctRepos int64) ([]db.DiscoverTrendingFollow, error)
}

// DiscoverTrendingEligibilityReader answers "which bar-passing candidates carry a signal under their own DID", a different filter over the same table as DiscoverTrendingSignalsReader, hence a separate query and interface.
type DiscoverTrendingEligibilityReader interface {
	ListDiscoverTrendingSignalsForEligibleSubjects(ctx context.Context, minDistinctRepos int64) ([]db.DiscoverTrendingSignal, error)
}

// DiscoverPersonReasonWire is the structured basis for a person suggestion; the frontend formats it into English.
type DiscoverPersonReasonWire struct {
	BlueskyFollow     bool   `json:"blueskyFollow,omitempty"`
	TangledFollow     bool   `json:"tangledFollow,omitempty"`
	FollowedByDID     string `json:"followedByDid,omitempty"`
	SharedSourceCount int    `json:"sharedSourceCount,omitempty"`
	// Trending is true when the candidate also clears the network-wide trending bar, personal or trending-only alike. SPEC <discovery>.
	Trending bool `json:"trending,omitempty"`
}

// DiscoverPersonTastePreviewWire carries source titles, or a latest-share fallback when the candidate has none.
type DiscoverPersonTastePreviewWire struct {
	Titles             []string `json:"titles,omitempty"`
	LatestShareItemURL string   `json:"latestShareItemUrl,omitempty"`
	LatestShareComment string   `json:"latestShareComment,omitempty"`
}

// DiscoverPersonWire is one "For you" person card; avatar/handle are deliberately absent, joined by the frontend via /api/profiles/{did} (same as FollowWire).
type DiscoverPersonWire struct {
	DID          string                          `json:"did"`
	Reason       DiscoverPersonReasonWire        `json:"reason"`
	TastePreview *DiscoverPersonTastePreviewWire `json:"tastePreview,omitempty"`
}

// personCandidateBuild accumulates one candidate's discovery-class evidence before any per-DID crawling.
type personCandidateBuild struct {
	bluesky, tangled bool
	followedByDID    string
}

// personCandidateResult is one candidate's crawled outcome; ok is false for a malformed candidate DID, which drops it rather than suggesting with no data.
type personCandidateResult struct {
	ok        bool
	candidate discoverrank.PersonCandidate
	preview   *DiscoverPersonTastePreviewWire
}

// crawlPersonCandidate never returns an error: a failed crawl degrades that signal only, so one candidate never aborts the others.
func crawlPersonCandidate(
	ctx context.Context,
	did string,
	rc *personCandidateBuild,
	viewerKeys map[string]struct{},
	crawler SubscriptionCrawler,
	authored AuthoredPublicationCrawler,
	shares PersonalShareCrawler,
) personCandidateResult {
	personDID, err := syntax.ParseDID(did)
	if err != nil {
		slog.Warn("/api/discover/people: malformed candidate did", "did", did, "err", err)
		return personCandidateResult{}
	}

	var activity []discoverrank.Signal
	var ownKeys []string
	var titles []personTitleEntry

	if found, err := crawler.FetchSubscriptions(ctx, personDID); err != nil {
		slog.Warn("/api/discover/people: subscription crawl failed", "did", did, "err", err)
	} else {
		for _, s := range found {
			at := parseDiscoverTime(s.CreatedAt)
			activity = append(activity, discoverrank.Signal{Kind: discoverrank.SignalSubscribe, At: at})
			ownKeys = append(ownKeys, s.Key)
			titles = append(titles, personTitleEntry{title: firstNonEmptyString(s.Title, s.SiteURL, s.Key), at: at})
		}
	}
	if found, err := authored.FetchAuthoredPublications(ctx, personDID); err != nil {
		slog.Warn("/api/discover/people: authored-publication crawl failed", "did", did, "err", err)
	} else {
		for _, p := range found {
			at := parseDiscoverTime(p.LastPublishedAt)
			activity = append(activity, discoverrank.Signal{Kind: discoverrank.SignalAuthor, At: at})
			ownKeys = append(ownKeys, p.Key)
			titles = append(titles, personTitleEntry{title: firstNonEmptyString(p.Title, p.SiteURL, p.Key), at: at})
		}
	}
	var latestShare *discovercrawl.Share
	if found, err := shares.FetchShares(ctx, personDID); err != nil {
		slog.Warn("/api/discover/people: share crawl failed", "did", did, "err", err)
	} else {
		for i := range found {
			sh := found[i]
			activity = append(activity, discoverrank.Signal{Kind: discoverrank.SignalShare, At: parseDiscoverTime(sh.CreatedAt)})
			if latestShare == nil || sh.CreatedAt > latestShare.CreatedAt {
				latestShare = &found[i]
			}
		}
	}
	shared := 0
	seenKey := map[string]struct{}{}
	for _, k := range ownKeys {
		if _, dup := seenKey[k]; dup {
			continue
		}
		seenKey[k] = struct{}{}
		if _, ok := viewerKeys[k]; ok {
			shared++
		}
	}

	return personCandidateResult{
		ok: true,
		candidate: discoverrank.PersonCandidate{
			DID:               did,
			BlueskyFollow:     rc.bluesky,
			TangledFollow:     rc.tangled,
			FollowedByDID:     rc.followedByDID,
			Activity:          activity,
			SharedSourceCount: shared,
		},
		preview: buildPersonTastePreview(titles, latestShare),
	}
}

// DiscoverPeopleHandler builds the People subtab's unified list: personal "For you" cards first, then trending-only cards appended after. Cold start (zero personal candidates) still renders network-wide trending, same posture as DiscoverSourcesHandler. SPEC <discovery> People.
// Eligibility is hard: a personal candidate with no subscribe/share/author record under their own DID is never suggested, regardless of graph proximity.
func DiscoverPeopleHandler(
	follows DiscoverFollowsReader,
	adjacent AdjacentFollowCrawler,
	readerFollows ReaderNetworkFollowCrawler,
	subs DiscoverSubscriptionsReader,
	crawler SubscriptionCrawler,
	authored AuthoredPublicationCrawler,
	shares PersonalShareCrawler,
	hides DiscoverHiddenReader,
	trendingFollows DiscoverTrendingFollowsReader,
	signals DiscoverTrendingEligibilityReader,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		cursor, ok := discoverCursorFromRequest(w, r, "people")
		if !ok {
			return
		}
		didStr := sess.Data.AccountDID.String()

		followRows, err := follows.ListUserFollows(r.Context(), didStr)
		if err != nil {
			slog.Warn("/api/discover/people: list follows failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		followedSet := make(map[string]struct{}, len(followRows))
		strongDIDs := make([]string, 0, len(followRows))
		for _, f := range followRows {
			followedSet[f.SubjectDid] = struct{}{}
			strongDIDs = append(strongDIDs, f.SubjectDid)
		}
		sort.Strings(strongDIDs)

		raw := map[string]*personCandidateBuild{}
		addCandidate := func(did string) *personCandidateBuild {
			if did == "" || did == didStr {
				return nil
			}
			if _, excluded := followedSet[did]; excluded {
				return nil
			}
			c, ok := raw[did]
			if !ok {
				c = &personCandidateBuild{}
				raw[did] = c
			}
			return c
		}

		// Adjacent-graph crawl failure degrades to zero adjacent candidates rather than failing the page.
		if adjacentFollows, err := adjacent.CrawlAdjacentFollows(r.Context(), sess.Data.AccountDID); err != nil {
			slog.Warn("/api/discover/people: adjacent-graph crawl failed", "err", err)
		} else {
			for _, af := range adjacentFollows {
				c := addCandidate(af.DID)
				if c == nil {
					continue
				}
				if af.Network == "bluesky" {
					c.bluesky = true
				} else if af.Network == "tangled" {
					c.tangled = true
				}
			}
		}

		// One hop inside the reader network; a single friend's crawl failure degrades only that friend's contribution.
		for _, friendDID := range strongDIDs {
			friend, err := syntax.ParseDID(friendDID)
			if err != nil {
				slog.Warn("/api/discover/people: malformed followed did", "did", friendDID, "err", err)
				continue
			}
			found, err := readerFollows.FetchReaderNetworkFollows(r.Context(), friend)
			if err != nil {
				slog.Warn("/api/discover/people: reader-network follow crawl failed", "did", friendDID, "err", err)
				continue
			}
			for _, f := range found {
				c := addCandidate(f.DID)
				if c == nil {
					continue
				}
				// Tie-break when multiple friends follow the same candidate: alphabetically-first friend DID wins.
				if c.followedByDID == "" || friendDID < c.followedByDID {
					c.followedByDID = friendDID
				}
			}
		}

		candidateDIDs := make([]string, 0, len(raw))
		for did := range raw {
			candidateDIDs = append(candidateDIDs, did)
		}
		sort.Strings(candidateDIDs)
		if len(candidateDIDs) > maxPersonCandidateDIDs {
			dropped := len(candidateDIDs) - maxPersonCandidateDIDs
			slog.Warn("/api/discover/people: candidate set truncated",
				"kept", maxPersonCandidateDIDs, "dropped", dropped)
			candidateDIDs = candidateDIDs[:maxPersonCandidateDIDs]
		}

		subRows, err := subs.ListUserSubscriptions(r.Context(), didStr)
		if err != nil {
			slog.Warn("/api/discover/people: list subscriptions failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		viewerKeys := make(map[string]struct{}, len(subRows))
		for _, s := range subRows {
			// Tier-2 stores feed_url verbatim; candidate keys are normalized.
			viewerKeys[feedkey.Normalize(s.FeedUrl)] = struct{}{}
		}

		// results[i] is written only by goroutine i, so no lock is needed.
		// candidateDIDs is sorted, so folding results in index order gives deterministic output regardless of goroutine finish order.
		results := make([]personCandidateResult, len(candidateDIDs))
		g, gctx := errgroup.WithContext(r.Context())
		g.SetLimit(discoverCrawlFanoutLimit)
		for i, did := range candidateDIDs {
			i, did := i, did
			g.Go(func() error {
				results[i] = crawlPersonCandidate(gctx, did, raw[did], viewerKeys, crawler, authored, shares)
				return nil // one candidate's crawl failure never aborts the group
			})
		}
		_ = g.Wait()

		candidates := make([]discoverrank.PersonCandidate, 0, len(candidateDIDs))
		previews := make(map[string]*DiscoverPersonTastePreviewWire, len(candidateDIDs))
		for _, res := range results {
			if !res.ok {
				continue
			}
			candidates = append(candidates, res.candidate)
			previews[res.candidate.DID] = res.preview
		}

		requestNow := time.Now().UTC()
		hiddenDIDs, err := hides.ListActiveDiscoverHides(r.Context(), db.ListActiveDiscoverHidesParams{
			Did:         didStr,
			TargetKind:  string(discoverhide.TargetPerson),
			HiddenUntil: requestNow.Format(time.RFC3339),
		})
		if err != nil {
			slog.Warn("/api/discover/people: list hides failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		excluded := make(map[string]struct{}, len(hiddenDIDs))
		for _, did := range hiddenDIDs {
			excluded[did] = struct{}{}
		}

		rankingNow := time.Unix(0, cursor.RankedAt).UTC()
		personalPage := discoverrank.Page[discoverrank.PersonSuggestion]{Items: []discoverrank.Ranked[discoverrank.PersonSuggestion]{}}
		if !cursor.Personal.Done {
			personalPage = discoverrank.RankPeoplePage(candidates, excluded, discoverPageSize, cursor.Seed, rankingNow, cursor.Personal.After)
		}

		// Trending-eligibility reads degrade to personal-only output rather than failing the page: personal suggestions are the load-bearing half of this endpoint. SPEC <discovery>.
		trendingFollowRows, err := trendingFollows.ListDiscoverTrendingFollowsAboveBar(r.Context(), discoverrank.MinDistinctRepos)
		if err != nil {
			slog.Warn("/api/discover/people: list trending follows failed", "err", err)
			trendingFollowRows = nil
		}
		trendingSignalRows, err := signals.ListDiscoverTrendingSignalsForEligibleSubjects(r.Context(), discoverrank.MinDistinctRepos)
		if err != nil {
			slog.Warn("/api/discover/people: list trending signals failed", "err", err)
			trendingSignalRows = nil
		}

		// The reader reads are already bounded to the bar in SQL, but the distinct-repo count is re-derived here as defense in depth, same posture as RankPeopleTrending's own check.
		reposByDID := make(map[string]map[string]struct{}, len(trendingFollowRows))
		for _, row := range trendingFollowRows {
			repos, ok := reposByDID[row.SubjectDid]
			if !ok {
				repos = map[string]struct{}{}
				reposByDID[row.SubjectDid] = repos
			}
			repos[row.RepoDid] = struct{}{}
		}
		eligibleDIDs := make(map[string]struct{}, len(trendingSignalRows))
		for _, row := range trendingSignalRows {
			eligibleDIDs[row.RepoDid] = struct{}{}
		}
		trendingDIDs := make(map[string]struct{}, len(reposByDID))
		for did, repos := range reposByDID {
			if _, ok := eligibleDIDs[did]; !ok {
				continue
			}
			if len(repos) >= discoverrank.MinDistinctRepos {
				trendingDIDs[did] = struct{}{}
			}
		}

		// Any personal candidate keeps its stronger personal reason throughout pagination.
		excludedForTrendingOnly := make(map[string]struct{}, len(excluded)+len(followedSet)+len(candidates)+1)
		for did := range excluded {
			excludedForTrendingOnly[did] = struct{}{}
		}
		for did := range followedSet {
			excludedForTrendingOnly[did] = struct{}{}
		}
		excludedForTrendingOnly[didStr] = struct{}{}
		for _, c := range candidates {
			excludedForTrendingOnly[c.DID] = struct{}{}
		}

		trendingCandidates := groupTrendingFollows(trendingFollowRows, trendingSignalRows)
		trendingPreviews := trendingPersonTastePreviews(trendingSignalRows)
		trendingPage := discoverrank.Page[discoverrank.PersonSuggestion]{Items: []discoverrank.Ranked[discoverrank.PersonSuggestion]{}}
		if !cursor.Trending.Done {
			trendingPage = discoverrank.RankPeopleTrendingPage(trendingCandidates, excludedForTrendingOnly, discoverPageSize, cursor.Seed, rankingNow, cursor.Trending.After)
		}
		page := balanceDiscoverPages(cursor, personalPage, trendingPage)
		out := make([]DiscoverPersonWire, 0, len(page.Personal)+len(page.Trending))
		for _, ranked := range page.Personal {
			s := ranked.Value
			wire := DiscoverPersonWire{
				DID: s.DID,
				Reason: DiscoverPersonReasonWire{
					BlueskyFollow:     s.Reason.BlueskyFollow,
					TangledFollow:     s.Reason.TangledFollow,
					FollowedByDID:     s.Reason.FollowedByDID,
					SharedSourceCount: s.Reason.SharedSourceCount,
				},
				TastePreview: previews[s.DID],
			}
			if _, ok := trendingDIDs[s.DID]; ok {
				wire.Reason.Trending = true
			}
			out = append(out, wire)
		}
		for _, ranked := range page.Trending {
			s := ranked.Value
			out = append(out, DiscoverPersonWire{
				DID:          s.DID,
				Reason:       DiscoverPersonReasonWire{Trending: true},
				TastePreview: trendingPreviews[s.DID],
			})
		}

		nextCursor, err := discoverNextCursor(page.Cursor, page.HasMore)
		if err != nil {
			slog.Error("/api/discover/people: encode cursor failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		writeJSON(w, discoverPageWire[DiscoverPersonWire]{Items: out, NextCursor: nextCursor})
	})
}

// groupTrendingFollows folds follower rows into one TrendingPersonCandidate per subject DID, and folds signal rows into that DID's eligibility flag and decayed share-activity. SPEC <discovery> People "Eligibility".
func groupTrendingFollows(followRows []db.DiscoverTrendingFollow, signalRows []db.DiscoverTrendingSignal) []discoverrank.TrendingPersonCandidate {
	followersByDID := map[string][]string{}
	order := make([]string, 0, len(followRows))
	for _, f := range followRows {
		if _, seen := followersByDID[f.SubjectDid]; !seen {
			order = append(order, f.SubjectDid)
		}
		followersByDID[f.SubjectDid] = append(followersByDID[f.SubjectDid], f.RepoDid)
	}

	eligible := map[string]struct{}{}
	shares := map[string][]discoverrank.Signal{}
	for _, row := range signalRows {
		eligible[row.RepoDid] = struct{}{}
		if row.SignalKind == "share" {
			shares[row.RepoDid] = append(shares[row.RepoDid], discoverrank.Signal{
				Kind: discoverrank.SignalShare,
				At:   parseDiscoverTime(derefOptString(row.SignalAt)),
			})
		}
	}

	out := make([]discoverrank.TrendingPersonCandidate, 0, len(order))
	for _, did := range order {
		_, ok := eligible[did]
		out = append(out, discoverrank.TrendingPersonCandidate{
			DID:           did,
			FollowerDIDs:  followersByDID[did],
			ShareActivity: shares[did],
			Eligible:      ok,
		})
	}
	return out
}

// trendingPersonTastePreviews builds each eligible trending DID's taste preview directly from the eligibility read already in hand, no crawl: subscribe/author titles only (never share, never save; save is already excluded by the query itself), newest first, capped at maxTastePreviewTitles, omitted when none.
func trendingPersonTastePreviews(rows []db.DiscoverTrendingSignal) map[string]*DiscoverPersonTastePreviewWire {
	titlesByDID := map[string][]personTitleEntry{}
	for _, row := range rows {
		if row.SignalKind != "subscribe" && row.SignalKind != "author" {
			continue
		}
		title := derefOptString(row.Title)
		if title == "" {
			continue
		}
		titlesByDID[row.RepoDid] = append(titlesByDID[row.RepoDid], personTitleEntry{
			title: title,
			at:    parseDiscoverTime(derefOptString(row.SignalAt)),
		})
	}
	out := make(map[string]*DiscoverPersonTastePreviewWire, len(titlesByDID))
	for did, titles := range titlesByDID {
		if preview := buildPersonTastePreview(titles, nil); preview != nil {
			out[did] = preview
		}
	}
	return out
}

// personTitleEntry pairs a taste-preview title with the recency to sort by (newest first).
type personTitleEntry struct {
	title string
	at    time.Time
}

// maxTastePreviewTitles SPEC <discovery> Cards.
const maxTastePreviewTitles = 3

// buildPersonTastePreview prefers the candidate's own source titles, newest first, falling back to their latest share when none exist.
func buildPersonTastePreview(titles []personTitleEntry, latestShare *discovercrawl.Share) *DiscoverPersonTastePreviewWire {
	if len(titles) > 0 {
		sort.Slice(titles, func(i, j int) bool { return titles[i].at.After(titles[j].at) })
		out := make([]string, 0, maxTastePreviewTitles)
		seen := map[string]struct{}{}
		for _, t := range titles {
			if t.title == "" {
				continue
			}
			if _, dup := seen[t.title]; dup {
				continue
			}
			seen[t.title] = struct{}{}
			out = append(out, t.title)
			if len(out) == maxTastePreviewTitles {
				break
			}
		}
		if len(out) > 0 {
			return &DiscoverPersonTastePreviewWire{Titles: out}
		}
	}
	if latestShare != nil {
		return &DiscoverPersonTastePreviewWire{
			LatestShareItemURL: latestShare.ItemURL,
			LatestShareComment: latestShare.Comment,
		}
	}
	return nil
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

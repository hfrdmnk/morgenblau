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

// ReaderNetworkFollowCrawler builds the one-hop candidate class from the viewer's whole follow list in one call. SPEC <discovery> People.
type ReaderNetworkFollowCrawler interface {
	FetchReaderNetworkFollowsBatch(ctx context.Context, dids []string) map[string][]discovercrawl.ReaderNetworkFollow
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

// personActivityEntry pairs a raw activity signal with the canonical source key it came from; an empty key (unresolved share) is never deduped against anything else.
type personActivityEntry struct {
	key    string
	signal discoverrank.Signal
}

// dedupPersonActivity keeps one signal per canonical source key, strongest wins, so a source visible via more than one record (e.g. subscribed and authored) never counts twice. SPEC <discovery> Personal ranking: one signal per person per source.
func dedupPersonActivity(entries []personActivityEntry) []discoverrank.Signal {
	byKey := map[string]discoverrank.Signal{}
	var unkeyed []discoverrank.Signal
	for _, e := range entries {
		if e.key == "" {
			unkeyed = append(unkeyed, e.signal)
			continue
		}
		if cur, ok := byKey[e.key]; !ok || discoverrank.StrongerSignal(e.signal, cur) {
			byKey[e.key] = e.signal
		}
	}
	out := make([]discoverrank.Signal, 0, len(byKey)+len(unkeyed))
	for _, s := range byKey {
		out = append(out, s)
	}
	return append(out, unkeyed...)
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

	var activityEntries []personActivityEntry
	var ownKeys []string
	var titles []personTitleEntry

	if found, err := crawler.FetchSubscriptions(ctx, personDID); err != nil {
		slog.Warn("/api/discover/people: subscription crawl failed", "did", did, "err", err)
	} else {
		for _, s := range found {
			at := parseDiscoverTime(s.CreatedAt)
			activityEntries = append(activityEntries, personActivityEntry{key: s.Key, signal: discoverrank.Signal{Kind: discoverrank.SignalSubscribe, At: at}})
			ownKeys = append(ownKeys, s.Key)
			titles = append(titles, personTitleEntry{title: firstNonEmptyString(s.Title, s.SiteURL, s.Key), at: at})
		}
	}
	if found, err := authored.FetchAuthoredPublications(ctx, personDID); err != nil {
		slog.Warn("/api/discover/people: authored-publication crawl failed", "did", did, "err", err)
	} else {
		for _, p := range found {
			at := parseDiscoverTime(p.LastPublishedAt)
			activityEntries = append(activityEntries, personActivityEntry{key: p.Key, signal: discoverrank.Signal{Kind: discoverrank.SignalAuthor, At: at}})
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
			activityEntries = append(activityEntries, personActivityEntry{key: sh.FeedURL, signal: discoverrank.Signal{Kind: discoverrank.SignalShare, At: parseDiscoverTime(sh.CreatedAt)}})
			if latestShare == nil || sh.CreatedAt > latestShare.CreatedAt {
				latestShare = &found[i]
			}
		}
	}
	activity := dedupPersonActivity(activityEntries)
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

// DiscoverPeoplePayload is one user's assembled People input: every read and crawl the endpoint needs, held together so ranking can run per page without touching the network again. Ranking is deliberately not part of it, since it is a pure function of this plus the request cursor.
type DiscoverPeoplePayload struct {
	personal                []discoverrank.PersonCandidate
	previews                map[string]*DiscoverPersonTastePreviewWire
	excluded                map[string]struct{}
	excludedForTrendingOnly map[string]struct{}
	trendingDIDs            map[string]struct{}
	trending                []discoverrank.TrendingPersonCandidate
	trendingPreviews        map[string]*DiscoverPersonTastePreviewWire
}

// discoverPeopleBuilder groups the readers and crawlers the assembly needs, so buildDiscoverPeople takes a request's worth of arguments rather than ten dependencies.
type discoverPeopleBuilder struct {
	follows         DiscoverFollowsReader
	adjacent        AdjacentFollowCrawler
	readerFollows   ReaderNetworkFollowCrawler
	subs            DiscoverSubscriptionsReader
	crawler         SubscriptionCrawler
	authored        AuthoredPublicationCrawler
	shares          PersonalShareCrawler
	hides           DiscoverHiddenReader
	trendingFollows DiscoverTrendingFollowsReader
	signals         DiscoverTrendingEligibilityReader
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
	memo DiscoverMemo[DiscoverPeoplePayload],
) http.Handler {
	builder := discoverPeopleBuilder{
		follows:         follows,
		adjacent:        adjacent,
		readerFollows:   readerFollows,
		subs:            subs,
		crawler:         crawler,
		authored:        authored,
		shares:          shares,
		hides:           hides,
		trendingFollows: trendingFollows,
		signals:         signals,
	}
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

		payload, err := memoizedPayload(memo, didStr, func() (DiscoverPeoplePayload, error) {
			return builder.build(r.Context(), sess.Data.AccountDID)
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		out, nextState, hasMore := renderDiscoverPeople(payload, cursor)
		nextCursor, err := discoverNextCursor(nextState, hasMore)
		if err != nil {
			slog.Error("/api/discover/people: encode cursor failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		writeJSON(w, discoverPageWire[DiscoverPersonWire]{Items: out, NextCursor: nextCursor})
	})
}

// build assembles the whole candidate pool. Only the three reads the page cannot degrade without (follows, subscriptions, hides) return an error; every other failure degrades its own signal and leaves the rest of the page standing.
func (b discoverPeopleBuilder) build(ctx context.Context, did syntax.DID) (DiscoverPeoplePayload, error) {
	didStr := did.String()

	followRows, err := b.follows.ListUserFollows(ctx, didStr)
	if err != nil {
		slog.Warn("/api/discover/people: list follows failed", "err", err)
		return DiscoverPeoplePayload{}, err
	}
	followedSet := make(map[string]struct{}, len(followRows))
	strongDIDs := make([]string, 0, len(followRows))
	for _, f := range followRows {
		followedSet[f.SubjectDid] = struct{}{}
		strongDIDs = append(strongDIDs, f.SubjectDid)
	}
	sort.Strings(strongDIDs)

	raw := map[string]*personCandidateBuild{}
	addCandidate := func(candidateDID string) *personCandidateBuild {
		if candidateDID == "" || candidateDID == didStr {
			return nil
		}
		if _, excluded := followedSet[candidateDID]; excluded {
			return nil
		}
		c, ok := raw[candidateDID]
		if !ok {
			c = &personCandidateBuild{}
			raw[candidateDID] = c
		}
		return c
	}

	// Adjacent-graph crawl failure degrades to zero adjacent candidates rather than failing the page.
	if adjacentFollows, err := b.adjacent.CrawlAdjacentFollows(ctx, did); err != nil {
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

	// One hop inside the reader network, read in one batch; a single friend's crawl failure degrades only that friend's contribution. strongDIDs is sorted, so the tie-break below is stable.
	followsByFriend := b.readerFollows.FetchReaderNetworkFollowsBatch(ctx, strongDIDs)
	for _, friendDID := range strongDIDs {
		for _, f := range followsByFriend[friendDID] {
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
	for candidateDID := range raw {
		candidateDIDs = append(candidateDIDs, candidateDID)
	}
	sort.Strings(candidateDIDs)
	if len(candidateDIDs) > maxPersonCandidateDIDs {
		dropped := len(candidateDIDs) - maxPersonCandidateDIDs
		slog.Warn("/api/discover/people: candidate set truncated",
			"kept", maxPersonCandidateDIDs, "dropped", dropped)
		candidateDIDs = candidateDIDs[:maxPersonCandidateDIDs]
	}

	subRows, err := b.subs.ListUserSubscriptions(ctx, didStr)
	if err != nil {
		slog.Warn("/api/discover/people: list subscriptions failed", "err", err)
		return DiscoverPeoplePayload{}, err
	}
	viewerKeys := make(map[string]struct{}, len(subRows))
	for _, s := range subRows {
		// Tier-2 stores feed_url verbatim; candidate keys are normalized.
		viewerKeys[feedkey.Normalize(s.FeedUrl)] = struct{}{}
	}

	// results[i] is written only by goroutine i, so no lock is needed.
	// candidateDIDs is sorted, so folding results in index order gives deterministic output regardless of goroutine finish order.
	results := make([]personCandidateResult, len(candidateDIDs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(discoverCrawlFanoutLimit)
	for i, candidateDID := range candidateDIDs {
		i, candidateDID := i, candidateDID
		g.Go(func() error {
			results[i] = crawlPersonCandidate(gctx, candidateDID, raw[candidateDID], viewerKeys, b.crawler, b.authored, b.shares)
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
	hiddenDIDs, err := b.hides.ListActiveDiscoverHides(ctx, db.ListActiveDiscoverHidesParams{
		Did:         didStr,
		TargetKind:  string(discoverhide.TargetPerson),
		HiddenUntil: requestNow.Format(time.RFC3339),
	})
	if err != nil {
		slog.Warn("/api/discover/people: list hides failed", "err", err)
		return DiscoverPeoplePayload{}, err
	}
	excluded := make(map[string]struct{}, len(hiddenDIDs))
	for _, hiddenDID := range hiddenDIDs {
		excluded[hiddenDID] = struct{}{}
	}

	// Trending-eligibility reads degrade to personal-only output rather than failing the page: personal suggestions are the load-bearing half of this endpoint. SPEC <discovery>.
	trendingFollowRows, err := b.trendingFollows.ListDiscoverTrendingFollowsAboveBar(ctx, discoverrank.MinDistinctRepos)
	if err != nil {
		slog.Warn("/api/discover/people: list trending follows failed", "err", err)
		trendingFollowRows = nil
	}
	trendingSignalRows, err := b.signals.ListDiscoverTrendingSignalsForEligibleSubjects(ctx, discoverrank.MinDistinctRepos)
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
	for trendingDID, repos := range reposByDID {
		if _, ok := eligibleDIDs[trendingDID]; !ok {
			continue
		}
		if len(repos) >= discoverrank.MinDistinctRepos {
			trendingDIDs[trendingDID] = struct{}{}
		}
	}

	// Any personal candidate keeps its stronger personal reason throughout pagination.
	excludedForTrendingOnly := make(map[string]struct{}, len(excluded)+len(followedSet)+len(candidates)+1)
	for excludedDID := range excluded {
		excludedForTrendingOnly[excludedDID] = struct{}{}
	}
	for followedDID := range followedSet {
		excludedForTrendingOnly[followedDID] = struct{}{}
	}
	excludedForTrendingOnly[didStr] = struct{}{}
	for _, c := range candidates {
		excludedForTrendingOnly[c.DID] = struct{}{}
	}

	return DiscoverPeoplePayload{
		personal:                candidates,
		previews:                previews,
		excluded:                excluded,
		excludedForTrendingOnly: excludedForTrendingOnly,
		trendingDIDs:            trendingDIDs,
		trending:                groupTrendingFollows(trendingFollowRows, trendingSignalRows),
		trendingPreviews:        trendingPersonTastePreviews(trendingSignalRows),
	}, nil
}

// renderDiscoverPeople ranks and slices one page out of an assembled payload. Pure: the same payload and cursor always produce the same page, which is what makes serving a cursor page off the memo safe.
func renderDiscoverPeople(payload DiscoverPeoplePayload, cursor discoverCursor) ([]DiscoverPersonWire, discoverCursor, bool) {
	rankingNow := time.Unix(0, cursor.RankedAt).UTC()
	personalPage := discoverrank.Page[discoverrank.PersonSuggestion]{Items: []discoverrank.Ranked[discoverrank.PersonSuggestion]{}}
	if !cursor.Personal.Done {
		personalPage = discoverrank.RankPeoplePage(payload.personal, payload.excluded, discoverPageSize, cursor.Seed, rankingNow, cursor.Personal.After)
	}
	trendingPage := discoverrank.Page[discoverrank.PersonSuggestion]{Items: []discoverrank.Ranked[discoverrank.PersonSuggestion]{}}
	if !cursor.Trending.Done {
		trendingPage = discoverrank.RankPeopleTrendingPage(payload.trending, payload.excludedForTrendingOnly, discoverPageSize, cursor.Seed, rankingNow, cursor.Trending.After)
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
			TastePreview: payload.previews[s.DID],
		}
		if _, ok := payload.trendingDIDs[s.DID]; ok {
			wire.Reason.Trending = true
		}
		out = append(out, wire)
	}
	for _, ranked := range page.Trending {
		s := ranked.Value
		out = append(out, DiscoverPersonWire{
			DID:          s.DID,
			Reason:       DiscoverPersonReasonWire{Trending: true},
			TastePreview: payload.trendingPreviews[s.DID],
		})
	}
	return out, page.Cursor, page.HasMore
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

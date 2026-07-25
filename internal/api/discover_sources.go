package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"

	"morgenblau/internal/database/db"
	"morgenblau/internal/discovercrawl"
	"morgenblau/internal/discoverhide"
	"morgenblau/internal/discoverlang"
	"morgenblau/internal/discoverrank"
	"morgenblau/internal/feedkey"
)

// discoverCrawlFanoutLimit bounds concurrent per-candidate PDS crawls (shared by DiscoverPeopleHandler and LibraryNetworkSharesHandler) so a cold cache with hundreds of candidates can't flood SQLite's single writer connection or blow the server's 30s WriteTimeout. The batched crawlers enforce the same ceiling themselves via discovercrawl.BatchCrawlFanout.
const discoverCrawlFanoutLimit = 8

// DiscoverFollowsReader reads the user's follow graph.
type DiscoverFollowsReader interface {
	ListUserFollows(ctx context.Context, did string) ([]db.UserFollow, error)
}

// DiscoverSubscriptionsReader excludes already-subscribed keys from suggestions.
type DiscoverSubscriptionsReader interface {
	ListUserSubscriptions(ctx context.Context, did string) ([]db.UserSubscription, error)
}

// SubscriptionCrawler fetches feed subscriptions: one whole fan-out at a time for Sources, one candidate at a time for People's per-candidate crawl.
type SubscriptionCrawler interface {
	FetchSubscriptions(ctx context.Context, did syntax.DID) ([]discovercrawl.Subscription, error)
	FetchSubscriptionsBatch(ctx context.Context, dids []string) map[string][]discovercrawl.Subscription
}

// AuthoredPublicationCrawler fetches publications a person authors. SPEC <discovery> Signal ordering.
type AuthoredPublicationCrawler interface {
	FetchAuthoredPublications(ctx context.Context, did syntax.DID) ([]discovercrawl.AuthoredPublication, error)
	FetchAuthoredPublicationsBatch(ctx context.Context, dids []string) map[string][]discovercrawl.AuthoredPublication
}

// PersonalShareCrawler fetches a followed person's shares.
type PersonalShareCrawler interface {
	FetchShares(ctx context.Context, did syntax.DID) ([]discovercrawl.Share, error)
	FetchSharesBatch(ctx context.Context, dids []string) map[string][]discovercrawl.Share
}

// DiscoverEntryResolver resolves a share/save to its source key via Tier-2 provenance; both queries hit the read-only entries table the sync pipeline maintains. SPEC <discovery>.
type DiscoverEntryResolver interface {
	GetFeedURLByGuid(ctx context.Context, guid string) (string, error)
	GetFeedURLByItemURL(ctx context.Context, url string) (string, error)
}

// AdjacentFollowCrawler builds the weak trust tier from Bluesky/Tangled follows. Lists the session user's own repo, not a followed person's; see internal/discovercrawl/adjacent_store.go for the SelfCrawlTTL cache wrapping this.
type AdjacentFollowCrawler interface {
	CrawlAdjacentFollows(ctx context.Context, did syntax.DID) ([]discovercrawl.AdjacentFollow, error)
}

// OwnForeignSubscriptionCrawler builds the self trust tier from the user's own foreign records, same posture as AdjacentFollowCrawler: lists the session user's own repo, cached at SelfCrawlTTL (internal/discovercrawl/self_foreign_store.go). SPEC <discovery>.
type OwnForeignSubscriptionCrawler interface {
	CrawlOwnForeignSubscriptions(ctx context.Context, did syntax.DID) ([]discovercrawl.ForeignSubscription, error)
}

// DiscoverHiddenReader excludes hidden targets before ranking; expired hides resurface naturally since the query only returns rows where hidden_until is still future. SPEC <discovery>.
type DiscoverHiddenReader interface {
	ListActiveDiscoverHides(ctx context.Context, arg db.ListActiveDiscoverHidesParams) ([]string, error)
}

// DiscoverTrendingSignalsReader reads the daily batch's aggregate table, wired to the reader pool (this is a suggestion read, not a batch write).
type DiscoverTrendingSignalsReader interface {
	ListDiscoverTrendingSignalsAboveBar(ctx context.Context, minDistinctRepos int64) ([]db.DiscoverTrendingSignal, error)
}

// DiscoverFeedLanguageReader reads Tier-2's detected source languages; see ListFeedLanguages in queries/subscriptions.sql for why a whole-table read is cheap here.
type DiscoverFeedLanguageReader interface {
	ListFeedLanguages(ctx context.Context) ([]db.ListFeedLanguagesRow, error)
}

// DiscoverSourceTitleBackfillReader looks up the network-wide trending table for a page's source keys, used to backfill reaction-only rss candidates that addSignal never gave a title. SPEC <discovery>.
type DiscoverSourceTitleBackfillReader interface {
	ListDiscoverTrendingSignalTitles(ctx context.Context, sourceKeys []string) ([]db.ListDiscoverTrendingSignalTitlesRow, error)
}

// DiscoverReasonWire is the structured basis for a suggestion; the frontend formats it into English. SPEC <discovery>.
type DiscoverReasonWire struct {
	StrongCount        int      `json:"strongCount"`
	WeakCount          int      `json:"weakCount"`
	TopFollowerDID     string   `json:"topFollowerDid,omitempty"`
	TopFollowerNetwork string   `json:"topFollowerNetwork,omitempty"` // "bluesky" | "tangled"
	TopSignal          string   `json:"topSignal,omitempty"`          // "author" | "subscribe" | "share" | "save"
	SelfSourceApp      string   `json:"selfSourceApp,omitempty"`      // "skyreader" | "glean"
	FollowerDIDs       []string `json:"followerDids,omitempty"`
	AuthorDID          string   `json:"authorDid,omitempty"` // set only when TopSignal == "author"
	Trending           bool     `json:"trending,omitempty"`  // true when the source also clears the network-wide trending bar
}

// DiscoverSourceWire is one "For you" source suggestion card.
type DiscoverSourceWire struct {
	Key         string             `json:"key"`
	Kind        string             `json:"kind"`
	Title       string             `json:"title,omitempty"`
	SiteURL     string             `json:"siteUrl,omitempty"`
	FeedURL     string             `json:"feedUrl,omitempty"`
	Publication string             `json:"publication,omitempty"`
	Reason      DiscoverReasonWire `json:"reason"`
}

// DiscoverSourcesPayload is one user's assembled Sources input: every read and crawl the endpoint needs, held together so ranking can run per page without touching the network again. Ranking is deliberately not part of it, since it is a pure function of this plus the request cursor.
type DiscoverSourcesPayload struct {
	personal                []discoverrank.Candidate
	excluded                map[string]struct{}
	excludedForTrendingOnly map[string]struct{}
	trendingKeys            map[string]struct{}
	trending                []discoverrank.Candidate
}

// discoverSourcesBuilder groups the readers and crawlers the assembly needs, so buildDiscoverSources takes a request's worth of arguments rather than a dozen dependencies.
type discoverSourcesBuilder struct {
	follows       DiscoverFollowsReader
	adjacent      AdjacentFollowCrawler
	ownForeign    OwnForeignSubscriptionCrawler
	subs          DiscoverSubscriptionsReader
	crawler       SubscriptionCrawler
	authored      AuthoredPublicationCrawler
	shares        PersonalShareCrawler
	entries       DiscoverEntryResolver
	hides         DiscoverHiddenReader
	trending      DiscoverTrendingSignalsReader
	languages     DiscoverFeedLanguageReader
	titleBackfill DiscoverSourceTitleBackfillReader
}

// DiscoverSourcesHandler serves the unified Sources list: personal cards (cold start with no strong or adjacent follows crawls nothing but still renders network-wide trending), then trending-only cards appended after. One followed repo's crawl failure degrades only that signal. SPEC <discovery>.
func DiscoverSourcesHandler(
	follows DiscoverFollowsReader,
	adjacent AdjacentFollowCrawler,
	ownForeign OwnForeignSubscriptionCrawler,
	subs DiscoverSubscriptionsReader,
	crawler SubscriptionCrawler,
	authored AuthoredPublicationCrawler,
	shares PersonalShareCrawler,
	entries DiscoverEntryResolver,
	hides DiscoverHiddenReader,
	trending DiscoverTrendingSignalsReader,
	languages DiscoverFeedLanguageReader,
	titleBackfill DiscoverSourceTitleBackfillReader,
	memo DiscoverMemo[DiscoverSourcesPayload],
) http.Handler {
	builder := discoverSourcesBuilder{
		follows:       follows,
		adjacent:      adjacent,
		ownForeign:    ownForeign,
		subs:          subs,
		crawler:       crawler,
		authored:      authored,
		shares:        shares,
		entries:       entries,
		hides:         hides,
		trending:      trending,
		languages:     languages,
		titleBackfill: titleBackfill,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := requireSession(w, r)
		if !ok {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		cursor, ok := discoverCursorFromRequest(w, r, "sources")
		if !ok {
			return
		}
		didStr := sess.Data.AccountDID.String()

		acceptLanguage := r.Header.Get("Accept-Language")
		payload, err := memoizedPayload(memo, didStr, func() (DiscoverSourcesPayload, error) {
			return builder.build(r.Context(), sess.Data.AccountDID, acceptLanguage)
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}

		out, nextState, hasMore := renderDiscoverSources(payload, cursor)
		nextCursor, err := discoverNextCursor(nextState, hasMore)
		if err != nil {
			slog.Error("/api/discover/sources: encode cursor failed", "err", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, "internal error")
			return
		}
		writeJSON(w, discoverPageWire[DiscoverSourceWire]{Items: out, NextCursor: nextCursor})
	})
}

// build assembles the whole candidate pool. Only the three reads the page cannot degrade without (follows, subscriptions, hides) return an error; every other failure degrades its own signal and leaves the rest of the page standing.
func (b discoverSourcesBuilder) build(ctx context.Context, did syntax.DID, acceptLanguage string) (DiscoverSourcesPayload, error) {
	didStr := did.String()

	followRows, err := b.follows.ListUserFollows(ctx, didStr)
	if err != nil {
		slog.Warn("/api/discover/sources: list follows failed", "err", err)
		return DiscoverSourcesPayload{}, err
	}
	strongDIDs := make([]string, 0, len(followRows))
	for _, f := range followRows {
		strongDIDs = append(strongDIDs, f.SubjectDid)
	}

	// The adjacent graph is an enrichment signal, not a hard dependency: a failure degrades to zero weak candidates rather than failing the page. SPEC <discovery>.
	var weakFollows []discoverrank.WeakFollow
	if adjacentFollows, err := b.adjacent.CrawlAdjacentFollows(ctx, did); err != nil {
		slog.Warn("/api/discover/sources: adjacent-graph crawl failed", "err", err)
	} else {
		for _, af := range adjacentFollows {
			weakFollows = append(weakFollows, discoverrank.WeakFollow{DID: af.DID, Network: discoverrank.Network(af.Network)})
		}
	}

	// A person in both tiers counts once, at the higher (strong) tier. SPEC <discovery>.
	tiers := discoverrank.ResolveFollowerTiers(strongDIDs, weakFollows)

	// subjectDIDs is sorted (not map order) so folding the batch results below is deterministic.
	subjectDIDs := make([]string, 0, len(tiers))
	for subjectDID := range tiers {
		subjectDIDs = append(subjectDIDs, subjectDID)
	}
	sort.Strings(subjectDIDs)

	// One batch per signal instead of three crawls per candidate; each crawler serves what it has cached and re-crawls only the stale DIDs, bounded internally.
	subsByDID := b.crawler.FetchSubscriptionsBatch(ctx, subjectDIDs)
	authoredByDID := b.authored.FetchAuthoredPublicationsBatch(ctx, subjectDIDs)
	sharesByDID := b.shares.FetchSharesBatch(ctx, subjectDIDs)

	candidates := map[string]*discoverrank.Candidate{}
	for _, subjectDID := range subjectDIDs {
		follower := tiers[subjectDID]
		for _, s := range subsByDID[subjectDID] {
			addSignal(candidates, s.Key, s.Kind, s.Title, s.SiteURL, follower, discoverrank.SignalSubscribe, parseDiscoverTime(s.CreatedAt))
		}
		for _, p := range authoredByDID[subjectDID] {
			addSignal(candidates, p.Key, p.Kind, p.Title, p.SiteURL, follower, discoverrank.SignalAuthor, parseDiscoverTime(p.LastPublishedAt))
		}
		for _, sh := range sharesByDID[subjectDID] {
			key, ok := resolveReactionKey(ctx, b.entries, sh.FeedURL, sh.Document, sh.ItemURL)
			if !ok {
				continue // unresolvable reaction: no candidate, no error. SPEC <discovery>.
			}
			addSignal(candidates, key, kindForReactionKey(key), "", "", follower, discoverrank.SignalShare, parseDiscoverTime(sh.CreatedAt))
		}
	}

	// The user's own Skyreader/Glean subscriptions (self trust tier) always run, independent of the follow graph, unlike the fan-out above. SPEC <discovery>.
	if foreign, err := b.ownForeign.CrawlOwnForeignSubscriptions(ctx, did); err != nil {
		slog.Warn("/api/discover/sources: own-foreign crawl failed", "err", err)
	} else {
		selfFollower := discoverrank.Follower{DID: didStr, Tier: discoverrank.TierSelf}
		for _, s := range foreign {
			selfFollower.SelfApp = discoverrank.SelfSourceApp(s.App)
			addSignal(candidates, s.Key, s.Kind, s.Title, s.SiteURL, selfFollower, discoverrank.SignalSubscribe, parseDiscoverTime(s.CreatedAt))
		}
	}

	backfillMissingTitles(ctx, b.titleBackfill, candidates)

	subRows, err := b.subs.ListUserSubscriptions(ctx, didStr)
	if err != nil {
		slog.Warn("/api/discover/sources: list subscriptions failed", "err", err)
		return DiscoverSourcesPayload{}, err
	}
	requestNow := time.Now().UTC()
	hiddenKeys, err := b.hides.ListActiveDiscoverHides(ctx, db.ListActiveDiscoverHidesParams{
		Did:         didStr,
		TargetKind:  string(discoverhide.TargetSource),
		HiddenUntil: requestNow.Format(time.RFC3339),
	})
	if err != nil {
		slog.Warn("/api/discover/sources: list hides failed", "err", err)
		return DiscoverSourcesPayload{}, err
	}

	excluded := make(map[string]struct{}, len(subRows)+len(hiddenKeys))
	for _, s := range subRows {
		// Tier-2 stores feed_url verbatim; candidate keys are normalized.
		excluded[feedkey.Normalize(s.FeedUrl)] = struct{}{}
	}
	for _, key := range hiddenKeys {
		excluded[key] = struct{}{}
	}

	list := make([]discoverrank.Candidate, 0, len(candidates))
	for _, c := range candidates {
		list = append(list, *c)
	}

	// A trending-signals read failure degrades to no trending flag/cards rather than failing the page: personal suggestions are the load-bearing half of this endpoint. SPEC <discovery>.
	trendingRows, err := b.trending.ListDiscoverTrendingSignalsAboveBar(ctx, discoverrank.MinDistinctRepos)
	if err != nil {
		slog.Warn("/api/discover/sources: list trending signals failed", "err", err)
		trendingRows = nil
	}
	// The reader read is already bounded to the bar in SQL, but the distinct-repo count is re-derived here as defense in depth, same posture as RankTrending's own check.
	reposBySource := make(map[string]map[string]struct{}, len(trendingRows))
	for _, row := range trendingRows {
		repos, ok := reposBySource[row.SourceKey]
		if !ok {
			repos = map[string]struct{}{}
			reposBySource[row.SourceKey] = repos
		}
		repos[row.RepoDid] = struct{}{}
	}
	trendingKeys := make(map[string]struct{}, len(reposBySource))
	for key, repos := range reposBySource {
		if len(repos) >= discoverrank.MinDistinctRepos {
			trendingKeys[key] = struct{}{}
		}
	}

	// Any personal candidate keeps its stronger personal reason throughout pagination, even when it has not appeared yet.
	excludedForTrendingOnly := make(map[string]struct{}, len(excluded)+len(candidates))
	for key := range excluded {
		excludedForTrendingOnly[key] = struct{}{}
	}
	for key := range candidates {
		excludedForTrendingOnly[key] = struct{}{}
	}

	trendingCandidates := groupTrendingSignals(trendingRows)
	// A feed-language read failure degrades to an unfiltered trending-only list rather than dropping those cards entirely.
	if langRows, err := b.languages.ListFeedLanguages(ctx); err != nil {
		slog.Warn("/api/discover/sources: list feed languages failed", "err", err)
	} else {
		langByKey := make(map[string]discoverlang.Language, len(langRows))
		for _, row := range langRows {
			if row.Language != nil && *row.Language != "" {
				langByKey[feedkey.Normalize(row.FeedUrl)] = discoverlang.Language(*row.Language)
			}
		}
		// Accept-Language is the cold-start fallback when subscriptions yield no language (no locale field exists yet elsewhere in the app).
		subLangs := make([]discoverlang.Language, 0, len(subRows))
		for _, s := range subRows {
			subLangs = append(subLangs, langByKey[feedkey.Normalize(s.FeedUrl)])
		}
		readerLangs := discoverlang.ReaderLanguages(subLangs, discoverlang.ParseAcceptLanguage(acceptLanguage))
		for i := range trendingCandidates {
			trendingCandidates[i].Language = langByKey[trendingCandidates[i].Key]
		}
		trendingCandidates = discoverrank.FilterByLanguage(trendingCandidates, readerLangs)
	}

	return DiscoverSourcesPayload{
		personal:                list,
		excluded:                excluded,
		excludedForTrendingOnly: excludedForTrendingOnly,
		trendingKeys:            trendingKeys,
		trending:                trendingCandidates,
	}, nil
}

// renderDiscoverSources ranks and slices one page out of an assembled payload. Pure: the same payload and cursor always produce the same page, which is what makes serving a cursor page off the memo safe.
func renderDiscoverSources(payload DiscoverSourcesPayload, cursor discoverCursor) ([]DiscoverSourceWire, discoverCursor, bool) {
	rankingNow := time.Unix(0, cursor.RankedAt).UTC()
	personalPage := discoverrank.Page[discoverrank.Suggestion]{Items: []discoverrank.Ranked[discoverrank.Suggestion]{}}
	if !cursor.Personal.Done {
		personalPage = discoverrank.RankPage(payload.personal, payload.excluded, discoverPageSize, cursor.Seed, rankingNow, cursor.Personal.After)
	}
	trendingPage := discoverrank.Page[discoverrank.Suggestion]{Items: []discoverrank.Ranked[discoverrank.Suggestion]{}}
	if !cursor.Trending.Done {
		trendingPage = discoverrank.RankTrendingPage(payload.trending, payload.excludedForTrendingOnly, discoverPageSize, cursor.Seed, rankingNow, cursor.Trending.After)
	}

	page := balanceDiscoverPages(cursor, personalPage, trendingPage)
	out := make([]DiscoverSourceWire, 0, len(page.Personal)+len(page.Trending))
	for _, ranked := range page.Personal {
		s := ranked.Value
		wire := DiscoverSourceWire{
			Key:     s.Key,
			Kind:    s.Kind,
			Title:   s.Title,
			SiteURL: s.SiteURL,
			Reason: DiscoverReasonWire{
				StrongCount:        s.Reason.StrongCount,
				WeakCount:          s.Reason.WeakCount,
				TopFollowerDID:     s.Reason.TopFollowerDID,
				TopFollowerNetwork: string(s.Reason.TopFollowerNetwork),
				TopSignal:          s.Reason.TopSignal.String(),
				SelfSourceApp:      string(s.Reason.SelfApp),
				FollowerDIDs:       s.Reason.FollowerDIDs,
			},
		}
		if s.Reason.TopSignal == discoverrank.SignalAuthor {
			wire.Reason.AuthorDID = s.Reason.TopFollowerDID
		}
		if _, ok := payload.trendingKeys[s.Key]; ok {
			wire.Reason.Trending = true
		}
		if s.Kind == "standardfeed" {
			wire.Publication = s.Key
		} else {
			wire.FeedURL = s.Key
		}
		out = append(out, wire)
	}
	for _, ranked := range page.Trending {
		s := ranked.Value
		wire := DiscoverSourceWire{Key: s.Key, Kind: s.Kind, Title: s.Title, SiteURL: s.SiteURL, Reason: DiscoverReasonWire{Trending: true}}
		if s.Kind == "standardfeed" {
			wire.Publication = s.Key
		} else {
			wire.FeedURL = s.Key
		}
		out = append(out, wire)
	}
	return out, page.Cursor, page.HasMore
}

// addSignal folds one raw signal into the candidates map. Reaction signals pass empty title/siteUrl, which never clobbers a value a subscribe/author signal already set. kind is ignored: a carried kind can be poisoned (e.g. a foreign reader mirroring an at:// publication into a plain feedUrl field), so Kind is always derived fresh from key shape.
func addSignal(
	candidates map[string]*discoverrank.Candidate,
	key, kind, title, siteURL string,
	follower discoverrank.Follower,
	signalKind discoverrank.SignalKind,
	at time.Time,
) {
	if key == "" {
		return
	}
	c, ok := candidates[key]
	if !ok {
		c = &discoverrank.Candidate{Key: key, Kind: feedkey.Kind(key)}
		candidates[key] = c
	}
	if c.Title == "" {
		c.Title = title
	}
	if c.SiteURL == "" {
		c.SiteURL = siteURL
	}
	follower.Signal = discoverrank.Signal{Kind: signalKind, At: at}
	c.Followers = append(c.Followers, follower)
}

// backfillMissingTitles fills title/siteUrl for reaction-only rss candidates from the network-wide trending table in one read, since a share/save signal carries neither (see resolveReactionKey). Not gated by the trending quality bar: a title only needs one contributing repo. A lookup miss or error skips those candidates; it must never fail the request.
func backfillMissingTitles(ctx context.Context, reader DiscoverSourceTitleBackfillReader, candidates map[string]*discoverrank.Candidate) {
	keys := make([]string, 0, len(candidates))
	for key, c := range candidates {
		if c.Kind != "rss" || c.Title != "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return
	}
	sort.Strings(keys)

	rows, err := reader.ListDiscoverTrendingSignalTitles(ctx, keys)
	if err != nil {
		slog.Debug("/api/discover/sources: title backfill lookup failed", "keys", len(keys), "err", err)
		return
	}
	for _, row := range rows {
		c, ok := candidates[row.SourceKey]
		if !ok || c.Title != "" {
			continue // first titled row per source key wins
		}
		c.Title = derefOptString(row.Title)
		if c.SiteURL == "" {
			c.SiteURL = derefOptString(row.SiteUrl)
		}
	}
}

// resolveReactionKey resolves a share/save to its source key: feedUrl provenance first, then Tier-2 document/itemUrl lookup as fallback; an unresolved reaction returns ok=false. SPEC <discovery>.
// feedkey.Normalize runs on every branch since Tier-2 lookups return feed_url verbatim; it must land on the same key a normalized subscribe/author signal set (mirrors internal/discoverbatch/signals.go).
func resolveReactionKey(ctx context.Context, resolver DiscoverEntryResolver, feedURL, document, itemURL string) (string, bool) {
	if feedURL != "" {
		return feedkey.Normalize(feedURL), true
	}
	if document != "" {
		if fu, err := resolver.GetFeedURLByGuid(ctx, document); err == nil && fu != "" {
			return feedkey.Normalize(fu), true
		}
	}
	if itemURL != "" {
		if fu, err := resolver.GetFeedURLByItemURL(ctx, itemURL); err == nil && fu != "" {
			return feedkey.Normalize(fu), true
		}
	}
	return "", false
}

// kindForReactionKey infers a Tier-2 kind from key shape when no subscribe/author signal already set it.
func kindForReactionKey(key string) string {
	return feedkey.Kind(key)
}

// parseDiscoverTime parses a record's RFC3339 timestamp; empty or malformed values degrade to the zero Time, which discoverrank treats as neutral "unknown recency."
func parseDiscoverTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// groupTrendingSignals folds the aggregate table's flat rows into one Candidate per source key, each Follower standing in for one contributing repo.
func groupTrendingSignals(rows []db.DiscoverTrendingSignal) []discoverrank.Candidate {
	bySource := map[string]*discoverrank.Candidate{}
	for _, row := range rows {
		c, ok := bySource[row.SourceKey]
		if !ok {
			c = &discoverrank.Candidate{Key: row.SourceKey, Kind: feedkey.Kind(row.SourceKey), Title: derefOptString(row.Title), SiteURL: derefOptString(row.SiteUrl)}
			bySource[row.SourceKey] = c
		}
		c.Followers = append(c.Followers, discoverrank.Follower{
			DID:  row.RepoDid,
			Tier: discoverrank.TierStrong,
			Signal: discoverrank.Signal{
				Kind: signalKindFromWire(row.SignalKind),
				At:   parseDiscoverTime(derefOptString(row.SignalAt)),
			},
		})
	}
	out := make([]discoverrank.Candidate, 0, len(bySource))
	for _, c := range bySource {
		out = append(out, *c)
	}
	return out
}

func signalKindFromWire(s string) discoverrank.SignalKind {
	switch s {
	case "author":
		return discoverrank.SignalAuthor
	case "subscribe":
		return discoverrank.SignalSubscribe
	case "share":
		return discoverrank.SignalShare
	default:
		return discoverrank.SignalSave
	}
}

func derefOptString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

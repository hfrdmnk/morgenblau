package sync

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"golang.org/x/sync/errgroup"

	"morgenblau/internal/atprepo"
	"morgenblau/internal/database/db"
	"morgenblau/internal/jobs"
	"morgenblau/internal/tags"
)

// guardWindow matches SPEC <feed-sources>: a repeat login or refresh within
// this window collapses to the already-running job id rather than starting
// a new one.
const guardWindow = 5 * time.Minute

// PDSLister snapshots a user's blue.morgen.feed.* and site.standard.graph.*
// records. Stubbed in tests with a canned fake.
type PDSLister interface {
	ListSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSSubscription, error)
	ListSaves(ctx context.Context, sess *oauth.ClientSession) ([]PDSSave, error)
	ListStandardSubscriptions(ctx context.Context, sess *oauth.ClientSession) ([]PDSStandardSubscription, error)
	ListShares(ctx context.Context, sess *oauth.ClientSession) ([]PDSShare, error)
	ListRecommends(ctx context.Context, sess *oauth.ClientSession) ([]PDSRecommend, error)
}

// PDSStandardSubscription is the trimmed shape of a site.standard.graph
// subscription record — the sole existence authority for publication sources.
// CreatedAt is optional in the standard lexicon.
type PDSStandardSubscription struct {
	URI         string
	Rkey        string
	Publication string
	CreatedAt   string
}

// PDSSubscription is the trimmed shape of a blue.morgen.feed.subscription
// record, dispatched on the `source` union. rssFeed variants carry FeedURL
// (+SiteURL) and are sources in their own right; standardPublication variants
// carry Publication and are metadata sidecars of a site.standard.graph
// subscription record.
type PDSSubscription struct {
	URI         string
	Rkey        string
	Kind        string // "rss" | "standardfeed"
	FeedURL     string // rssFeed variant
	SiteURL     string // rssFeed variant
	Publication string // standardPublication variant (at-uri)
	Title       string
	Primary     bool
	Tags        []string
}

// PDSSave is the trimmed shape of a blue.morgen.feed.save record we mirror into
// the user_saves index. feedUrl is optional on the record.
type PDSSave struct {
	URI       string
	Rkey      string
	ItemURL   string
	FeedURL   string
	CreatedAt string
}

// PDSShare is the trimmed shape of a blue.morgen.feed.share record. Shares
// with a Document are comment sidecars of a site.standard.graph.recommend
// record; shares without one are rss shares in their own right.
type PDSShare struct {
	URI       string
	Rkey      string
	ItemURL   string
	Document  string
	FeedURL   string
	Comment   string
	CreatedAt string
}

// PDSRecommend is the trimmed shape of a site.standard.graph.recommend
// record — the sole existence authority for a standardfeed share.
type PDSRecommend struct {
	URI       string
	Rkey      string
	Document  string
	CreatedAt string
}

// SyncStore is the slice of *db.Queries SyncUser depends on. Defined here so
// the orchestrator's full surface remains hideable behind one interface.
type SyncStore interface {
	ListUserSubscriptionsForSync(ctx context.Context, did string) ([]db.ListUserSubscriptionsForSyncRow, error)
	UpsertUserSubscription(ctx context.Context, arg db.UpsertUserSubscriptionParams) error
	DeleteUserSubscription(ctx context.Context, arg db.DeleteUserSubscriptionParams) error
	UpsertFeed(ctx context.Context, arg db.UpsertFeedParams) error
	ListUserSavesForSync(ctx context.Context, did string) ([]db.ListUserSavesForSyncRow, error)
	UpsertUserSave(ctx context.Context, arg db.UpsertUserSaveParams) error
	DeleteUserSave(ctx context.Context, arg db.DeleteUserSaveParams) error
	ListUserSharesForSync(ctx context.Context, did string) ([]db.ListUserSharesForSyncRow, error)
	UpsertUserShare(ctx context.Context, arg db.UpsertUserShareParams) error
	DeleteUserShare(ctx context.Context, arg db.DeleteUserShareParams) error
}

// SessionResumer hands SyncUser an authenticated session for the given DID,
// independent of any incoming request. The login path may complete before the
// SyncUser goroutine starts, so we resume by (did, sessionID).
type SessionResumer interface {
	ResumeSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSession, error)
}

// Engine is the deep module surface — SyncUser hides errgroup orchestration,
// PDS reconcile, dual-track fan-out, and the 5-min in-flight guard.
type Engine struct {
	jobs    *jobs.Tracker
	store   SyncStore
	lister  PDSLister
	fetcher FeedFetcher
	resumer SessionResumer
	pds     atprepo.Writer
	now     func() time.Time

	parentCtx context.Context
	wg        *sync.WaitGroup
}

// attachLifecycle binds the engine to a parent ctx + WaitGroup so its goroutines
// participate in graceful shutdown.
func (e *Engine) attachLifecycle(ctx context.Context, wg *sync.WaitGroup) {
	e.parentCtx = ctx
	e.wg = wg
}

// NewEngine wires the dependencies. resumer may be nil for tests that call
// runDualTrack directly. pds is used only to delete orphaned blue.morgen
// sidecar records — the single reconcile write exception; nil disables it.
func NewEngine(
	tracker *jobs.Tracker,
	store SyncStore,
	lister PDSLister,
	fetcher FeedFetcher,
	resumer SessionResumer,
	pds atprepo.Writer,
) *Engine {
	return &Engine{
		jobs:      tracker,
		store:     store,
		lister:    lister,
		fetcher:   fetcher,
		resumer:   resumer,
		pds:       pds,
		now:       time.Now,
		parentCtx: context.Background(),
	}
}

// SyncUser orchestrates the dual-track refresh for the given (did, sessionID).
// Returns the created job's id. The 5-minute in-flight guard coalesces repeat
// triggers — a second call within the guard window returns the existing id.
func (e *Engine) SyncUser(ctx context.Context, did syntax.DID, sessionID string, trigger jobs.Trigger) (string, error) {
	j, existed := e.jobs.CreateOrReturnExisting(jobs.KindSyncUser, did, trigger, guardWindow)
	if existed {
		return j.ID, nil
	}
	if e.wg != nil {
		e.wg.Add(1)
	}
	go func() {
		if e.wg != nil {
			defer e.wg.Done()
		}
		e.run(j.ID, did, sessionID)
	}()
	return j.ID, nil
}

func (e *Engine) run(id string, did syntax.DID, sessionID string) {
	e.jobs.SetRunning(id)
	bg, cancel := context.WithTimeout(e.parentCtx, 5*time.Minute)
	defer cancel()

	sess, err := e.resumer.ResumeSession(bg, did, sessionID)
	if err != nil {
		slog.Warn("sync_user: resume failed", "did", did, "err", err)
		e.jobs.SetFailed(id)
		return
	}

	if err := e.runDualTrack(bg, did, sess); err != nil {
		slog.Warn("sync_user: failed", "did", did, "err", err)
		e.jobs.SetFailed(id)
		return
	}
	e.jobs.SetDone(id)
}

// runDualTrack is the heart of the engine — public-by-package so tests can
// drive it directly without the goroutine wrapping.
func (e *Engine) runDualTrack(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	// Snapshot Tier-1 BEFORE reconcile so Phase 1B doesn't wait on 1A.
	snapshot, err := e.store.ListUserSubscriptionsForSync(ctx, did.String())
	if err != nil {
		return err
	}
	snapURLs := make([]string, 0, len(snapshot))
	for _, row := range snapshot {
		snapURLs = append(snapURLs, row.FeedUrl)
	}

	// addedFeedURLs is populated by Phase 1A; Phase 2 reads it after fan-in.
	var (
		addedMu       sync.Mutex
		addedFeedURLs []string
	)

	g, gctx := errgroup.WithContext(ctx)

	// Phase 1A: PDS reconcile.
	g.Go(func() error {
		return e.reconcileTier1(gctx, did, sess, snapshot, func(url string) {
			addedMu.Lock()
			addedFeedURLs = append(addedFeedURLs, url)
			addedMu.Unlock()
		})
	})

	// Phase 1B: Local-known fan-out fetch.
	g.Go(func() error {
		fetchAll(gctx, snapURLs, e.fetcher)
		return nil
	})

	// Phase 1C: saves reconcile. Independent of the subscription/fetch tracks
	// and best-effort — a saves hiccup must never fail the primary refresh.
	g.Go(func() error {
		if err := e.reconcileSaves(gctx, did, sess); err != nil {
			slog.Warn("sync_user: saves reconcile failed", "did", did, "err", err)
		}
		return nil
	})

	// Phase 1D: shares reconcile. Same contract as saves: best-effort.
	g.Go(func() error {
		if err := e.reconcileShares(gctx, did, sess); err != nil {
			slog.Warn("sync_user: shares reconcile failed", "did", did, "err", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	// Phase 2: top-up — fetch newly-discovered URLs that 1B didn't already cover.
	already := make(map[string]struct{}, len(snapURLs))
	for _, u := range snapURLs {
		already[u] = struct{}{}
	}
	var topUp []string
	addedMu.Lock()
	for _, u := range addedFeedURLs {
		if _, ok := already[u]; !ok {
			topUp = append(topUp, u)
		}
	}
	addedMu.Unlock()
	fetchAll(ctx, topUp, e.fetcher)
	return nil
}

func fetchAll(ctx context.Context, urls []string, f FeedFetcher) {
	if len(urls) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			if err := f.FetchAndStore(ctx, url); err != nil {
				slog.Debug("sync_user: fetch failed", "url", url, "err", err)
			}
		}(u)
	}
	wg.Wait()
}

func (e *Engine) reconcileTier1(
	ctx context.Context,
	did syntax.DID,
	sess *oauth.ClientSession,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	onAdded func(feedURL string),
) error {
	// Both lists are fetched before ANY mutation: a failed standard listing
	// must not let the rss pass run against a partial picture, and vice
	// versa — deletes against a partial snapshot would wipe healthy sources.
	remote, err := e.lister.ListSubscriptions(ctx, sess)
	if err != nil {
		return err
	}
	standard, err := e.lister.ListStandardSubscriptions(ctx, sess)
	if err != nil {
		return err
	}
	e.reconcileRSS(ctx, did, snapshot, remote, onAdded)
	e.reconcileStandardfeed(ctx, did, sess, snapshot, remote, standard, onAdded)
	return nil
}

func (e *Engine) reconcileRSS(
	ctx context.Context,
	did syntax.DID,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	remote []PDSSubscription,
	onAdded func(feedURL string),
) {
	// The rss pass diffs rssFeed-variant records against kind=rss local rows.
	// standardPublication variants are metadata sidecars, not sources — they
	// join the standardfeed pass instead. Local standardfeed rows must never
	// be deleted by this pass.
	localByRkey := make(map[string]db.ListUserSubscriptionsForSyncRow, len(snapshot))
	for _, row := range snapshot {
		if row.Kind == "standardfeed" {
			continue
		}
		localByRkey[row.Rkey] = row
	}
	remoteByRkey := make(map[string]PDSSubscription, len(remote))
	for _, r := range remote {
		if r.Kind != "rss" {
			continue
		}
		remoteByRkey[r.Rkey] = r
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// Inserts + updates from remote.
	for rkey, r := range remoteByRkey {
		_, existed := localByRkey[rkey]
		// Tier-2 first; only on success can the FK from feed_entries.feed_url
		// be satisfied — so onAdded (Phase 2 fetch trigger) is gated on it.
		if err := e.store.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   r.FeedURL,
			SiteUrl:   nilIfEmpty(r.SiteURL),
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("reconcile: Tier-2 upsert failed", "feedUrl", r.FeedURL, "err", err)
			continue
		}
		if !existed {
			onAdded(r.FeedURL)
		}
		if err := e.store.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
			Did:       didStr,
			Rkey:      rkey,
			AtUri:     r.URI,
			FeedUrl:   r.FeedURL,
			Title:     nilIfEmpty(r.Title),
			IsPrimary: boolToInt64(r.Primary),
			Tags:      tags.Marshal(r.Tags),
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("reconcile: Tier-1 upsert failed", "err", err)
		}
	}

	// Deletes: locals that no longer exist remotely.
	for rkey := range localByRkey {
		if _, stillThere := remoteByRkey[rkey]; stillThere {
			continue
		}
		if err := e.store.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{
			Did:  didStr,
			Rkey: rkey,
		}); err != nil {
			slog.Warn("reconcile: Tier-1 delete failed", "err", err)
		}
	}
}

// reconcileStandardfeed applies the publication-source model: the standard
// record is the sole existence authority, the blue.morgen standardPublication
// variant is a metadata sidecar joined by publication at-uri. Duplicate
// standard records for one publication collapse to the smallest rkey (TID ⇒
// earliest created). Orphaned sidecars — publication no longer subscribed —
// are deleted from the PDS: the single reconcile write exception.
func (e *Engine) reconcileStandardfeed(
	ctx context.Context,
	did syntax.DID,
	sess *oauth.ClientSession,
	snapshot []db.ListUserSubscriptionsForSyncRow,
	morgen []PDSSubscription,
	standard []PDSStandardSubscription,
	onAdded func(feedURL string),
) {
	localStd := make(map[string]db.ListUserSubscriptionsForSyncRow)
	for _, row := range snapshot {
		if row.Kind == "standardfeed" {
			localStd[row.Rkey] = row
		}
	}

	var sidecars []PDSSubscription
	sidecarByPub := make(map[string]PDSSubscription)
	for _, s := range morgen {
		if s.Kind != "standardfeed" {
			continue
		}
		sidecars = append(sidecars, s)
		if cur, ok := sidecarByPub[s.Publication]; !ok || s.Rkey < cur.Rkey {
			if ok {
				slog.Warn("reconcile: duplicate sidecar for publication", "publication", s.Publication, "kept", s.Rkey, "dropped", cur.Rkey)
			}
			sidecarByPub[s.Publication] = s
		}
	}

	canonicalByPub := make(map[string]PDSStandardSubscription)
	for _, s := range standard {
		if cur, ok := canonicalByPub[s.Publication]; !ok || s.Rkey < cur.Rkey {
			canonicalByPub[s.Publication] = s
		}
	}
	desired := make(map[string]PDSStandardSubscription, len(canonicalByPub))
	for _, canon := range canonicalByPub {
		desired[canon.Rkey] = canon
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// Deletes FIRST — when the canonical rkey for a publication changes
	// (duplicate collapse, delete+recreate in another app), the stale local
	// row still holds the publication key and the new upsert would trip
	// UNIQUE(did, feed_url).
	for rkey := range localStd {
		if _, keep := desired[rkey]; keep {
			continue
		}
		if err := e.store.DeleteUserSubscription(ctx, db.DeleteUserSubscriptionParams{
			Did:  didStr,
			Rkey: rkey,
		}); err != nil {
			slog.Warn("reconcile: standardfeed Tier-1 delete failed", "err", err)
		}
	}

	// Upserts. Tier-2 before Tier-1 (FK), onAdded gated on Tier-2 success —
	// the dispatched Phase-2 fetch resolves name/site/icon, so reconcile
	// itself never calls a publisher PDS.
	for rkey, canon := range desired {
		if err := e.store.UpsertFeed(ctx, db.UpsertFeedParams{
			FeedUrl:   canon.Publication,
			Kind:      "standardfeed",
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("reconcile: standardfeed Tier-2 upsert failed", "publication", canon.Publication, "err", err)
			continue
		}
		if _, existed := localStd[rkey]; !existed {
			onAdded(canon.Publication)
		}
		var (
			title       *string
			primary     int64
			tagsJSON    *string
			sidecarRkey *string
		)
		if sc, ok := sidecarByPub[canon.Publication]; ok {
			title = nilIfEmpty(sc.Title)
			primary = boolToInt64(sc.Primary)
			tagsJSON = tags.Marshal(sc.Tags)
			rk := sc.Rkey
			sidecarRkey = &rk
		}
		if err := e.store.UpsertUserSubscription(ctx, db.UpsertUserSubscriptionParams{
			Did:         didStr,
			Rkey:        rkey,
			AtUri:       canon.URI,
			FeedUrl:     canon.Publication,
			Kind:        "standardfeed",
			SidecarRkey: sidecarRkey,
			Title:       title,
			IsPrimary:   primary,
			Tags:        tagsJSON,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			slog.Warn("reconcile: standardfeed Tier-1 upsert failed", "err", err)
		}
	}

	// Orphaned sidecars: the standard record is gone (unsubscribed in another
	// app), so the metadata sidecar is dead weight — delete it from the PDS.
	// Covered by the blue.morgen grant; no standard-scope check needed.
	for _, sc := range sidecars {
		if _, alive := canonicalByPub[sc.Publication]; alive {
			continue
		}
		if e.pds == nil {
			continue
		}
		if err := e.pds.DeleteRecord(ctx, sess, syntax.NSID(subscriptionCollection), sc.Rkey); err != nil {
			slog.Warn("reconcile: orphaned sidecar delete failed", "rkey", sc.Rkey, "err", err)
		}
	}
}

// reconcileSaves diffs the user's blue.morgen.feed.save records on the PDS
// against the local user_saves index and applies inserts/updates/deletes.
// Simpler than reconcileTier1: saves are leaf bookmarks, so there's no Tier-2
// feed upsert and no fetch to trigger.
func (e *Engine) reconcileSaves(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	snapshot, err := e.store.ListUserSavesForSync(ctx, did.String())
	if err != nil {
		return err
	}
	remote, err := e.lister.ListSaves(ctx, sess)
	if err != nil {
		return err
	}

	localByRkey := make(map[string]db.ListUserSavesForSyncRow, len(snapshot))
	for _, row := range snapshot {
		localByRkey[row.Rkey] = row
	}
	remoteByRkey := make(map[string]PDSSave, len(remote))
	for _, r := range remote {
		remoteByRkey[r.Rkey] = r
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// Inserts + updates from remote.
	for rkey, r := range remoteByRkey {
		createdAt := r.CreatedAt
		if createdAt == "" {
			createdAt = now
		}
		if err := e.store.UpsertUserSave(ctx, db.UpsertUserSaveParams{
			Did:       didStr,
			Rkey:      rkey,
			AtUri:     r.URI,
			ItemUrl:   r.ItemURL,
			FeedUrl:   nilIfEmpty(r.FeedURL),
			CreatedAt: createdAt,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("reconcile: saves upsert failed", "rkey", rkey, "err", err)
		}
	}

	// Deletes: locals that no longer exist remotely.
	for rkey := range localByRkey {
		if _, stillThere := remoteByRkey[rkey]; stillThere {
			continue
		}
		if err := e.store.DeleteUserSave(ctx, db.DeleteUserSaveParams{
			Did:  didStr,
			Rkey: rkey,
		}); err != nil {
			slog.Warn("reconcile: saves delete failed", "rkey", rkey, "err", err)
		}
	}
	return nil
}

// reconcileShares diffs the user's shares across two record types. A
// site.standard.graph.recommend record is the existence authority for a
// standardfeed share; its comment/itemUrl live on a joined blue.morgen.feed.share
// sidecar (matched by document at-uri). A blue.morgen.feed.share with no document
// is an rss share in its own right. The local rkey of a standardfeed row is the
// RECOMMEND rkey — the sidecar is metadata, never its own row. Orphaned sidecars
// — the recommend is gone — are deleted from the PDS: the second reconcile write
// exception (the first is orphaned subscription sidecars).
func (e *Engine) reconcileShares(ctx context.Context, did syntax.DID, sess *oauth.ClientSession) error {
	snapshot, err := e.store.ListUserSharesForSync(ctx, did.String())
	if err != nil {
		return err
	}
	// Both lists before ANY mutation: a partial picture would let deletes wipe
	// healthy shares of the other kind.
	shares, err := e.lister.ListShares(ctx, sess)
	if err != nil {
		return err
	}
	recommends, err := e.lister.ListRecommends(ctx, sess)
	if err != nil {
		return err
	}

	// Partition blue.morgen shares: document-bearing ones are recommend
	// sidecars (metadata); document-less ones are rss shares (sources).
	var (
		rssShares    []PDSShare
		sidecars     []PDSShare
		sidecarByDoc = make(map[string]PDSShare)
	)
	for _, s := range shares {
		if s.Document == "" {
			rssShares = append(rssShares, s)
			continue
		}
		sidecars = append(sidecars, s)
		if cur, ok := sidecarByDoc[s.Document]; !ok || s.Rkey < cur.Rkey {
			if ok {
				slog.Warn("reconcile: duplicate share sidecar for document", "document", s.Document, "kept", s.Rkey, "dropped", cur.Rkey)
			}
			sidecarByDoc[s.Document] = s
		}
	}

	// Canonical recommend per document = smallest rkey (TID ⇒ earliest created).
	canonicalByDoc := make(map[string]PDSRecommend)
	for _, r := range recommends {
		if cur, ok := canonicalByDoc[r.Document]; !ok || r.Rkey < cur.Rkey {
			canonicalByDoc[r.Document] = r
		}
	}

	// Desired local rkeys = canonical recommend rkeys ∪ rss share rkeys.
	desired := make(map[string]struct{}, len(canonicalByDoc)+len(rssShares))
	for _, rec := range canonicalByDoc {
		desired[rec.Rkey] = struct{}{}
	}
	for _, s := range rssShares {
		desired[s.Rkey] = struct{}{}
	}

	now := e.now().UTC().Format(time.RFC3339)
	didStr := did.String()

	// Deletes FIRST — when the canonical recommend rkey changes, the stale local
	// row still holds (did, document); the partial unique index would trip the
	// new upsert otherwise.
	for _, row := range snapshot {
		if _, keep := desired[row.Rkey]; keep {
			continue
		}
		if err := e.store.DeleteUserShare(ctx, db.DeleteUserShareParams{
			Did:  didStr,
			Rkey: row.Rkey,
		}); err != nil {
			slog.Warn("reconcile: share delete failed", "rkey", row.Rkey, "err", err)
		}
	}

	// Upserts: canonical recommends as standardfeed rows with sidecar metadata.
	for _, rec := range canonicalByDoc {
		doc := rec.Document
		createdAt := rec.CreatedAt
		if createdAt == "" {
			createdAt = now
		}
		var (
			itemURL     *string
			comment     *string
			feedURL     *string
			sidecarRkey *string
		)
		if sc, ok := sidecarByDoc[doc]; ok {
			itemURL = nilIfEmpty(sc.ItemURL)
			comment = nilIfEmpty(sc.Comment)
			feedURL = nilIfEmpty(sc.FeedURL)
			rk := sc.Rkey
			sidecarRkey = &rk
		}
		if err := e.store.UpsertUserShare(ctx, db.UpsertUserShareParams{
			Did:         didStr,
			Rkey:        rec.Rkey,
			AtUri:       rec.URI,
			Kind:        "standardfeed",
			ItemUrl:     itemURL,
			Document:    &doc,
			Comment:     comment,
			FeedUrl:     feedURL,
			SidecarRkey: sidecarRkey,
			CreatedAt:   createdAt,
			UpdatedAt:   now,
		}); err != nil {
			slog.Warn("reconcile: share upsert failed", "rkey", rec.Rkey, "err", err)
		}
	}

	// Upserts: rss shares (document-less) as sources in their own right.
	for _, s := range rssShares {
		createdAt := s.CreatedAt
		if createdAt == "" {
			createdAt = now
		}
		itemURL := s.ItemURL
		if err := e.store.UpsertUserShare(ctx, db.UpsertUserShareParams{
			Did:       didStr,
			Rkey:      s.Rkey,
			AtUri:     s.URI,
			Kind:      "rss",
			ItemUrl:   &itemURL,
			Comment:   nilIfEmpty(s.Comment),
			FeedUrl:   nilIfEmpty(s.FeedURL),
			CreatedAt: createdAt,
			UpdatedAt: now,
		}); err != nil {
			slog.Warn("reconcile: rss share upsert failed", "rkey", s.Rkey, "err", err)
		}
	}

	// Orphaned sidecars: the recommend is gone (unshared in another app), so the
	// comment sidecar is dead weight — delete it from the PDS. Covered by the
	// blue.morgen grant; no standard-scope check needed.
	for _, sc := range sidecars {
		if _, alive := canonicalByDoc[sc.Document]; alive {
			continue
		}
		if e.pds == nil {
			continue
		}
		if err := e.pds.DeleteRecord(ctx, sess, syntax.NSID(shareCollection), sc.Rkey); err != nil {
			slog.Warn("reconcile: orphaned share sidecar delete failed", "rkey", sc.Rkey, "err", err)
		}
	}
	return nil
}

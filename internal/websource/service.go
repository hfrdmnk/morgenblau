package websource

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"morgenblau/internal/backoff"
	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
	"morgenblau/internal/postkey"
	"morgenblau/internal/webfeed"
)

const (
	previewItems    = 10
	feedItems       = 20
	historyPolls    = 6
	maxExcerptRunes = 280
	maxBackoff      = 24 * time.Hour
	pollBatch       = 50
	modelRepairGap  = 24 * time.Hour
	checkEvery      = 7 * 24 * time.Hour
	// An open weekly alarm is checked again this soon, so a wrong doubt does not silence a source for a week.
	checkRetry = 24 * time.Hour
	// An unresolved site records a repair every poll; a month is enough to read its story.
	keepRepairs = 30 * 24 * time.Hour
	// More model repairs than this in a week means the page fights the recipe; someone should look.
	weeklyModelRepairs = 3
	// An unhealthy verdict younger than this is returned as is, so a reader retrying at once does not regenerate.
	staleSetup = 10 * time.Minute
	// maxItems matches webfeed.MaxItems; it is enforced again here because every write goes through this package.
	maxItems = 200
	// dailySources caps the new sites one reader can make Morgenblau set up in a day; each may spend model calls.
	dailySources = 30
	// setupSlots bounds concurrent setups, each of which can hold tens of megabytes of pages.
	setupSlots       = 2
	modelSpendWindow = 24 * time.Hour
	// sectionChecksPerSweep spreads the one-time section check of older sources over sweeps, so a deploy does not stampede sites.
	sectionChecksPerSweep = 3
)

// SetupTimeout bounds one setup, waiting for a slot included; the create route keeps its connection open longer.
const SetupTimeout = 90 * time.Second

// timeLayout is fixed-width UTC so stored times order correctly as strings.
const timeLayout = "2006-01-02T15:04:05Z"

type Config struct {
	// PublicBaseURL is the origin readers reach this server on; feed URLs are PublicBaseURL + "/feed/" + id.
	PublicBaseURL string
	PollInterval  time.Duration
	// Limiter caps generations per reader DID; nil means no cap.
	Limiter Limiter
	// ModelDailyUSD caps model spend over any 24 hours across all sources; past it setup and repair stay heuristic.
	ModelDailyUSD float64
}

type Service struct {
	writer        *sql.DB
	read          *db.Queries
	gen           Generator
	limiter       Limiter
	base          string
	interval      time.Duration
	backoff       backoff.Policy
	modelDailyUSD float64
	setupTimeout  time.Duration
	slots         chan struct{}
	now           func() time.Time

	mu       sync.Mutex
	inflight map[string]bool
}

// NewService takes a nil Generator when generation is not configured: Create then answers ErrUnavailable and nothing polls.
func NewService(reader, writer *sql.DB, gen Generator, cfg Config) *Service {
	s := &Service{
		writer:        writer,
		read:          db.New(reader),
		gen:           gen,
		limiter:       cfg.Limiter,
		base:          strings.TrimSuffix(cfg.PublicBaseURL, "/"),
		interval:      cfg.PollInterval,
		modelDailyUSD: cfg.ModelDailyUSD,
		setupTimeout:  SetupTimeout,
		slots:         make(chan struct{}, setupSlots),
		now:           time.Now,
		inflight:      map[string]bool{},
	}
	if cfg.PollInterval > 0 {
		s.backoff.Steps = backoff.Exponential(2*cfg.PollInterval, 2, maxBackoff)
	}
	return s
}

// FeedURL is the address readers subscribe to for one web source.
func (s *Service) FeedURL(id string) string { return s.FeedURLPrefixes().Current + id }

// FeedPrefixes are every generated feed's URL up to its id, for SQL that recognizes them by prefix; the zero value matches none.
type FeedPrefixes struct {
	Current string
	// Legacy is where feeds were served first; readers' PDS records still name it, so it stays one of ours.
	Legacy string
}

func (s *Service) FeedURLPrefixes() FeedPrefixes {
	return FeedPrefixes{Current: s.base + feedPath, Legacy: s.base + legacyFeedPath}
}

// IDFromFeedURL names the web source a feed URL serves, when it is one of this server's generated feeds.
func (s *Service) IDFromFeedURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	base, berr := url.Parse(s.base)
	if err != nil || berr != nil || base.Host == "" || !strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) {
		return "", false
	}
	return feedID(u.Path)
}

// SameFeedURLs are the URLs serving the same generated feed as raw, current first; none when raw is not one of ours.
func (s *Service) SameFeedURLs(raw string) []string {
	id, ok := s.IDFromFeedURL(raw)
	if !ok {
		return nil
	}
	p := s.FeedURLPrefixes()
	return []string{p.Current + id, p.Legacy + id}
}

// IsFeedURL reports one of this server's generated feeds.
func (s *Service) IsFeedURL(raw string) bool {
	_, ok := s.IDFromFeedURL(raw)
	return ok
}

// Create returns the site's web source, generating it on first use. An anchorURL on a source that is not healthy, or an
// unsure setup older than staleSetup, re-runs setup; the new one replaces the old only when it is ok or healthier, and an
// anchored one on a source others follow only once vetAnchor passes it. An unanchored re-run never fails the request.
func (s *Service) Create(ctx context.Context, did, listingURL, anchorURL string) (Preview, error) {
	listing, err := parseHTTPURL(listingURL)
	if err != nil {
		return Preview{}, err
	}
	postkey.StripTracking(listing)
	if anchorURL != "" {
		anchor, err := parseHTTPURL(anchorURL)
		if err != nil {
			return Preview{}, err
		}
		if !sameSite(anchor.Hostname(), listing.Hostname()) {
			return Preview{}, fmt.Errorf("%w: anchor %q is not on %s", ErrInvalidURL, anchorURL, listing.Hostname())
		}
		anchorURL = anchor.String()
	}
	key := listingKey(listing)
	offered := db.OfferWebSourceByListingKeyParams{ListingKey: key, OfferedAt: formatTime(s.now().UTC())}
	existing, err := db.New(s.writer).OfferWebSourceByListingKey(ctx, offered)
	switch {
	case err == nil:
		takesAnchor := Health(existing.Health) != HealthOK || Health(existing.SetupHealth) == HealthNeedsAttention
		if !takesAnchor {
			anchorURL = ""
		}
		if anchorURL != "" {
			return s.generate(ctx, did, key, listing, anchorURL, existing, true)
		}
		if !s.staleUnsure(existing) {
			return s.preview(ctx, existing)
		}
		p, err := s.generate(ctx, did, key, listing, "", existing, true)
		if err != nil {
			// The reader asked for the shared feed, not this refresh, so whatever stopped it the existing source serves.
			slog.Info("web source refresh skipped", "id", existing.ID, "err", err)
			return s.preview(ctx, existing)
		}
		return p, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Preview{}, err
	}
	return s.generate(ctx, did, key, listing, anchorURL, db.WebSource{}, false)
}

// generate runs a setup for the site and stores it: a new source, or when regenerate a candidate to replace existing.
func (s *Service) generate(ctx context.Context, did, key string, listing *url.URL, anchorURL string, existing db.WebSource, regenerate bool) (Preview, error) {
	if s.gen == nil {
		return Preview{}, ErrUnavailable
	}
	release, err := s.claimReader(did)
	if err != nil {
		return Preview{}, err
	}
	defer release()
	if !regenerate {
		if err := s.underDailyCap(ctx, did); err != nil {
			return Preview{}, err
		}
	}
	if s.limiter != nil {
		if wait, ok := s.limiter.Allow(did); !ok {
			return Preview{}, &RateLimitError{RetryAfter: wait}
		}
	}

	// One deadline covers the setup and its model check, so the answer still reaches the reader's open request.
	setupCtx, cancel := context.WithTimeout(ctx, s.setupTimeout)
	defer cancel()
	gen, err := s.runSetup(setupCtx, listing.String(), anchorURL, true)
	if err != nil {
		if regenerate {
			// The deadline may be what failed the setup, and the next visitor should not pay for it again at once.
			tried := db.MarkWebSourceSetupTriedParams{ID: existing.ID, TriedAt: formatTime(s.now().UTC())}
			if merr := db.New(s.writer).MarkWebSourceSetupTried(context.WithoutCancel(ctx), tried); merr != nil {
				return Preview{}, errors.Join(err, merr)
			}
		}
		return Preview{}, err
	}
	if regenerate && anchorURL != "" {
		rejected, err := s.vetAnchor(setupCtx, did, existing, gen)
		if err != nil {
			return Preview{}, err
		}
		if rejected != "" {
			slog.Info("web source anchor rejected", "id", existing.ID, "why", rejected)
			p, err := s.preview(ctx, existing)
			p.AnchorRejected = rejected
			return p, err
		}
	}
	setup, err := newSetupRow(key, listing, gen)
	if err != nil {
		return Preview{}, err
	}
	if anchorURL != "" {
		setup.anchoredBy = did
	}
	if regenerate {
		setup.replaces = existing.ID
	}
	id, kept, err := s.storeSetup(ctx, did, setup)
	if err != nil {
		return Preview{}, err
	}
	row, err := s.read.GetWebSource(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	p, err := s.preview(ctx, row)
	if kept && anchorURL != "" {
		p.AnchorRejected = AnchorNoBetter
	}
	return p, err
}

// setupRow is a finished setup as the source row stores it. key and listingURL name the listing the setup ended on;
// pastedKey names the one it was given, which differs when the setup moved to a section page.
type setupRow struct {
	key, pastedKey, listingURL, siteURL, title, recipe, cost string
	anchoredBy                                               string
	// replaces is the source a regenerated setup is a candidate for; atSeq, when set, drops it once another setup replaced that source.
	replaces string
	atSeq    *int64
	gen      Generated
}

// vetAnchor holds an anchored setup that would replace a source other readers follow to two checks: the page links the
// reader's post, and the model agrees the new list is the site's posts. It says why the source stays, or nothing.
func (s *Service) vetAnchor(ctx context.Context, did string, existing db.WebSource, gen Generated) (AnchorRejection, error) {
	followed, err := s.read.IsWebSourceFollowedByOthers(ctx, db.IsWebSourceFollowedByOthersParams{
		FeedUrl: s.FeedURL(existing.ID), LegacyFeedUrl: s.FeedURLPrefixes().Legacy + existing.ID, Did: did,
	})
	if err != nil || !followed {
		return "", err
	}
	if !gen.AnchorOnPage {
		return AnchorNotOnPage, nil
	}
	if !improves(gen.Health, existing) {
		return "", nil
	}
	if gen.Confirm == nil {
		return AnchorNotConfirmed, nil
	}
	if left, err := s.modelBudgetLeft(ctx); err != nil || !left {
		return AnchorNotConfirmed, err
	}
	c, err := gen.Confirm(ctx)
	// The spend lands even when the setup's deadline cut the call short: it was paid for.
	if serr := s.recordModelSpend(context.WithoutCancel(ctx), "anchor_check", c.USD); serr != nil {
		return "", serr
	}
	if err != nil || !c.Agreed {
		slog.Info("web source anchor not confirmed", "id", existing.ID, "reason", c.Reason, "err", err)
		return AnchorNotConfirmed, nil
	}
	return "", nil
}

// newSetupRow fills in what a setup left blank: the recipe, the title and the site's address.
func newSetupRow(key string, listing *url.URL, gen Generated) (setupRow, error) {
	cost, err := json.Marshal(gen.Cost)
	if err != nil {
		return setupRow{}, err
	}
	ended := endedOn(listing, gen.ListingURL)
	row := setupRow{
		key: listingKey(ended), pastedKey: key, listingURL: ended.String(), recipe: string(gen.Recipe), cost: string(cost), gen: gen,
		title: strings.TrimSpace(gen.Title), siteURL: strings.TrimSpace(gen.SiteURL),
	}
	if row.recipe == "" {
		row.recipe = "{}"
	}
	if row.title == "" {
		row.title = strings.TrimPrefix(listing.Hostname(), "www.")
	}
	if row.siteURL == "" {
		row.siteURL = listing.Scheme + "://" + listing.Host + "/"
	}
	return row, nil
}

// endedOn is the listing a setup of listing ended on: the section page it moved to, else listing itself.
func endedOn(listing *url.URL, genListing string) *url.URL {
	u, err := parseHTTPURL(genListing)
	// A move off the site would let a paste of one site claim another site's key.
	if err != nil || !sameSite(u.Hostname(), listing.Hostname()) || listingKey(u) == listingKey(listing) {
		return listing
	}
	postkey.StripTracking(u)
	return u
}

// storeSetup writes a setup in one transaction: a new source, or a replacement of setup.replaces when the setup improves
// on it; kept reports a regenerated setup that did not. A source is keyed by the listing its setup ended on, and the
// pasted listing becomes an alias when the setup moved to a section page. Its items join as history; a healthy setup
// also starts the detector's history.
func (s *Service) storeSetup(ctx context.Context, did string, setup setupRow) (id string, kept bool, err error) {
	gen := setup.gen
	now := s.now().UTC()
	nextPoll := formatTime(now.Add(s.interval))
	var anchoredBy, anchoredAt *string
	if setup.anchoredBy != "" {
		anchoredBy, anchoredAt = &setup.anchoredBy, ptr(formatTime(now))
	}
	err = database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		if err := recordSpend(ctx, q, now, "setup", gen.Cost.USD); err != nil {
			return err
		}
		row, err := storedSource(ctx, q, setup)
		switch {
		case err == nil && row.ID != setup.replaces:
			// Another reader generated this site, or the section it moved to, while our setup ran; theirs stands.
			id = row.ID
			return addAlias(ctx, q, setup.pastedKey, row)
		case err == nil:
			id = row.ID
			if setup.atSeq != nil && row.SetupSeq != *setup.atSeq {
				slog.Info("web source setup dropped; a newer setup replaced the source while it ran", "id", id)
				kept = true
				return nil
			}
			if !improves(gen.Health, row) {
				slog.Info("web source setup kept the old one", "id", id, "old", row.SetupHealth, "new", gen.Health)
				kept = true
				return q.MarkWebSourceSetupTried(ctx, db.MarkWebSourceSetupTriedParams{ID: id, TriedAt: formatTime(now)})
			}
			if err := q.ReplaceWebSourceSetup(ctx, db.ReplaceWebSourceSetupParams{
				ID: id, ListingUrl: setup.listingURL, SiteUrl: setup.siteURL, Title: setup.title, Recipe: setup.recipe,
				Health: string(gen.Health), HealthReason: gen.HealthReason,
				SetupCost: setup.cost, UpdatedAt: formatTime(now), NextPollAt: &nextPoll,
				AnchoredByDid: anchoredBy, AnchoredAt: anchoredAt,
			}); err != nil {
				return err
			}
			if err := rekey(ctx, q, row, setup.key); err != nil {
				return err
			}
			// Readers already have the old items, so they stay; the detector's history belongs to the old recipe.
			if err := q.DeleteWebSourcePolls(ctx, id); err != nil {
				return err
			}
		case errors.Is(err, sql.ErrNoRows):
			if id, err = newID(); err != nil {
				return err
			}
			if err := q.CreateWebSource(ctx, db.CreateWebSourceParams{
				ID: id, ListingKey: setup.key, ListingUrl: setup.listingURL, SiteUrl: setup.siteURL, Title: setup.title, Recipe: setup.recipe,
				Health: string(gen.Health), HealthReason: gen.HealthReason,
				SetupCost: setup.cost, CreatedByDid: did, CreatedAt: formatTime(now), NextPollAt: &nextPoll,
				AnchoredByDid: anchoredBy, AnchoredAt: anchoredAt,
			}); err != nil {
				return err
			}
			if err := addAlias(ctx, q, setup.pastedKey, db.WebSource{ID: id, ListingKey: setup.key}); err != nil {
				return err
			}
		default:
			return err
		}
		items := dedupeItems(gen.Items)
		if err := insertItems(ctx, q, id, items, now, true); err != nil {
			return err
		}
		if gen.Health == HealthOK {
			return recordPoll(ctx, q, id, now, items, gen.Fingerprint)
		}
		return nil
	})
	return id, kept, err
}

// storedSource is the source a setup lands on: the one it replaces, else the one its pasted listing or the section it
// moved to already has.
func storedSource(ctx context.Context, q *db.Queries, setup setupRow) (db.WebSource, error) {
	if setup.replaces != "" {
		if row, err := q.GetWebSource(ctx, setup.replaces); !errors.Is(err, sql.ErrNoRows) {
			return row, err
		}
	}
	row, err := q.GetWebSourceByKey(ctx, setup.pastedKey)
	if errors.Is(err, sql.ErrNoRows) && setup.key != setup.pastedKey {
		return q.GetWebSourceByKey(ctx, setup.key)
	}
	return row, err
}

// addAlias lets key find row; a key another source already answers to stays with it.
func addAlias(ctx context.Context, q *db.Queries, key string, row db.WebSource) error {
	if key == row.ListingKey {
		return nil
	}
	return q.InsertWebSourceAlias(ctx, db.InsertWebSourceAliasParams{ListingKey: key, SourceID: row.ID})
}

// rekey moves row to the key of the listing its new setup ended on, and its old key becomes an alias. When another source
// already answers to that key, row keeps its own: ids never change and sources never merge.
func rekey(ctx context.Context, q *db.Queries, row db.WebSource, key string) error {
	if key == row.ListingKey {
		return nil
	}
	switch owner, err := q.GetWebSourceByKey(ctx, key); {
	case err == nil && owner.ID != row.ID:
		slog.Info("web source keeps its key; another source has the listing its setup moved to", "id", row.ID, "other", owner.ID)
		return nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if err := q.DeleteWebSourceAlias(ctx, db.DeleteWebSourceAliasParams{ListingKey: key, SourceID: row.ID}); err != nil {
		return err
	}
	if err := q.RekeyWebSource(ctx, db.RekeyWebSourceParams{ID: row.ID, ListingKey: key}); err != nil {
		return err
	}
	return q.InsertWebSourceAlias(ctx, db.InsertWebSourceAliasParams{ListingKey: row.ListingKey, SourceID: row.ID})
}

// runSetup runs one setup in a slot; mayUseModel lets it use the model while the daily budget lasts.
func (s *Service) runSetup(ctx context.Context, listingURL, anchorURL string, mayUseModel bool) (Generated, error) {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return Generated{}, ErrBusy
	}
	defer func() { <-s.slots }()
	allowModel := false
	if mayUseModel {
		var err error
		if allowModel, err = s.modelBudgetLeft(ctx); err != nil {
			return Generated{}, err
		}
	}
	gen, err := s.gen.Setup(ctx, listingURL, anchorURL, allowModel)
	if err == nil && !gen.Health.valid() {
		err = fmt.Errorf("unknown health %q", gen.Health)
	}
	if err != nil {
		// A failed setup's model calls were paid for, and its deadline may be what failed it.
		if serr := s.recordModelSpend(context.WithoutCancel(ctx), "setup", gen.Cost.USD); serr != nil {
			return Generated{}, errors.Join(fmt.Errorf("%w: %w", ErrGeneration, err), serr)
		}
		return Generated{}, fmt.Errorf("%w: %w", ErrGeneration, err)
	}
	return gen, nil
}

// claimReader allows one setup per reader at a time; a second would only double the bill for the same click.
func (s *Service) claimReader(did string) (release func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[did] {
		return nil, ErrSetupInFlight
	}
	s.inflight[did] = true
	return func() {
		s.mu.Lock()
		delete(s.inflight, did)
		s.mu.Unlock()
	}, nil
}

func (s *Service) underDailyCap(ctx context.Context, did string) error {
	now := s.now().UTC()
	created, err := s.read.ListWebSourcesCreatedSince(ctx, db.ListWebSourcesCreatedSinceParams{
		Did: did, Since: formatTime(now.Add(-24 * time.Hour)), Limit: dailySources,
	})
	if err != nil {
		return err
	}
	if len(created) < dailySources {
		return nil
	}
	oldest, err := time.Parse(timeLayout, created[0])
	if err != nil {
		return err
	}
	return &RateLimitError{RetryAfter: max(time.Second, oldest.Add(24*time.Hour).Sub(now))}
}

// modelBudgetLeft reports whether today's model spend leaves room for another model step.
func (s *Service) modelBudgetLeft(ctx context.Context) (bool, error) {
	spent, err := s.read.SumModelSpendSince(ctx, formatTime(s.now().UTC().Add(-modelSpendWindow)))
	if err != nil {
		return false, err
	}
	if spent >= s.modelDailyUSD {
		slog.Warn("web model budget spent; setup and repair stay heuristic", "spent_usd", spent, "budget_usd", s.modelDailyUSD)
		return false, nil
	}
	return true, nil
}

func (s *Service) recordModelSpend(ctx context.Context, kind string, usd float64) error {
	if usd <= 0 {
		return nil
	}
	return database.WithTx(ctx, s.writer, func(q *db.Queries) error { return recordSpend(ctx, q, s.now().UTC(), kind, usd) })
}

func recordSpend(ctx context.Context, q *db.Queries, now time.Time, kind string, usd float64) error {
	if usd <= 0 {
		return nil
	}
	if err := q.InsertModelSpend(ctx, db.InsertModelSpendParams{SpentAt: formatTime(now), Kind: kind, Usd: usd}); err != nil {
		return err
	}
	return q.PruneModelSpend(ctx, formatTime(now.Add(-2*modelSpendWindow)))
}

// staleUnsure is a setup that doubted its own list and was last tried long enough ago to try again; poll outages do not count.
func (s *Service) staleUnsure(row db.WebSource) bool {
	if Health(row.SetupHealth) == HealthOK {
		return false
	}
	tried, err := time.Parse(timeLayout, row.SetupTriedAt)
	return err != nil || s.now().Sub(tried) >= staleSetup
}

// improves says a new setup verdict should replace the source's: it is ok, or better than both the old setup and the source now.
func improves(next Health, row db.WebSource) bool {
	return next == HealthOK || (next.rank() > Health(row.SetupHealth).rank() && next.rank() > Health(row.Health).rank())
}

func (h Health) rank() int {
	switch h {
	case HealthOK:
		return 3
	case HealthNeedsAttention:
		return 2
	case HealthBlockedBySite:
		return 1
	}
	return 0
}

func (s *Service) preview(ctx context.Context, row db.WebSource) (Preview, error) {
	rows, err := s.read.ListWebSourceFeedItems(ctx, db.ListWebSourceFeedItemsParams{SourceID: row.ID, Limit: previewItems})
	if err != nil {
		return Preview{}, err
	}
	count, err := s.read.CountWebSourceItems(ctx, row.ID)
	if err != nil {
		return Preview{}, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, feedItem(r))
	}
	return Preview{Source: s.source(row), Items: items, ItemCount: int(count)}, nil
}

func (s *Service) source(row db.WebSource) Source {
	return Source{
		ID: row.ID, FeedURL: s.FeedURL(row.ID), ListingURL: row.ListingUrl, SiteURL: row.SiteUrl, Title: row.Title,
		Health: Health(row.Health), HealthReason: row.HealthReason, NeedsAnchor: Health(row.SetupHealth) == HealthNeedsAttention,
	}
}

// PollDue polls every followed web source whose next poll is due and reports how many it polled.
func (s *Service) PollDue(ctx context.Context) (int, error) {
	if s.gen == nil {
		return 0, ErrUnavailable
	}
	now := formatTime(s.now().UTC())
	prefixes := s.FeedURLPrefixes()
	servedSince := formatTime(s.now().UTC().Add(-database.FeedSweepGrace))
	due, err := s.read.ListDueWebSources(ctx, db.ListDueWebSourcesParams{
		Now: &now, FeedUrlPrefix: prefixes.Current, LegacyFeedUrlPrefix: prefixes.Legacy, ServedSince: &servedSince, Limit: pollBatch,
	})
	if err != nil {
		return 0, err
	}
	polled, checks := 0, 0
	for _, src := range due {
		if err := s.pollOne(ctx, src); err != nil {
			if ctx.Err() != nil {
				return polled, ctx.Err()
			}
			slog.Warn("web source poll", "id", src.ID, "err", err)
			continue
		}
		polled++
		// The check follows the poll, so a source is polled as usual whatever the check finds or fails at.
		if checks < sectionChecksPerSweep && needsSectionCheck(src) {
			checks++
			if err := s.checkSection(ctx, src); err != nil {
				if ctx.Err() != nil {
					return polled, ctx.Err()
				}
				slog.Info("web source section check left for a later sweep", "id", src.ID, "err", err)
			}
		}
	}
	return polled, nil
}

// needsSectionCheck is a plain streams or css setup stored before setup looked for a section page.
func needsSectionCheck(src db.WebSource) bool {
	var r webfeed.Recipe
	if src.AnchoredByDid != nil || json.Unmarshal([]byte(src.Recipe), &r) != nil {
		return false
	}
	return (r.Mode == webfeed.ModeStreams || r.Mode == webfeed.ModeCSS) && r.Setup.Anchor == "" && !r.Setup.SectionChecked
}

// checkSection runs one free setup of a source's listing. A healthy move to a section page becomes the source's setup,
// so readers gain the section's posts as history; any other answer marks the recipe checked so the check never repeats.
func (s *Service) checkSection(ctx context.Context, src db.WebSource) error {
	listing, err := parseHTTPURL(src.ListingUrl)
	if err != nil {
		return err
	}
	setupCtx, cancel := context.WithTimeout(ctx, s.setupTimeout)
	defer cancel()
	gen, err := s.runSetup(setupCtx, listing.String(), "", false)
	if err != nil && !errors.Is(err, ErrBrowserBuilt) {
		return err
	}
	setup, err := newSetupRow(listingKey(listing), listing, gen)
	if err != nil {
		return err
	}
	if setup.key != setup.pastedKey && gen.Health == HealthOK {
		slog.Info("web source moves to its section page", "id", src.ID, "from", src.ListingUrl, "to", setup.listingURL)
		setup.replaces, setup.atSeq = src.ID, &src.SetupSeq
		_, _, err = s.storeSetup(ctx, src.CreatedByDid, setup)
		return err
	}
	_, err = db.New(s.writer).MarkWebSourceSectionChecked(ctx, db.MarkWebSourceSectionCheckedParams{
		ID: src.ID, SetupSeq: src.SetupSeq, UpdatedAt: formatTime(s.now().UTC()),
	})
	return err
}

func (s *Service) pollOne(ctx context.Context, src db.WebSource) error {
	now := s.now().UTC()
	prev, err := s.pollState(ctx, src, now)
	if err != nil {
		return err
	}
	out, err := s.gen.Poll(ctx, json.RawMessage(src.Recipe), prev)
	if err != nil {
		if ctx.Err() != nil {
			// A repair cut short by shutdown already paid for its model calls, so the budget still counts them.
			if r := out.Repair; r != nil {
				if serr := s.recordModelSpend(context.WithoutCancel(ctx), "repair", r.USD); serr != nil {
					slog.Warn("web source repair spend", "id", src.ID, "err", serr)
				}
			}
			return ctx.Err()
		}
		slog.Warn("web source poll failed", "id", src.ID, "err", err)
		out = PollOutcome{Verdict: VerdictBroken, Reason: "the site could not be reached"}
	}

	if prev.WeeklyAlarm && out.Verdict == VerdictOK && !out.Checked && !answersAlarm(out.Repair) {
		out = PollOutcome{Verdict: VerdictSuspect, Reason: src.HealthReason, Repair: out.Repair}
	}
	update, publish := s.pollUpdate(src, out, now)
	return s.storePoll(ctx, src, out, update, publish, now)
}

// pollUpdate is the source row after a poll, and whether the poll's items are published.
func (s *Service) pollUpdate(src db.WebSource, out PollOutcome, now time.Time) (update db.UpdateWebSourcePollParams, publish bool) {
	update = db.UpdateWebSourcePollParams{
		ID: src.ID, SetupSeq: src.SetupSeq, Recipe: src.Recipe, LastPolledAt: ptr(formatTime(now)), UpdatedAt: formatTime(now),
		Etag: src.Etag, LastModified: src.LastModified, LinkSetHash: src.LinkSetHash,
	}
	if out.Checked {
		update.LastCheckedAt = ptr(formatTime(now))
	}
	switch out.Verdict {
	case VerdictOK:
		update.Health, update.HealthReason = string(HealthOK), ""
		update.NextPollAt = ptr(formatTime(now.Add(s.interval)))
		update.Etag = orKeep(out.ETag, src.Etag)
		update.LastModified = orKeep(out.LastModified, src.LastModified)
		update.LinkSetHash = orKeep(out.LinkSetHash, src.LinkSetHash)
		if len(out.Recipe) > 0 {
			update.Recipe = string(out.Recipe)
		}
		publish = !out.NotModified
	case VerdictSuspect:
		update.Health, update.HealthReason = string(HealthNeedsAttention), out.Reason
		update.NextPollAt = ptr(formatTime(now.Add(s.interval)))
	default:
		update.Health, update.HealthReason = string(HealthBroken), out.Reason
		if out.Blocked {
			update.Health = string(HealthBlockedBySite)
		}
		update.ConsecutiveFailures = src.ConsecutiveFailures + 1
		update.NextPollAt = ptr(formatTime(now.Add(s.backoff.Delay(int(update.ConsecutiveFailures)))))
	}
	// Only a later setup answers an unsure one; its own polls trust the very list it doubted.
	if out.Verdict != VerdictBroken && Health(src.SetupHealth) != HealthOK {
		update.Health, update.HealthReason = src.SetupHealth, src.SetupHealthReason
	}
	return update, publish
}

// storePoll writes one poll in a transaction: spend, the source row, its repair record and, when publish, the new items.
func (s *Service) storePoll(ctx context.Context, src db.WebSource, out PollOutcome, update db.UpdateWebSourcePollParams, publish bool, now time.Time) error {
	return database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		if err := recordSpend(ctx, q, now, "check", out.CheckUSD); err != nil {
			return err
		}
		if r := out.Repair; r != nil {
			if err := recordSpend(ctx, q, now, "repair", r.USD); err != nil {
				return err
			}
		}
		// The update goes first: it matches nothing when a setup replaced the source meanwhile, and then the poll writes nothing.
		if r := out.Repair; r != nil && r.UsedModel {
			n, err := q.CountModelRepairsSince(ctx, db.CountModelRepairsSinceParams{SourceID: src.ID, Since: formatTime(now.Add(-checkEvery))})
			if err != nil {
				return err
			}
			if n+1 > weeklyModelRepairs {
				slog.Warn("web source repair budget exceeded", "id", src.ID, "model_repairs_7d", n+1, "listing", src.ListingUrl)
				update.HealthReason = joinReasons(update.HealthReason, "the page changed often this week")
			}
		}
		updated, err := q.UpdateWebSourcePoll(ctx, update)
		if err != nil {
			return err
		}
		if updated == 0 {
			slog.Info("web source poll dropped; a new setup replaced the source while it ran", "id", src.ID)
			return nil
		}
		if r := out.Repair; r != nil {
			if err := q.InsertWebSourceRepair(ctx, db.InsertWebSourceRepairParams{
				SourceID: src.ID, AttemptedAt: formatTime(now), Cause: string(r.Cause), Outcome: string(r.Outcome),
				Step: r.Step, Rule: r.Rule, Reason: r.Reason, UsedModel: boolInt(r.UsedModel), Usd: r.USD,
			}); err != nil {
				return err
			}
			if err := q.PruneWebSourceRepairs(ctx, db.PruneWebSourceRepairsParams{SourceID: src.ID, Before: formatTime(now.Add(-keepRepairs))}); err != nil {
				return err
			}
		}
		if !publish {
			return nil
		}
		items := dedupeItems(out.Items)
		fresh, moved, err := unpublished(ctx, q, src.ID, items)
		if err != nil {
			return err
		}
		if err := insertItems(ctx, q, src.ID, fresh, now, false); err != nil {
			return err
		}
		if err := insertItems(ctx, q, src.ID, moved, now, true); err != nil {
			return err
		}
		return recordPoll(ctx, q, src.ID, now, items, out.Fingerprint)
	})
}

// answersAlarm is a repair the weekly alarm accepts: one that left the doubted list for another.
func answersAlarm(r *RepairReport) bool {
	return r != nil && r.Cause == RepairCauseWeeklyCheck && r.Outcome == RepairAccepted
}

func joinReasons(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func (s *Service) pollState(ctx context.Context, src db.WebSource, now time.Time) (PollState, error) {
	rows, err := s.read.ListWebSourcePolls(ctx, db.ListWebSourcePollsParams{SourceID: src.ID, Limit: historyPolls})
	if err != nil {
		return PollState{}, err
	}
	state := PollState{ETag: deref(src.Etag), LastModified: deref(src.LastModified), LinkSetHash: deref(src.LinkSetHash)}
	for _, r := range rows {
		rec := PollRecord{Fingerprint: Fingerprint{URLTemplate: r.UrlTemplate, DateFrac: r.DateFrac, TitleShape: r.TitleShape}}
		if err := json.Unmarshal([]byte(r.Urls), &rec.URLs); err != nil {
			return PollState{}, fmt.Errorf("poll %d urls: %w", r.ID, err)
		}
		if err := json.Unmarshal([]byte(r.Titles), &rec.Titles); err != nil {
			return PollState{}, fmt.Errorf("poll %d titles: %w", r.ID, err)
		}
		state.History = append(state.History, rec)
	}
	known, err := s.read.ListWebSourceKnownItems(ctx, src.ID)
	if err != nil {
		return PollState{}, err
	}
	for _, k := range known {
		it := Item{URL: k.Url, Title: k.Title}
		if k.PublishedAt != nil {
			if it.Date, err = time.Parse(timeLayout, *k.PublishedAt); err != nil {
				return PollState{}, fmt.Errorf("item %s date: %w", k.Url, err)
			}
		}
		state.Known = append(state.Known, it)
	}

	modelLeft, err := s.modelBudgetLeft(ctx)
	if err != nil {
		return PollState{}, err
	}
	state.AllowModelRepair = modelLeft
	switch last, err := s.read.GetLastModelRepairAt(ctx, src.ID); {
	case err == nil:
		at, perr := time.Parse(timeLayout, last)
		state.AllowModelRepair = modelLeft && perr == nil && now.Sub(at) >= modelRepairGap
	case !errors.Is(err, sql.ErrNoRows):
		return PollState{}, err
	}

	state.WeeklyAlarm, err = s.weeklyAlarmOpen(ctx, src)
	if err != nil {
		return PollState{}, err
	}
	since, every := src.SetupAt, checkEvery
	if src.LastCheckedAt != nil {
		since = *src.LastCheckedAt
	}
	if state.WeeklyAlarm {
		every = checkRetry
	}
	if at, err := time.Parse(timeLayout, since); err == nil {
		state.CheckDue = modelLeft && now.Sub(at) >= every
	}
	return state, nil
}

// weeklyAlarmOpen reports a weekly-check alarm that no later healthy check, accepted repair onto another list or fresh
// setup has answered. Ordinary polls cannot close it: they trust the same list the check doubted.
func (s *Service) weeklyAlarmOpen(ctx context.Context, src db.WebSource) (bool, error) {
	last, err := s.read.GetLastWeeklyRepair(ctx, src.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return RepairOutcome(last.Outcome) == RepairUnresolved && src.LastCheckedAt != nil &&
		last.AttemptedAt >= *src.LastCheckedAt && last.AttemptedAt >= src.SetupAt, nil
}

// unpublished drops items already in the feed and items dated before its oldest one, which defends against decoy lists
// that share the stream's URL shape. A post the site moved to a new URL shape joins as history, so a URL migration does
// not refill today's newspaper; a known title at a URL of the old shape is a recurring title, so a new post.
func unpublished(ctx context.Context, q *db.Queries, sourceID string, items []Item) (fresh, moved []Item, err error) {
	known, err := q.ListWebSourceKnownItems(ctx, sourceID)
	if err != nil {
		return nil, nil, err
	}
	urls := make(map[string]bool, len(known))
	byTitle := make(map[string][]string, len(known))
	for _, k := range known {
		urls[postkey.URL(k.Url)] = true
		byTitle[postkey.Title(k.Title)] = append(byTitle[postkey.Title(k.Title)], k.Url)
	}
	current := make(map[string]bool, len(items))
	for _, it := range items {
		current[postkey.URL(it.URL)] = true
	}
	var oldest time.Time
	switch raw, err := q.GetOldestWebSourceItemDate(ctx, sourceID); {
	case err == nil && raw != nil:
		if oldest, err = time.Parse(timeLayout, *raw); err != nil {
			return nil, nil, fmt.Errorf("oldest item date: %w", err)
		}
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return nil, nil, err
	}
	for _, it := range items {
		switch {
		case urls[postkey.URL(it.URL)] || (!it.Date.IsZero() && !oldest.IsZero() && it.Date.Before(oldest)):
		case movedFrom(it.URL, byTitle[postkey.Title(it.Title)], current):
			moved = append(moved, it)
		default:
			fresh = append(fresh, it)
		}
	}
	return fresh, moved, nil
}

// movedFrom reports a known post with the same title whose URL had another shape and is gone from the page now.
func movedFrom(newURL string, sameTitle []string, current map[string]bool) bool {
	shape := webfeed.URLTemplate(newURL)
	for _, old := range sameTitle {
		if !current[postkey.URL(old)] && webfeed.URLTemplate(old) != shape {
			return true
		}
	}
	return false
}

// dedupeItems is the gate every stored item passes: one per URL, only http(s) links, at most maxItems.
func dedupeItems(items []Item) []Item {
	seen := make(map[string]bool, len(items))
	out := make([]Item, 0, min(len(items), maxItems))
	for _, it := range items {
		if len(out) == maxItems {
			break
		}
		it.URL = strings.TrimSpace(it.URL)
		key := postkey.URL(it.URL)
		if seen[key] || !isWebLink(it.URL) {
			continue
		}
		seen[key] = true
		it.Title = strings.TrimSpace(it.Title)
		if it.Title == "" {
			it.Title = it.URL
		}
		out = append(out, it)
	}
	return out
}

func isWebLink(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}

func insertItems(ctx context.Context, q *db.Queries, sourceID string, items []Item, now time.Time, history bool) error {
	for i, it := range items {
		var published *string
		if !it.Date.IsZero() {
			published = ptr(formatTime(it.Date))
		}
		if err := q.InsertWebSourceItem(ctx, db.InsertWebSourceItemParams{
			SourceID: sourceID, Url: it.URL, Title: it.Title, PublishedAt: published, Excerpt: excerpt(it.Excerpt),
			FirstSeenAt: formatTime(now), History: boolInt(history), Position: int64(i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func recordPoll(ctx context.Context, q *db.Queries, sourceID string, now time.Time, items []Item, fp Fingerprint) error {
	urls := make([]string, len(items))
	titles := make([]string, len(items))
	for i, it := range items {
		urls[i], titles[i] = it.URL, it.Title
	}
	urlsJSON, err := json.Marshal(urls)
	if err != nil {
		return err
	}
	titlesJSON, err := json.Marshal(titles)
	if err != nil {
		return err
	}
	if err := q.InsertWebSourcePoll(ctx, db.InsertWebSourcePollParams{
		SourceID: sourceID, PolledAt: formatTime(now), ItemCount: int64(len(items)), Urls: string(urlsJSON), Titles: string(titlesJSON),
		UrlTemplate: fp.URLTemplate, DateFrac: fp.DateFrac, TitleShape: fp.TitleShape,
	}); err != nil {
		return err
	}
	return q.PruneWebSourcePolls(ctx, db.PruneWebSourcePollsParams{SourceID: sourceID, Keep: historyPolls})
}

// feedItem applies the history rule: a setup item without a date has none in the feed; a later one is dated when Morgenblau first saw it.
func feedItem(r db.ListWebSourceFeedItemsRow) Item {
	it := Item{URL: r.Url, Title: r.Title, Excerpt: r.Excerpt}
	switch {
	case r.PublishedAt != nil:
		it.Date, _ = time.Parse(timeLayout, *r.PublishedAt)
	case r.History == 0:
		it.Date, _ = time.Parse(timeLayout, r.FirstSeenAt)
	}
	return it
}

func excerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= maxExcerptRunes {
		return s
	}
	cut := string(runes[:maxExcerptRunes])
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return cut + "…"
}

func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidURL, raw)
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	return u, nil
}

// listingKey names the site a listing URL belongs to, so scheme, www., case, trailing slashes, fragments and tracking
// parameters do not make a second web source.
func listingKey(u *url.URL) string {
	c := *u
	postkey.StripTracking(&c)
	return postkey.FromURL(&c)
}

// sameSite holds when one host is the other or a subdomain of it, www. aside.
func sameSite(a, b string) bool {
	a, b = strings.TrimPrefix(strings.ToLower(a), "www."), strings.TrimPrefix(strings.ToLower(b), "www.")
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

func (h Health) valid() bool {
	switch h {
	case HealthOK, HealthNeedsAttention, HealthBlockedBySite, HealthBroken:
		return true
	}
	return false
}

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func orKeep(v string, old *string) *string {
	if v == "" {
		return old
	}
	return &v
}

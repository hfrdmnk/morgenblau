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

	"morgenblau/internal/database"
	"morgenblau/internal/database/db"
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
)

// SetupTimeout bounds one setup, waiting for a slot included; the create route keeps its connection open longer.
const SetupTimeout = 90 * time.Second

// timeLayout is fixed-width UTC so stored times order correctly as strings.
const timeLayout = "2006-01-02T15:04:05Z"

type Config struct {
	// PublicBaseURL is the origin readers reach this server on; feed URLs are PublicBaseURL + "/web/" + id.
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
	modelDailyUSD float64
	setupTimeout  time.Duration
	slots         chan struct{}
	now           func() time.Time

	mu       sync.Mutex
	inflight map[string]bool
}

// NewService takes a nil Generator when generation is not configured: Create then answers ErrUnavailable and nothing polls.
func NewService(reader, writer *sql.DB, gen Generator, cfg Config) *Service {
	return &Service{
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
}

func (s *Service) FeedURL(id string) string { return s.base + "/web/" + id }

// Create returns the site's web source, generating it on first use. An anchorURL on a source that is not healthy, or an
// unsure setup older than staleSetup, re-runs setup; the new one replaces the old only when it is ok or healthier.
func (s *Service) Create(ctx context.Context, did, listingURL, anchorURL string) (Preview, error) {
	listing, err := parseHTTPURL(listingURL)
	if err != nil {
		return Preview{}, err
	}
	stripTracking(listing)
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
	existing, err := s.read.GetWebSourceByListingKey(ctx, key)
	regenerate := false
	switch {
	case err == nil:
		takesAnchor := Health(existing.Health) != HealthOK || existing.NeedsAnchor != 0
		if !takesAnchor {
			anchorURL = ""
		}
		regenerate = anchorURL != "" || s.staleUnsure(existing)
		if !regenerate {
			return s.preview(ctx, existing)
		}
	case !errors.Is(err, sql.ErrNoRows):
		return Preview{}, err
	}
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

	gen, err := s.runSetup(ctx, listing.String(), anchorURL)
	if err != nil {
		return Preview{}, err
	}
	recipe := string(gen.Recipe)
	if recipe == "" {
		recipe = "{}"
	}
	cost, err := json.Marshal(gen.Cost)
	if err != nil {
		return Preview{}, err
	}
	title := strings.TrimSpace(gen.Title)
	if title == "" {
		title = strings.TrimPrefix(listing.Hostname(), "www.")
	}
	siteURL := strings.TrimSpace(gen.SiteURL)
	if siteURL == "" {
		siteURL = listing.Scheme + "://" + listing.Host + "/"
	}
	now := s.now().UTC()
	nextPoll := formatTime(now.Add(s.interval))

	var id string
	err = database.WithTx(ctx, s.writer, func(q *db.Queries) error {
		if err := recordSpend(ctx, q, now, "setup", gen.Cost.USD); err != nil {
			return err
		}
		row, err := q.GetWebSourceByListingKey(ctx, key)
		switch {
		case err == nil && !regenerate:
			// Another reader generated this site while our setup ran; theirs stands.
			id = row.ID
			return nil
		case err == nil:
			id = row.ID
			if !improves(gen.Health, row) {
				slog.Info("web source setup kept the old one", "id", id, "old", row.SetupHealth, "new", gen.Health)
				return q.MarkWebSourceSetupTried(ctx, db.MarkWebSourceSetupTriedParams{ID: id, TriedAt: formatTime(now)})
			}
			if err := q.ReplaceWebSourceSetup(ctx, db.ReplaceWebSourceSetupParams{
				ID: id, ListingUrl: listing.String(), SiteUrl: siteURL, Title: title, Recipe: recipe,
				Health: string(gen.Health), HealthReason: gen.HealthReason, NeedsAnchor: boolInt(gen.NeedsAnchor),
				SetupCost: string(cost), UpdatedAt: formatTime(now), NextPollAt: &nextPoll,
			}); err != nil {
				return err
			}
			// Readers already have the old items, so they stay; the detector's history belongs to the old recipe.
			if err := q.DeleteWebSourcePolls(ctx, id); err != nil {
				return err
			}
		case errors.Is(err, sql.ErrNoRows):
			id, err = newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateWebSource(ctx, db.CreateWebSourceParams{
				ID: id, ListingKey: key, ListingUrl: listing.String(), SiteUrl: siteURL, Title: title, Recipe: recipe,
				Health: string(gen.Health), HealthReason: gen.HealthReason, NeedsAnchor: boolInt(gen.NeedsAnchor),
				SetupCost: string(cost), CreatedByDid: did, CreatedAt: formatTime(now), NextPollAt: &nextPoll,
			}); err != nil {
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
	if err != nil {
		return Preview{}, err
	}
	row, err := s.read.GetWebSource(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	return s.preview(ctx, row)
}

// runSetup runs one setup in a slot, bounded by the setup deadline, with the model only while the daily budget lasts.
func (s *Service) runSetup(ctx context.Context, listingURL, anchorURL string) (Generated, error) {
	ctx, cancel := context.WithTimeout(ctx, s.setupTimeout)
	defer cancel()
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return Generated{}, ErrBusy
	}
	defer func() { <-s.slots }()
	allowModel, err := s.modelBudgetLeft(ctx)
	if err != nil {
		return Generated{}, err
	}
	gen, err := s.gen.Setup(ctx, listingURL, anchorURL, allowModel)
	if err != nil {
		return Generated{}, fmt.Errorf("%w: %w", ErrGeneration, err)
	}
	if !gen.Health.valid() {
		return Generated{}, fmt.Errorf("%w: unknown health %q", ErrGeneration, gen.Health)
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
	items := make([]FeedItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, feedItem(r))
	}
	return Preview{Source: s.source(row), Items: items, ItemCount: int(count)}, nil
}

func (s *Service) source(row db.WebSource) Source {
	return Source{
		ID: row.ID, FeedURL: s.FeedURL(row.ID), ListingURL: row.ListingUrl, SiteURL: row.SiteUrl, Title: row.Title,
		Health: Health(row.Health), HealthReason: row.HealthReason, NeedsAnchor: row.NeedsAnchor != 0,
	}
}

// PollDue polls every followed web source whose next poll is due and reports how many it polled.
func (s *Service) PollDue(ctx context.Context) (int, error) {
	if s.gen == nil {
		return 0, ErrUnavailable
	}
	now := formatTime(s.now().UTC())
	due, err := s.read.ListDueWebSources(ctx, db.ListDueWebSourcesParams{Now: &now, FeedUrlPrefix: s.FeedURL(""), Limit: pollBatch})
	if err != nil {
		return 0, err
	}
	polled := 0
	for _, src := range due {
		if err := s.pollOne(ctx, src); err != nil {
			if ctx.Err() != nil {
				return polled, ctx.Err()
			}
			slog.Warn("web source poll", "id", src.ID, "err", err)
			continue
		}
		polled++
	}
	return polled, nil
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
			return ctx.Err()
		}
		slog.Warn("web source poll failed", "id", src.ID, "err", err)
		out = PollOutcome{Verdict: VerdictBroken, Reason: "the site could not be reached"}
	}

	update := db.UpdateWebSourcePollParams{
		ID: src.ID, SetupSeq: src.SetupSeq, Recipe: src.Recipe, LastPolledAt: ptr(formatTime(now)), UpdatedAt: formatTime(now),
		Etag: src.Etag, LastModified: src.LastModified, LinkSetHash: src.LinkSetHash,
	}
	if out.Checked {
		update.LastCheckedAt = ptr(formatTime(now))
	}
	if prev.WeeklyAlarm && out.Verdict == VerdictOK && !out.Checked && !answersAlarm(out.Repair) {
		out = PollOutcome{Verdict: VerdictSuspect, Reason: src.HealthReason, Repair: out.Repair}
	}
	publish := false
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
		update.NextPollAt = ptr(formatTime(now.Add(s.backoff(update.ConsecutiveFailures))))
	}

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
		if publish {
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
			if err := recordPoll(ctx, q, src.ID, now, items, out.Fingerprint); err != nil {
				return err
			}
		}
		return nil
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

func (s *Service) backoff(failures int64) time.Duration {
	d := s.interval
	for i := int64(0); i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

func (s *Service) pollState(ctx context.Context, src db.WebSource, now time.Time) (PollState, error) {
	rows, err := s.read.ListWebSourcePolls(ctx, db.ListWebSourcePollsParams{SourceID: src.ID, Limit: historyPolls})
	if err != nil {
		return PollState{}, err
	}
	state := PollState{ETag: deref(src.Etag), LastModified: deref(src.LastModified), LinkSetHash: deref(src.LinkSetHash)}
	for _, r := range rows {
		rec := PollRecord{Count: int(r.ItemCount), Fingerprint: Fingerprint{URLTemplate: r.UrlTemplate, DateFrac: r.DateFrac, TitleShape: r.TitleShape}}
		rec.PolledAt, _ = time.Parse(timeLayout, r.PolledAt)
		if err := json.Unmarshal([]byte(r.Urls), &rec.URLs); err != nil {
			return PollState{}, fmt.Errorf("poll %d urls: %w", r.ID, err)
		}
		if err := json.Unmarshal([]byte(r.Titles), &rec.Titles); err != nil {
			return PollState{}, fmt.Errorf("poll %d titles: %w", r.ID, err)
		}
		state.History = append(state.History, rec)
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
		urls[k.Url] = true
		byTitle[normTitle(k.Title)] = append(byTitle[normTitle(k.Title)], k.Url)
	}
	current := make(map[string]bool, len(items))
	for _, it := range items {
		current[it.URL] = true
	}
	var oldest time.Time
	switch raw, err := q.GetOldestWebSourceItemDate(ctx, sourceID); {
	case err == nil && raw != nil:
		oldest, _ = time.Parse(timeLayout, *raw)
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return nil, nil, err
	}
	for _, it := range items {
		switch {
		case urls[it.URL] || (!it.Date.IsZero() && !oldest.IsZero() && it.Date.Before(oldest)):
		case movedFrom(it.URL, byTitle[normTitle(it.Title)], current):
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
		if !current[old] && webfeed.URLTemplate(old) != shape {
			return true
		}
	}
	return false
}

func normTitle(t string) string { return strings.ToLower(strings.Join(strings.Fields(t), " ")) }

// dedupeItems is the gate every stored item passes: one per URL, only http(s) links, at most maxItems.
func dedupeItems(items []Item) []Item {
	seen := make(map[string]bool, len(items))
	out := make([]Item, 0, min(len(items), maxItems))
	for _, it := range items {
		if len(out) == maxItems {
			break
		}
		it.URL = strings.TrimSpace(it.URL)
		if seen[it.URL] || !isWebLink(it.URL) {
			continue
		}
		seen[it.URL] = true
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
func feedItem(r db.ListWebSourceFeedItemsRow) FeedItem {
	it := FeedItem{URL: r.Url, Title: r.Title, Excerpt: r.Excerpt}
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
	stripTracking(&c)
	key := strings.TrimPrefix(c.Host, "www.") + strings.TrimRight(c.EscapedPath(), "/")
	if c.RawQuery != "" {
		key += "?" + c.RawQuery
	}
	return key
}

// stripTracking drops query parameters that only say where a click came from, keeping the ones that pick content.
func stripTracking(u *url.URL) {
	if u.RawQuery == "" {
		return
	}
	q := u.Query()
	for k := range q {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "utm_") || lk == "ref" || lk == "fbclid" || lk == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
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

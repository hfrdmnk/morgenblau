package webfeed

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"sort"
	"strings"

	"morgenblau/internal/postkey"
)

// Options configures Setup.
type Options struct {
	// Client fetches every page; it must be SSRF-safe for attacker-chosen URLs.
	Client Doer
	// Model judges unsure setups and writes a CSS recipe when fewer than 3 items survive; nil keeps the heuristic's
	// pick, records that the judge was wanted and skips induction.
	Model Model
	// MaxSetupBytes caps the bytes one setup or poll reads across all its fetches.
	MaxSetupBytes int64
	// Robots lets polls reuse each site's robots.txt for a day; nil fetches it on every poll.
	Robots *RobotsCache
}

// Result is a finished setup: the recipe to store, the items it yields now, and the self-check verdict.
type Result struct {
	Recipe Recipe
	Title  string
	Items  []Item
	// Excerpts are the items' teasers on the listing, keyed by item URL.
	Excerpts map[string]string
	// Blocked says the site refused us: robots.txt, or a refusal with no sitemap to fall back on.
	Blocked bool
	// Reasons say why the site refused us or why the self-check doubts the result; none means it is healthy.
	Reasons []Reason
	// Candidates are the top lists the heuristic found, in the order the judge saw them.
	Candidates []Candidate

	listing *pageDoc
}

const (
	judgeBelowConfidence = 0.6
	judgeBelowMargin     = 2.0
	verifyItems          = 3
	calibrateItems       = 20
	calibrateUndated     = 30
	dateOrderShare       = 0.7
)

// Setup finds the post stream on a listing page that has no feed and builds its recipe.
func Setup(ctx context.Context, listingURL string, opts Options) (*Result, error) {
	res, err := setup(ctx, listingURL, "", opts)
	if err != nil {
		return nil, err
	}
	if opts.Model != nil && res.listing != nil && len(res.Items) < induceBelowItems {
		if err := induce(ctx, res, res.listing, opts.Model); err != nil {
			return nil, err
		}
	}
	res.release()
	return res, nil
}

// Healthy is a result the self-check does not doubt.
func (r *Result) Healthy() bool { return len(r.Reasons) == 0 }

// release takes the teasers from the parsed listing and drops it, the biggest thing a setup holds.
func (r *Result) release() {
	if r.listing != nil {
		r.Excerpts = listingExcerpts(r.listing, r.Items)
	}
	r.listing = nil
}

// SetupAnchored re-runs setup choosing the lists that contain the reader's example post or share its URL shape.
func SetupAnchored(ctx context.Context, listingURL, anchorPostURL string, opts Options) (*Result, error) {
	if URLTemplate(anchorPostURL) == "" {
		return nil, fmt.Errorf("webfeed: anchor %q is not an http(s) URL", anchorPostURL)
	}
	res, err := setup(ctx, listingURL, anchorPostURL, opts)
	if res != nil {
		res.release()
	}
	return res, err
}

func setup(ctx context.Context, listingURL, anchorURL string, opts Options) (*Result, error) {
	if opts.Client == nil {
		return nil, errors.New("webfeed: Options.Client is required")
	}
	u, err := url.Parse(strings.TrimSpace(listingURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("webfeed: %q is not an http(s) URL", listingURL)
	}
	u.Fragment = ""
	s := newSession(opts)
	res := &Result{Recipe: newRecipe(u.String(), ModeStreams)}
	rec := &res.Recipe

	rb := s.robotsFor(ctx, u.Scheme+"://"+u.Host)
	if !rb.allowed(u.RequestURI()) {
		rec.Setup.Stages = append(rec.Setup.Stages, "robots")
		res.Blocked, res.Reasons = true, []Reason{ReasonRobots}
		return res, nil
	}
	resp := s.get(ctx, u.String())
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, blocked := resp.refusal(); blocked {
		return s.sitemapSetup(ctx, res, u, rb)
	}
	p, err := listingPage(resp)
	if err != nil {
		return nil, err
	}
	res.listing = p
	s.setLang(p.lang)
	rec.Lang = p.lang
	res.Title = p.title
	rec.Setup.Stages = append(rec.Setup.Stages, "heuristic")

	all := p.candidates()
	top := all[:min(len(all), maxCandidates)]
	for _, c := range top {
		res.Candidates = append(res.Candidates, c.public())
	}
	conf, margin := confidence(top)
	rec.Setup.Confidence, rec.Setup.Margin = round2(conf), round2(margin)
	if len(top) == 0 && anchorURL == "" {
		res.Reasons = []Reason{ReasonCount}
		return res, nil
	}

	pool, pick, judgeNone := top, []int(nil), false
	if anchorURL != "" {
		pool, pick = p.anchoredPick(all, anchorURL)
		rec.Setup.Stages = append(rec.Setup.Stages, "anchored")
		rec.Setup.Anchor = anchorURL
	} else if pick, judgeNone, err = s.choose(ctx, res, top, conf, margin, opts.Model); err != nil {
		return nil, err
	}
	if len(pick) == 0 {
		res.Reasons = []Reason{ReasonCount}
		return res, nil
	}
	picked := make([]*cluster, len(pick))
	for i, k := range pick {
		picked[i] = pool[k]
	}
	s.build(ctx, res, p, picked)
	// Without a judge, an unsure pick is the reader's call: serving it as healthy would be a silent failure.
	if anchorURL == "" && opts.Model == nil && conf < judgeBelowConfidence {
		res.Reasons = append(res.Reasons, ReasonUnsure)
	}
	if judgeNone {
		res.Reasons = append(res.Reasons, ReasonJudgeNone)
	}
	return res, nil
}

// listingPage parses a listing the site did not refuse, or says why there is nothing to parse.
func listingPage(resp *response) (*pageDoc, error) {
	switch reason, _ := resp.refusal(); {
	case resp.err != nil:
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, resp.err)
	case resp.unreachable():
		return nil, fmt.Errorf("%w: listing returned HTTP %d", ErrUnreachable, resp.status)
	case reason != "":
		return nil, fmt.Errorf("webfeed: listing returned HTTP %d", resp.status)
	case !resp.isHTML():
		return nil, fmt.Errorf("webfeed: listing is not HTML (%s)", resp.header.Get("Content-Type"))
	}
	p, err := parsePage(resp.body, resp.url)
	if err != nil {
		return nil, fmt.Errorf("webfeed: parse listing: %w", err)
	}
	return p, nil
}

// judgeReasons says why the heuristic is unsure of its pick; none means it decides alone.
func judgeReasons(conf, margin float64, verifyFlagged bool) []string {
	var reasons []string
	if conf < judgeBelowConfidence {
		reasons = append(reasons, "confidence")
	}
	if margin < judgeBelowMargin {
		reasons = append(reasons, "margin")
	}
	if verifyFlagged {
		reasons = append(reasons, "verify")
	}
	return reasons
}

// choose picks among the top candidates: the heuristic's pick, or the judge's when the heuristic is unsure and there is
// a model. none reports a judge majority that no list is a post stream; the heuristic's pick then stands.
func (s *session) choose(ctx context.Context, res *Result, top []*cluster, conf, margin float64, m Model) (pick []int, none bool, err error) {
	rec := &res.Recipe.Setup
	flagged := s.verify(ctx, top[0])
	rec.Stages = append(rec.Stages, "verify")
	pick = heuristicPick(top, res.Recipe.ListingURL)
	reasons := judgeReasons(conf, margin, flagged)
	if len(reasons) == 0 {
		return pick, false, nil
	}
	rec.JudgeWanted, rec.JudgeReasons = true, reasons
	if m == nil {
		return pick, false, nil
	}
	jr, err := runJudge(ctx, m, renderUserMessage(res.Recipe.ListingURL, res.Title, res.Candidates), len(top), orderFor(top[0].dateFrac))
	if err != nil {
		return nil, false, err
	}
	rec.Stages = append(rec.Stages, "judge")
	rec.Judge = &jr
	if jr.None {
		return pick, true, nil
	}
	return sectionGuard(top, jr.Pick, res.Recipe.ListingURL), false, nil
}

// build turns the picked lists into the recipe's streams and the setup's items, checked against the posts' own pages,
// and runs the self-check on them.
func (s *session) build(ctx context.Context, res *Result, p *pageDoc, picked []*cluster) {
	rec := &res.Recipe
	streams := buildStreams(picked, rec.ListingURL)
	entries := p.mergeEntries(picked, streamTemplates(streams))
	excludes, hints, dateSource := s.calibrate(ctx, entries, picked)
	for i := range streams {
		if h, ok := hints[streams[i].URLTemplate]; ok {
			streams[i].TitleHints = h
		}
	}
	entries = slices.DeleteFunc(entries, func(e *entry) bool { return slices.Contains(excludes, e.URL) })

	items := make([]Item, len(entries))
	for i, e := range entries {
		items[i] = e.item()
	}
	sig := ComputeSignals(items, streamTemplates(streams))
	rec.Order = orderFor(sig.DateFrac)
	if rec.Setup.Judge != nil && rec.Setup.Judge.Order == OrderPage {
		rec.Order = OrderPage
	}
	sortItems(items, rec.Order)
	if len(items) > MaxItems {
		items = capItems(items)
		sig = ComputeSignals(items, streamTemplates(streams))
	}

	rec.Streams = streams
	rec.ExcludeURLs = append(rec.ExcludeURLs, excludes...)
	rec.DateSource = dateSource
	rec.Fingerprint = fingerprintOf(sig, streamTemplates(streams))
	res.Items = items
	res.Reasons = sig.doubts()
}

func orderFor(dateFrac float64) Order {
	if dateFrac >= dateOrderShare {
		return OrderDate
	}
	return OrderPage
}

// sortItems orders newest first for date order, undated items last; page order is left alone.
func sortItems(items []Item, order Order) {
	if order != OrderDate {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].Date, items[j].Date
		if a.IsZero() != b.IsZero() {
			return !a.IsZero()
		}
		return a.After(b)
	})
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// verify fetches the top candidate's first items; it flags the stream for the judge, never rejects it.
func (s *session) verify(ctx context.Context, top *cluster) bool {
	var urls []string
	for _, e := range top.entries[:min(len(top.entries), verifyItems)] {
		urls = append(urls, e.URL)
	}
	infos := s.fetchArticles(ctx, urls)
	fetched, hubs, mismatched := 0, 0, 0
	for _, e := range top.entries[:len(urls)] {
		info := infos[e.URL]
		if info == nil || !info.ok {
			continue
		}
		fetched++
		if info.isHub(top.template) {
			hubs++
		}
		if info.clean != "" && titleSimilarity(e.Title, info.clean) < 0.3 && !strings.Contains(strings.ToLower(e.text), strings.ToLower(info.clean)) {
			mismatched++
		}
	}
	return fetched >= 2 && (hubs >= 2 || mismatched >= 2)
}

// buildStreams turns picked clusters into streams: each list's own shape, plus post-like minority shapes inside it
// (another post type, a series), generalized per depth. A shape an earlier stream's pattern covers folds into it.
func buildStreams(picked []*cluster, listingURL string) []Stream {
	var out []Stream
	add := func(tmpl string, c *cluster) {
		i := -1
		for k, st := range out {
			if matchTemplate(st.URLTemplate, tmpl) {
				i = k
				break
			}
		}
		if i < 0 {
			i = len(out)
			out = append(out, Stream{URLTemplate: tmpl, TitleHints: titleHintFor(c)})
		}
		if !slices.Contains(out[i].ContainerHints, c.hint) {
			out[i].ContainerHints = append(out[i].ContainerHints, c.hint)
		}
	}
	for _, c := range picked {
		add(c.template, c)
		groups := map[string][]*entry{}
		var keys []string
		for _, e := range c.entries {
			t := URLTemplate(e.URL)
			if matchTemplate(c.template, t) {
				continue
			}
			k := depthKey(t)
			if groups[k] == nil {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], e)
		}
		for _, k := range keys {
			if tmpl := minorityShape(groups[k]); tmpl != "" && inSection(tmpl, c.template, listingURL) {
				add(tmpl, c)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Stream) int { return strings.Compare(a.URLTemplate, b.URLTemplate) })
	return out
}

// minorityShape is the shape of same-depth leftovers in a picked list when at least two of them read as posts.
func minorityShape(es []*entry) string {
	if len(es) < 2 {
		return ""
	}
	postLike := 0
	var ts []string
	for _, e := range es {
		if goodTitleShape(e.Title) && (postLikePath(e.URL) || !e.Date.IsZero() || e.TitleSource == TitleHeading) {
			postLike++
		}
		ts = append(ts, URLTemplate(e.URL))
	}
	if postLike*2 < len(es) || postLike < 2 {
		return ""
	}
	if distinct := slices.Compact(slices.Sorted(slices.Values(ts))); len(distinct) == 1 {
		return distinct[0]
	}
	return generalizeTemplates(ts)
}

// titleHintFor records the most common place titles came from.
func titleHintFor(c *cluster) TitleHints {
	counts := map[TitleHints]int{}
	var best TitleHints
	for _, e := range c.entries {
		h := TitleHints{Source: e.TitleSource, Tag: e.titleTag}
		counts[h]++
		if counts[h] > counts[best] {
			best = h
		}
	}
	return best
}

func streamTemplates(streams []Stream) []string {
	out := make([]string, 0, len(streams))
	for _, s := range streams {
		out = append(out, s.URLTemplate)
	}
	return out
}

// mergeEntries unions the picked lists in page order, keeping one entry per URL and only the streams' URL shapes.
func (p *pageDoc) mergeEntries(picked []*cluster, templates []string) []*entry {
	var all []*entry
	seen := map[string]bool{}
	for _, c := range picked {
		for _, e := range c.entries {
			k := postkey.URL(e.URL)
			if seen[k] || !matchAny(templates, URLTemplate(e.URL)) {
				continue
			}
			seen[k] = true
			all = append(all, e)
		}
	}
	slices.SortStableFunc(all, func(a, b *entry) int { return p.order[a.node] - p.order[b.node] })
	return all
}

// calibrate fetches each emitted item once: article titles replace weak listing titles, article dates fill gaps, hub pages are excluded.
func (s *session) calibrate(ctx context.Context, entries []*entry, picked []*cluster) (excludes []string, hints map[string]TitleHints, source DateSource) {
	dated := 0
	for _, e := range entries {
		if !e.Date.IsZero() {
			dated++
		}
	}
	var urls []string
	for _, e := range entries[:min(len(entries), calibrateLimit(len(entries), dated))] {
		urls = append(urls, e.URL)
	}
	infos := s.fetchArticles(ctx, urls)
	trustDates := trustArticleDates(infos)

	hintVotes := map[string]map[string]int{}
	filled := 0
	for _, e := range entries {
		info := infos[e.URL]
		if info == nil || !info.ok {
			continue
		}
		tmpl := streamFor(picked, e.URL)
		if info.isHub(tmpl) {
			excludes = append(excludes, e.URL)
			continue
		}
		if e.Date.IsZero() && trustDates && !info.published.IsZero() {
			e.Date = info.published
			filled++
		}
		if t := info.clean; t != "" && betterTitle(e, t) {
			if n := segmentMatching(e, t); n != nil {
				if hintVotes[tmpl] == nil {
					hintVotes[tmpl] = map[string]int{}
				}
				hintVotes[tmpl][nodeSig(n, true)]++
			}
			e.Title, e.TitleSource = t, TitleArticle
		}
	}
	return excludes, titleHintsFrom(hintVotes), dateSourceOf(len(entries)-len(excludes), dated, filled)
}

// calibrateLimit is how many posts calibration fetches: more when most lack a listing date, since their pages must supply it.
func calibrateLimit(n, dated int) int {
	if n > 0 && float64(dated)/float64(n) < 0.5 {
		return calibrateUndated
	}
	return calibrateItems
}

// titleHintsFrom keeps, per stream, the element that held the article's title for at least two posts and half the votes.
func titleHintsFrom(votes map[string]map[string]int) map[string]TitleHints {
	hints := map[string]TitleHints{}
	for tmpl, byTag := range votes {
		best, bestN, total := "", 0, 0
		for sig, n := range byTag {
			total += n
			if n > bestN || (n == bestN && sig < best) {
				best, bestN = sig, n
			}
		}
		if bestN*2 >= total && bestN >= 2 {
			hints[tmpl] = TitleHints{Source: TitleCalibrated, Tag: best}
		}
	}
	return hints
}

// dateSourceOf says where n kept posts got their dates: dated from the listing, filled from their own pages.
func dateSourceOf(n, dated, filled int) DateSource {
	switch {
	case n <= 0:
		return DateSourceNone
	case float64(dated)/float64(n) >= 0.5:
		return DateSourceListing
	case filled > 0 && float64(dated+filled)/float64(n) >= 0.5:
		return DateSourceArticle
	case dated > 0:
		return DateSourceListing
	}
	return DateSourceNone
}

// trustArticleDates is false for a site that stamps every page with one date (a build date), which says nothing about its posts.
func trustArticleDates(infos map[string]*articleInfo) bool {
	days := map[string]bool{}
	dated := 0
	for _, info := range infos {
		if !info.published.IsZero() {
			dated++
			days[info.published.Format("2006-01-02")] = true
		}
	}
	return dated < 3 || len(days) != 1
}

// betterTitle says whether the article's own title should replace the listing's: when the listing gave a slug, label or subtitle.
func betterTitle(e *entry, article string) bool {
	if strings.EqualFold(e.Title, article) || (goodTitleShape(e.Title) && !goodTitleShape(article)) {
		return false
	}
	if e.TitleSource == TitleSlug || e.TitleSource == TitleAttr {
		return true
	}
	if !goodTitleShape(e.Title) && goodTitleShape(article) {
		return true
	}
	// A listing title that wraps the article's title in a date or label prefix is the label, not a rewrite.
	if l, a := strings.ToLower(e.Title), strings.ToLower(collapse(article)); strings.Contains(l, a) && len(l) > len(a)*6/5 {
		return true
	}
	if titleSimilarity(e.Title, article) >= 0.8 {
		return false
	}
	return strings.Contains(strings.ToLower(e.text), strings.ToLower(collapse(article)))
}

// anchoredPick chooses the lists holding the reader's example post, else lists with its URL shape, else every link of that shape.
func (p *pageDoc) anchoredPick(all []*cluster, anchorURL string) ([]*cluster, []int) {
	key, tmpl := postkey.URL(anchorURL), URLTemplate(anchorURL)
	var pick []int
	for i, c := range all {
		for _, e := range c.entries {
			if postkey.URL(e.URL) == key {
				pick = append(pick, i)
				break
			}
		}
	}
	if len(pick) == 0 {
		for i, c := range all {
			if matchTemplate(c.template, tmpl) && c.navShare < 0.5 {
				pick = append(pick, i)
			}
		}
	}
	if len(pick) > 0 {
		// Every same-shape list joins, since the reader vouched for the shape.
		for i, c := range all {
			if c.template == all[pick[0]].template && c.navShare < 0.5 && !slices.Contains(pick, i) {
				pick = append(pick, i)
			}
		}
		return all, pick
	}
	var as []*anchor
	for _, a := range p.anchors {
		if matchTemplate(tmpl, a.tmpl) {
			as = append(as, a)
		}
	}
	if len(as) == 0 {
		return all, nil
	}
	c := &cluster{anchors: as, container: p.groupContainer(as), template: tmpl}
	p.buildEntries(c)
	c.hint = containerHint(c.container)
	p.score(c)
	return append(all, c), []int{len(all)}
}

// streamFor is the URL shape of the picked list that holds an item, its own template when none matches.
func streamFor(picked []*cluster, rawURL string) string {
	t := URLTemplate(rawURL)
	for _, c := range picked {
		if matchTemplate(c.template, t) {
			return c.template
		}
	}
	return t
}

// inSection is sectionGuard for minority shapes: on the site itself they must share the listing's or the list's first
// segment; shapes on other hosts pass, since a list that links out there does so on purpose.
func inSection(tmpl, primary, listingURL string) bool {
	host, _, _ := strings.Cut(tmpl, "/")
	primaryHost, _, _ := strings.Cut(primary, "/")
	if !sameSite(host, primaryHost) {
		return true
	}
	seg := templateFirstSegment(tmpl)
	return seg == pathFirstSegment(listingURL) || seg == templateFirstSegment(primary)
}

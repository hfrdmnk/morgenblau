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

type Options struct {
	Client        Doer
	Model         Model
	MaxSetupBytes int64
	Robots        *RobotsCache
}

type Result struct {
	Recipe       Recipe
	Title        string
	Items        []Item
	Excerpts     map[string]string
	Blocked      bool
	Reasons      []Reason
	Candidates   []Candidate
	AnchorOnPage bool

	listing *pageDoc
	digest  *pageDigest
}

const (
	judgeBelowConfidence = 0.6
	judgeBelowMargin     = 2.0
	verifyItems          = 3
	calibrateItems       = 20
	calibrateUndated     = 30
	dateOrderShare       = 0.7
)

func Setup(ctx context.Context, listingURL string, opts Options) (*Result, error) {
	return setupListing(ctx, listingURL, opts, true)
}

func setupListing(ctx context.Context, listingURL string, opts Options, toSection bool) (*Result, error) {
	if opts.Client == nil {
		return nil, errClient
	}
	s := newSession(opts)
	res, err := s.setup(ctx, listingURL, "", opts.Model)
	if err != nil {
		return res, err
	}
	if toSection {
		if res, err = s.preferSection(ctx, res); err != nil {
			res.release()
			return res, err
		}
	}
	if opts.Model != nil && res.listing != nil && len(res.Items) < induceBelowItems && !res.BrowserBuilt() {
		if err := induce(ctx, res, res.listing, opts.Model); err != nil {
			res.release()
			return res, err
		}
	}
	res.release()
	res.Recipe.Setup.SectionChecked = toSection
	return res, nil
}

func (r *Result) BrowserBuilt() bool { return slices.Contains(r.Reasons, ReasonSPAShell) }

func (r *Result) Healthy() bool { return len(r.Reasons) == 0 }

func (r *Result) release() {
	if r.listing != nil {
		r.Excerpts = listingExcerpts(r.listing, r.Items)
	}
	r.listing = nil
}

func SetupAnchored(ctx context.Context, listingURL, anchorPostURL string, opts Options) (*Result, error) {
	if URLTemplate(anchorPostURL) == "" {
		return nil, fmt.Errorf("webfeed: anchor %q is not an http(s) URL", anchorPostURL)
	}
	if opts.Client == nil {
		return nil, errClient
	}
	res, err := newSession(opts).setup(ctx, listingURL, anchorPostURL, opts.Model)
	if res != nil {
		if res.listing != nil {
			d := res.listing.digest()
			res.digest = &d
		}
		res.release()
	}
	return res, err
}

var errClient = errors.New("webfeed: Options.Client is required")

func (s *session) setup(ctx context.Context, listingURL, anchorURL string, model Model) (*Result, error) {
	u, err := url.Parse(strings.TrimSpace(listingURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("webfeed: %q is not an http(s) URL", listingURL)
	}
	u.Fragment = ""
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
	if anchorURL == "" && len(top) == 0 && emptyTier0(p, resp.body) == ReasonSPAShell {
		res.Reasons = []Reason{ReasonSPAShell}
		return res, nil
	}
	if len(top) == 0 && anchorURL == "" {
		res.Reasons = []Reason{ReasonCount}
		return res, nil
	}

	pool, pick, judgeNone := top, []int(nil), false
	if anchorURL != "" {
		res.AnchorOnPage = p.links(anchorURL)
		pool, pick = p.anchoredPick(all, anchorURL)
		rec.Setup.Stages = append(rec.Setup.Stages, "anchored")
		rec.Setup.Anchor = anchorURL
	} else if pick, judgeNone, err = s.choose(ctx, res, top, conf, margin, model); err != nil {
		res.release()
		return res, err
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
	if anchorURL == "" && model == nil && conf < judgeBelowConfidence {
		res.Reasons = append(res.Reasons, ReasonUnsure)
	}
	if judgeNone {
		res.Reasons = append(res.Reasons, ReasonJudgeNone)
	}
	return res, nil
}

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
		rec.Judge = &jr
		return nil, false, err
	}
	rec.Stages = append(rec.Stages, "judge")
	rec.Judge = &jr
	if jr.None {
		return pick, true, nil
	}
	return sectionGuard(top, jr.Pick, res.Recipe.ListingURL), false, nil
}

func (s *session) build(ctx context.Context, res *Result, p *pageDoc, picked []*cluster) {
	rec := &res.Recipe
	streams := buildStreams(picked, rec.ListingURL)
	entries := p.mergeEntries(picked, streamTemplates(streams))
	excludes, hints, dateSource := s.calibrate(ctx, entries, picked, streamTemplates(streams))
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
		if info.ownTitle != "" && titleSimilarity(e.Title, info.ownTitle) < 0.3 && !strings.Contains(strings.ToLower(e.text), strings.ToLower(info.ownTitle)) {
			mismatched++
		}
	}
	return fetched >= 2 && (hubs >= 2 || mismatched >= 2)
}

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

func minorityShape(es []*entry) string {
	if len(es) < 2 {
		return ""
	}
	postLike := 0
	var ts []string
	for _, e := range es {
		u, err := url.Parse(e.URL)
		if err == nil && !isPagination(u, e.Title) && goodTitleShape(e.Title) && (postLikePath(e.URL) || !e.Date.IsZero() || e.TitleSource == TitleHeading) {
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

func (s *session) calibrate(ctx context.Context, entries []*entry, picked []*cluster, streams []string) (excludes []string, hints map[string]TitleHints, source DateSource) {
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
		if t := info.ownTitle; t != "" && betterTitle(e, t) {
			if n := segmentMatching(e, t); n != nil {
				st := streamOf(streams, e.URL)
				if hintVotes[st] == nil {
					hintVotes[st] = map[string]int{}
				}
				hintVotes[st][nodeSig(n, true)]++
			}
			e.Title, e.TitleSource = t, TitleArticle
		}
	}
	return excludes, titleHintsFrom(hintVotes), dateSourceOf(len(entries)-len(excludes), dated, filled)
}

func calibrateLimit(n, dated int) int {
	if n > 0 && float64(dated)/float64(n) < 0.5 {
		return calibrateUndated
	}
	return calibrateItems
}

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
	if l, a := strings.ToLower(e.Title), strings.ToLower(collapse(article)); strings.Contains(l, a) && len(l) > len(a)*6/5 {
		return true
	}
	if titleSimilarity(e.Title, article) >= 0.8 {
		return false
	}
	return strings.Contains(strings.ToLower(e.text), strings.ToLower(collapse(article)))
}

func (p *pageDoc) anchoredPick(all []*cluster, anchorURL string) ([]*cluster, []int) {
	key, tmpl := postkey.URL(anchorURL), URLTemplate(anchorURL)
	var pick []int
	for i, c := range all {
		if c.navShare < 0.5 && slices.ContainsFunc(c.entries, func(e *entry) bool { return postkey.URL(e.URL) == key }) {
			pick = append(pick, i)
		}
	}
	if len(pick) == 0 {
		for i, c := range all {
			if matchTemplate(c.template, tmpl) && c.navShare < 0.5 {
				pick = append(pick, i)
			}
		}
	}
	if main := slices.DeleteFunc(slices.Clone(pick), func(i int) bool { return all[i].sideList() }); len(main) > 0 {
		pick = main
	}
	if len(pick) > 0 {
		for i, c := range all {
			if c.template == all[pick[0]].template && !c.sideList() && !slices.Contains(pick, i) {
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

func (p *pageDoc) links(rawURL string) bool {
	abs, ok := resolveLink(p.url, rawURL)
	if !ok {
		return false
	}
	key := postkey.URL(abs)
	return slices.ContainsFunc(p.anchors, func(a *anchor) bool { return a.key == key })
}

func streamOf(streams []string, rawURL string) string {
	t := URLTemplate(rawURL)
	if i := slices.IndexFunc(streams, func(p string) bool { return matchTemplate(p, t) }); i >= 0 {
		return streams[i]
	}
	return t
}

func streamFor(picked []*cluster, rawURL string) string {
	t := URLTemplate(rawURL)
	for _, c := range picked {
		if matchTemplate(c.template, t) {
			return c.template
		}
	}
	return t
}

func inSection(tmpl, primary, listingURL string) bool {
	host, _, _ := strings.Cut(tmpl, "/")
	primaryHost, _, _ := strings.Cut(primary, "/")
	if !sameSite(host, primaryHost) {
		return true
	}
	seg := templateFirstSegment(tmpl)
	return seg == pathFirstSegment(listingURL) || seg == templateFirstSegment(primary)
}

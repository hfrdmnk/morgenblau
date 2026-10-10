package webfeed

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// RepairStep names a rung of the repair ladder, cheapest first.
type RepairStep string

const (
	RepairKnownURLs RepairStep = "known_urls"
	RepairTemplate  RepairStep = "template"
	RepairHeuristic RepairStep = "heuristic"
	RepairJudge     RepairStep = "judge"
)

// Acceptance rules a repair can pass.
const (
	AcceptKnownURL = "known_url"
	AcceptTemplate = "template"
	AcceptTitles   = "titles"
	AcceptModel    = "model"
)

const (
	acceptMinItems   = 3
	acceptTitleShare = 0.5
)

// RepairInput is what the ladder anchors on and whether its model step may run.
type RepairInput struct {
	Recipe  Recipe
	History []GoodPoll
	// AllowModel lets the judge step run; the caller enforces its once-a-day budget.
	AllowModel bool
}

// RepairResult reports the ladder: the accepted recipe and its items, or the steps that were tried and rejected.
type RepairResult struct {
	Accepted bool
	Step     RepairStep
	Rule     string
	// FalseAlarm marks an accepted repair that reproduces the last good poll's posts exactly.
	FalseAlarm  bool
	UsedModel   bool
	Tried       []RepairStep
	Recipe      Recipe
	Items       []Item
	Excerpts    map[string]string
	URLTemplate string
	ModelCalls  int
	ModelUSD    float64
}

// Repair re-finds the post stream on the page a suspect poll fetched, escalating until a result passes the acceptance rule.
func Repair(ctx context.Context, polled *PollResult, in RepairInput, opts Options) (*RepairResult, error) {
	if polled == nil || polled.page == nil {
		return nil, errors.New("webfeed: repair needs the page a poll fetched")
	}
	out := &RepairResult{}
	if len(in.History) == 0 {
		return out, nil
	}
	pg := polled.page
	if in.Recipe.Mode != ModeSitemap {
		for _, step := range []RepairStep{RepairKnownURLs, RepairTemplate, RepairHeuristic} {
			rec, ok := reinfer(step, pg, in)
			if !ok {
				continue
			}
			items, err := Apply(rec, pg.body, pg.url)
			if err != nil {
				return nil, err
			}
			out.Tried = append(out.Tried, step)
			if out.accept(step, rec, items, in.History) {
				out.Excerpts = ListingExcerpts(pg.body, pg.url, items)
				return out, nil
			}
		}
	}
	if !in.AllowModel || (opts.Judge == nil && opts.Inducer == nil) {
		return out, nil
	}
	res, err := Setup(ctx, in.Recipe.ListingURL, opts)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return out, nil
	}
	out.Tried = append(out.Tried, RepairJudge)
	out.UsedModel = true
	out.ModelCalls, out.ModelUSD = res.Recipe.Setup.ModelCost()
	if res.Health != HealthBlockedBySite && out.accept(RepairJudge, res.Recipe, res.Items, in.History) {
		out.Excerpts = ListingExcerpts(pg.body, pg.url, res.Items)
	}
	return out, nil
}

func (out *RepairResult) accept(step RepairStep, rec Recipe, items []Item, history []GoodPoll) bool {
	templates := streamTemplates(rec.Streams)
	dominant := DominantTemplate(items, templates)
	rule := acceptRepair(items, dominant, history, step == RepairJudge)
	if rule == "" {
		return false
	}
	sig := ComputeSignals(items, templates)
	rec.Fingerprint = Fingerprint{Count: sig.Count, URLTemplates: templates, DateFrac: round2(sig.DateFrac), TitleShape: round2(sig.TitleShape)}
	out.Accepted, out.Step, out.Rule = true, step, rule
	out.Recipe, out.Items, out.URLTemplate = rec, items, dominant
	out.FalseAlarm = slices.Equal(hashInput(itemURLKeys(items)), hashInput(urlKeys(history[0].URLs)))
	return true
}

// acceptRepair names the first acceptance rule a repair passes, or "" to escalate. Titles carry a site through a URL
// migration, where neither known URLs nor the old template can.
func acceptRepair(items []Item, dominant string, history []GoodPoll, modelStep bool) string {
	if len(items) == 0 || len(history) == 0 {
		return ""
	}
	last := history[0]
	known := map[string]bool{}
	for _, u := range last.URLs {
		known[urlKey(u)] = true
	}
	for _, it := range items {
		if known[urlKey(it.URL)] {
			return AcceptKnownURL
		}
	}
	if len(items) >= acceptMinItems && last.URLTemplate != "" && dominant == last.URLTemplate {
		return AcceptTemplate
	}
	current := map[string]bool{}
	for _, it := range items {
		current[normTitle(it.Title)] = true
	}
	reappear := 0
	for _, t := range last.Titles {
		if current[normTitle(t)] {
			reappear++
		}
	}
	if len(last.Titles) > 0 && float64(reappear) >= acceptTitleShare*float64(len(last.Titles)) {
		return AcceptTitles
	}
	if modelStep && len(items) >= acceptMinItems && reappear > 0 {
		return AcceptModel
	}
	return ""
}

func normTitle(t string) string { return strings.ToLower(collapse(t)) }

func urlKeys(urls []string) []string {
	keys := make([]string, len(urls))
	for i, u := range urls {
		keys[i] = urlKey(u)
	}
	return keys
}

func hashInput(keys []string) []string { return slices.Compact(slices.Sorted(slices.Values(keys))) }

// reinfer re-finds the stream on the polled page without a model or another fetch.
func reinfer(step RepairStep, pg *fetchedPage, in RepairInput) (Recipe, bool) {
	p, err := parsePage(pg.body, pg.url)
	if err != nil {
		return Recipe{}, false
	}
	all := p.candidates()
	last := in.History[0]
	var picked []*cluster
	switch step {
	case RepairKnownURLs:
		known := map[string]bool{}
		for _, u := range last.URLs {
			known[urlKey(u)] = true
		}
		for _, c := range all {
			if !c.sideList() && slices.ContainsFunc(c.entries, func(e *entry) bool { return known[urlKey(e.URL)] }) {
				picked = append(picked, c)
			}
		}
	case RepairTemplate:
		if last.URLTemplate == "" {
			return Recipe{}, false
		}
		for _, c := range all {
			if !c.sideList() && matchTemplate(last.URLTemplate, c.template) {
				picked = append(picked, c)
			}
		}
	case RepairHeuristic:
		top := all[:min(len(all), maxCandidates)]
		for _, i := range heuristicPick(top, in.Recipe.ListingURL) {
			picked = append(picked, top[i])
		}
	}
	if len(picked) == 0 {
		return Recipe{}, false
	}
	return rebuilt(in.Recipe, picked, step), true
}

// rebuilt replaces the recipe's streams with the picked lists, keeping its listing, order and calibrated title hints.
func rebuilt(old Recipe, picked []*cluster, step RepairStep) Recipe {
	rec := old
	rec.Mode, rec.CSS = ModeStreams, nil
	rec.Streams = buildStreams(picked, old.ListingURL)
	for i := range rec.Streams {
		for _, o := range old.Streams {
			if o.URLTemplate == rec.Streams[i].URLTemplate && o.TitleHints.Source == "calibrated" {
				rec.Streams[i].TitleHints = o.TitleHints
			}
		}
	}
	rec.ExcludeURLs = slices.Clone(old.ExcludeURLs)
	if rec.ExcludeURLs == nil {
		rec.ExcludeURLs = []string{}
	}
	rec.Setup.Stages = append(slices.Clone(old.Setup.Stages), "repair:"+string(step))
	return rec
}

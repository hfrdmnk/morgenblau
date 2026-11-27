package webfeed

import (
	"context"
	"errors"
	"slices"

	"morgenblau/internal/postkey"
)

type RepairStep string

const (
	RepairKnownURLs RepairStep = "known_urls"
	RepairTemplate  RepairStep = "template"
	RepairHeuristic RepairStep = "heuristic"
	RepairJudge     RepairStep = "judge"
)

const (
	AcceptKnownURL  = "known_url"
	AcceptTemplate  = "template"
	AcceptTitles    = "titles"
	AcceptModel     = "model"
	AcceptDifferent = "different_list"
)

const (
	acceptMinItems   = 3
	acceptTitleShare = 0.5
)

type RepairInput struct {
	Recipe     Recipe
	History    []GoodPoll
	AllowModel bool
	Doubted    bool
}

type RepairResult struct {
	Accepted   bool
	Step       RepairStep
	Rule       string
	FalseAlarm bool
	UsedModel  bool
	Tried      []RepairStep
	Recipe     Recipe
	Items      []Item
	Excerpts   map[string]string
	ModelUSD   float64
}

func Repair(ctx context.Context, polled *PollResult, in RepairInput, opts Options) (*RepairResult, error) {
	if polled == nil || polled.page == nil {
		return nil, errors.New("webfeed: repair needs the page a poll fetched")
	}
	out := &RepairResult{}
	if len(in.History) == 0 {
		return out, nil
	}
	page := polled.page.doc
	accept := func(step RepairStep, rec Recipe, items []Item, modelPicked bool) bool {
		return out.accept(step, rec, items, in.History, modelPicked)
	}
	steps := []RepairStep{RepairKnownURLs, RepairTemplate, RepairHeuristic}
	if in.Doubted {
		steps = []RepairStep{RepairHeuristic}
		doubted := keySet(itemURLKeys(polled.Items))
		accept = func(step RepairStep, rec Recipe, items []Item, _ bool) bool {
			return out.acceptOther(step, rec, items, doubted)
		}
	}
	if page != nil {
		for _, step := range steps {
			rec, ok := reinfer(step, page, in)
			if !ok {
				continue
			}
			items, err := applyPage(rec, page)
			if err != nil {
				return nil, err
			}
			out.Tried = append(out.Tried, step)
			if accept(step, rec, items, false) {
				out.Excerpts = listingExcerpts(page, items)
				return out, nil
			}
		}
	}
	if !in.AllowModel || opts.Model == nil {
		return out, nil
	}
	res, err := setupListing(ctx, in.Recipe.ListingURL, opts, false)
	if res != nil {
		res.Recipe.Setup.SectionChecked = in.Recipe.Setup.SectionChecked
		out.Tried = append(out.Tried, RepairJudge)
		calls, usd := res.Recipe.Setup.ModelCost()
		out.UsedModel, out.ModelUSD = calls > 0, usd
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return out, ctxErr
		}
		return out, nil
	}
	if in.Doubted && !res.Healthy() {
		return out, nil
	}
	if !res.Blocked && accept(RepairJudge, res.Recipe, res.Items, out.UsedModel) {
		out.Excerpts = res.Excerpts
	}
	return out, nil
}

func (out *RepairResult) accept(step RepairStep, rec Recipe, items []Item, history []GoodPoll, modelPicked bool) bool {
	templates := recipeTemplates(rec)
	dominant := DominantTemplate(items, templates)
	rule := acceptRepair(items, dominant, history, modelPicked)
	if rule == "" {
		return false
	}
	out.take(step, rule, rec, items)
	out.FalseAlarm = slices.Equal(keySet(itemURLKeys(items)), keySet(urlKeys(history[0].URLs)))
	return true
}

func (out *RepairResult) acceptOther(step RepairStep, rec Recipe, items []Item, doubted []string) bool {
	if len(items) < acceptMinItems || slices.Equal(keySet(itemURLKeys(items)), doubted) {
		return false
	}
	templates := recipeTemplates(rec)
	if len(ComputeSignals(items, templates).doubts()) > 0 {
		return false
	}
	out.take(step, AcceptDifferent, rec, items)
	return true
}

func recipeTemplates(rec Recipe) []string {
	if rec.Mode == ModeCSS {
		return slices.Clone(rec.Fingerprint.URLTemplates)
	}
	return streamTemplates(rec.Streams)
}

func (out *RepairResult) take(step RepairStep, rule string, rec Recipe, items []Item) {
	templates := recipeTemplates(rec)
	sig := ComputeSignals(items, templates)
	rec.Fingerprint = fingerprintOf(sig, templates)
	out.Accepted, out.Step, out.Rule = true, step, rule
	out.Recipe, out.Items = rec, items
}

func acceptRepair(items []Item, dominant string, history []GoodPoll, modelStep bool) string {
	if len(items) == 0 || len(history) == 0 {
		return ""
	}
	last := history[0]
	known := map[string]bool{}
	for _, u := range last.URLs {
		known[postkey.URL(u)] = true
	}
	for _, it := range items {
		if known[postkey.URL(it.URL)] {
			return AcceptKnownURL
		}
	}
	if len(items) >= acceptMinItems && last.URLTemplate != "" && dominant == last.URLTemplate {
		return AcceptTemplate
	}
	current := map[string]bool{}
	for _, it := range items {
		current[postkey.Title(it.Title)] = true
	}
	reappear := 0
	for _, t := range last.Titles {
		if current[postkey.Title(t)] {
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

func urlKeys(urls []string) []string {
	keys := make([]string, len(urls))
	for i, u := range urls {
		keys[i] = postkey.URL(u)
	}
	return keys
}

func keySet(keys []string) []string { return slices.Compact(slices.Sorted(slices.Values(keys))) }

func reinfer(step RepairStep, p *pageDoc, in RepairInput) (Recipe, bool) {
	all := p.candidates()
	last := in.History[0]
	var picked []*cluster
	switch step {
	case RepairKnownURLs:
		known := map[string]bool{}
		for _, u := range last.URLs {
			known[postkey.URL(u)] = true
		}
		for _, c := range all {
			if !c.sideList() && slices.ContainsFunc(c.entries, func(e *entry) bool { return known[postkey.URL(e.URL)] }) {
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

func rebuilt(old Recipe, picked []*cluster, step RepairStep) Recipe {
	rec := old
	rec.Mode, rec.CSS = ModeStreams, nil
	rec.Streams = buildStreams(picked, old.ListingURL)
	for i := range rec.Streams {
		for _, o := range old.Streams {
			if o.URLTemplate == rec.Streams[i].URLTemplate && o.TitleHints.Source == TitleCalibrated {
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

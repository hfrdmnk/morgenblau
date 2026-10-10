package websource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"morgenblau/internal/webfeed"
)

// WebfeedGenerator is the Generator over internal/webfeed. Polls and the free repair steps never call a model; the
// judge and inducer in its options run only at setup and in the once-a-day model repair, the watcher once a week.
type WebfeedGenerator struct {
	opts    webfeed.Options
	watcher webfeed.Watcher
}

// NewWebfeedGenerator takes options with an SSRF-safe client; nil Judge, Inducer or watcher leave those steps out.
// Polls share one robots.txt cache unless opts brings its own.
func NewWebfeedGenerator(opts webfeed.Options, watcher webfeed.Watcher) *WebfeedGenerator {
	if opts.Robots == nil {
		opts.Robots = webfeed.NewRobotsCache()
	}
	return &WebfeedGenerator{opts: opts, watcher: watcher}
}

func (g *WebfeedGenerator) Setup(ctx context.Context, listingURL, anchorURL string, allowModel bool) (Generated, error) {
	opts := g.opts
	if !allowModel {
		opts = g.freeOpts()
	}
	var res *webfeed.Result
	var err error
	if anchorURL != "" {
		res, err = webfeed.SetupAnchored(ctx, listingURL, anchorURL, opts)
	} else {
		res, err = webfeed.Setup(ctx, listingURL, opts)
	}
	if err != nil {
		return Generated{}, err
	}
	recipe, err := json.Marshal(res.Recipe)
	if err != nil {
		return Generated{}, err
	}
	health := Health(res.Health)
	gen := Generated{
		Recipe:       recipe,
		Title:        res.Title,
		Health:       health,
		HealthReason: setupReason(health, res.HealthReasons),
		NeedsAnchor:  health == HealthNeedsAttention,
		Items:        toItems(res.Items, res.Excerpts),
		Fingerprint:  fingerprint(res.Items, res.Recipe),
		Cost:         setupCost(res.Recipe.Setup),
	}
	slog.Info("web source setup", "listing", listingURL, "anchored", anchorURL != "", "health", health,
		"reasons", res.HealthReasons, "items", len(res.Items), "stages", gen.Cost.Stages, "usd", gen.Cost.USD)
	return gen, nil
}

func (g *WebfeedGenerator) Poll(ctx context.Context, raw json.RawMessage, prev PollState) (PollOutcome, error) {
	var recipe webfeed.Recipe
	if err := json.Unmarshal(raw, &recipe); err != nil {
		return PollOutcome{}, fmt.Errorf("web source recipe: %w", err)
	}
	history := goodPolls(prev.History)
	check := prev.CheckDue && g.watcher != nil
	polled, err := webfeed.Poll(ctx, webfeed.PollInput{
		Recipe: recipe, ETag: prev.ETag, LastModified: prev.LastModified, LinkSetHash: prev.LinkSetHash,
		History: history, Unconditional: check || prev.WeeklyAlarm,
	}, g.freeOpts())
	if err != nil {
		return PollOutcome{}, err
	}
	out := PollOutcome{
		ETag: polled.ETag, LastModified: polled.LastModified, LinkSetHash: polled.LinkSetHash, NotModified: polled.NotModified,
	}
	switch polled.Verdict {
	case webfeed.PollBroken:
		slog.Info("web source poll broken", "listing", recipe.ListingURL, "reason", polled.Reason, "blocked", polled.Blocked)
		out.Verdict, out.Reason, out.Blocked = VerdictBroken, brokenReason(polled.Reason, polled.Blocked), polled.Blocked
		return out, nil
	case webfeed.PollSuspect:
		if prev.WeeklyAlarm {
			return g.repair(ctx, out, polled, recipe, history, prev, RepairCauseWeeklyCheck, polled.Reason)
		}
		return g.repair(ctx, out, polled, recipe, history, prev, RepairCauseDetector, polled.Reason)
	}
	if polled.NotModified {
		out.Verdict = VerdictOK
		return out, nil
	}
	if check && polled.Page() != nil {
		out.Checked = true
		rec, err := webfeed.WeeklyCheck(ctx, g.watcher, recipe.ListingURL, polled.Page(), historyItems(history), polled.Items)
		out.CheckUSD = rec.USD
		slog.Info("web source weekly check", "listing", recipe.ListingURL, "broken", rec.Broken, "reason", rec.Reason, "usd", rec.USD, "err", err)
		if err == nil && rec.Broken {
			return g.repair(ctx, out, polled, recipe, history, prev, RepairCauseWeeklyCheck, "weekly_check")
		}
		if err == nil {
			prev.WeeklyAlarm = false
		}
	}
	if prev.WeeklyAlarm && polled.Page() != nil {
		return g.repair(ctx, out, polled, recipe, history, prev, RepairCauseWeeklyCheck, "weekly_check")
	}
	out.Verdict = VerdictOK
	out.Items = toItems(polled.Items, polled.Excerpts)
	out.Fingerprint = fingerprint(polled.Items, recipe)
	return out, nil
}

// repair runs the ladder on the page the poll fetched; an accepted repair publishes like an ok poll with its new recipe.
func (g *WebfeedGenerator) repair(ctx context.Context, out PollOutcome, polled *webfeed.PollResult, recipe webfeed.Recipe,
	history []webfeed.GoodPoll, prev PollState, cause RepairCause, reason string) (PollOutcome, error) {
	res, err := webfeed.Repair(ctx, polled, webfeed.RepairInput{
		Recipe: recipe, History: history, AllowModel: prev.AllowModelRepair, Doubted: cause == RepairCauseWeeklyCheck,
	}, g.opts)
	if err != nil {
		return PollOutcome{}, err
	}
	report := &RepairReport{Cause: cause, Reason: reason, UsedModel: res.UsedModel, USD: res.ModelUSD, Outcome: RepairUnresolved}
	if n := len(res.Tried); n > 0 {
		report.Step = string(res.Tried[n-1])
	}
	out.Repair = report
	slog.Info("web source repair", "listing", recipe.ListingURL, "cause", cause, "reason", reason, "tried", res.Tried,
		"accepted", res.Accepted, "step", res.Step, "rule", res.Rule, "false_alarm", res.FalseAlarm, "model", res.UsedModel, "usd", res.ModelUSD)
	// The weekly check doubts the very posts a free re-infer anchors on, so reproducing them cannot close its alarm.
	if !res.Accepted || (cause == RepairCauseWeeklyCheck && res.FalseAlarm) {
		out.Verdict, out.Reason = VerdictSuspect, unresolvedReason(cause)
		return out, nil
	}
	newRecipe, err := json.Marshal(res.Recipe)
	if err != nil {
		return PollOutcome{}, err
	}
	report.Outcome, report.Step, report.Rule = RepairAccepted, string(res.Step), res.Rule
	if res.FalseAlarm {
		report.Outcome = RepairFalseAlarm
	}
	out.Verdict, out.Recipe = VerdictOK, newRecipe
	out.Items = toItems(res.Items, res.Excerpts)
	out.Fingerprint = fingerprint(res.Items, res.Recipe)
	return out, nil
}

// freeOpts are the options for steps that must never reach a model.
func (g *WebfeedGenerator) freeOpts() webfeed.Options {
	opts := g.opts
	opts.Judge, opts.Inducer = nil, nil
	return opts
}

// setupReason words the self-check's first reason for the Sources page.
func setupReason(h Health, reasons []string) string {
	if h == HealthOK {
		return ""
	}
	if h == HealthBlockedBySite {
		return "this site blocks feed readers"
	}
	for _, r := range reasons {
		switch r {
		case "count":
			return "only a few posts were found"
		case "title_shape":
			return "the post titles look unusual"
		case "dup_titles":
			return "many posts share one title"
		case "template_share":
			return "many links do not look like posts"
		case "unsure":
			return unsureReason
		case "judge_none":
			return "no list on the page looks like posts"
		}
	}
	return unsureReason
}

const unsureReason = "Morgenblau isn't sure which links are posts"

func brokenReason(code string, blocked bool) string {
	switch code {
	case webfeed.ReasonRobots:
		return "this site asks feed readers not to read this page"
	case webfeed.ReasonChallenge:
		return "the site showed a bot check instead of the page"
	case webfeed.ReasonHTTPStatus:
		if blocked {
			return "the site refused to show the page"
		}
		return "the site returned an error"
	case webfeed.ReasonErrorPage:
		return "the site showed an error page"
	case webfeed.ReasonConsentWall:
		return "the site showed a cookie wall instead of the page"
	case webfeed.ReasonSPAShell:
		return "the page now needs JavaScript to show its posts"
	case webfeed.ReasonNotHTML:
		return "the page is no longer a web page"
	}
	return "the site could not be reached"
}

func unresolvedReason(cause RepairCause) string {
	if cause == RepairCauseWeeklyCheck {
		return "a weekly check found posts that no longer match the page"
	}
	return "the page changed and the post list could not be found again"
}

func toItems(items []webfeed.Item, excerpts map[string]string) []Item {
	out := make([]Item, len(items))
	for i, it := range items {
		out[i] = Item{URL: it.URL, Title: it.Title, Date: it.Date, Excerpt: excerpts[it.URL]}
	}
	return out
}

func fingerprint(items []webfeed.Item, r webfeed.Recipe) Fingerprint {
	templates := make([]string, 0, len(r.Streams))
	for _, st := range r.Streams {
		templates = append(templates, st.URLTemplate)
	}
	sig := webfeed.ComputeSignals(items, templates)
	return Fingerprint{URLTemplate: webfeed.DominantTemplate(items, templates), DateFrac: sig.DateFrac, TitleShape: sig.TitleShape}
}

func goodPolls(history []PollRecord) []webfeed.GoodPoll {
	out := make([]webfeed.GoodPoll, len(history))
	for i, h := range history {
		out[i] = webfeed.GoodPoll{URLs: h.URLs, Titles: h.Titles, URLTemplate: h.URLTemplate, TitleShape: h.TitleShape}
	}
	return out
}

// historyItems is the last good poll as items, the weekly check's reference.
func historyItems(history []webfeed.GoodPoll) []webfeed.Item {
	if len(history) == 0 {
		return nil
	}
	last := history[0]
	out := make([]webfeed.Item, len(last.URLs))
	for i, u := range last.URLs {
		out[i].URL = u
		if i < len(last.Titles) {
			out[i].Title = last.Titles[i]
		}
	}
	return out
}

func setupCost(r webfeed.SetupRecord) Cost {
	c := Cost{Stages: r.Stages}
	if r.Judge != nil {
		c.JudgeCalls = r.Judge.Calls
		c.InputTokens += r.Judge.InputTokens
		c.OutputTokens += r.Judge.OutputTokens
	}
	if r.Induce != nil {
		c.InputTokens += r.Induce.InputTokens
		c.OutputTokens += r.Induce.OutputTokens
	}
	_, c.USD = r.ModelCost()
	return c
}

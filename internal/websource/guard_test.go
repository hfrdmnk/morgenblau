package websource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func sourceSetupSeq(t *testing.T, env *testEnv, id string) (recipe string, polls int) {
	t.Helper()
	return readSource(t, env, id).recipe, pollCount(t, env, id)
}

func TestPollDropsItsWritesWhenASetupReplacedTheSourceMeanwhile(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictSuspect, Reason: "count dropped"}}
	pollDue(t, env)

	env.advance(31 * time.Minute)
	anchored := Generated{
		Recipe: json.RawMessage(`{"version":1,"mode":"css"}`), Title: "Example Publication", SiteURL: "https://news.example.com/", Health: HealthOK,
		Items:       []Item{{URL: "https://news.example.com/stories/a", Title: "Story A"}},
		Fingerprint: Fingerprint{URLTemplate: "news.example.com/stories/*"},
	}
	env.gen.outcomes = []PollOutcome{{
		Verdict: VerdictOK, Recipe: json.RawMessage(`{"version":1,"mode":"streams","stale":true}`),
		Items:  []Item{{URL: "https://news.example.com/posts/fresh", Title: "Fresh from the old recipe"}},
		Repair: &RepairReport{Cause: RepairCauseDetector, Outcome: RepairAccepted, Step: "known_urls", Rule: "known_url"},
	}}
	env.gen.duringPoll = func() {
		env.gen.duringPoll = nil
		env.gen.generated = anchored
		if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/stories/a"); err != nil {
			t.Error(err)
		}
	}
	pollDue(t, env)

	recipe, polls := sourceSetupSeq(t, env, id)
	if recipe != `{"version":1,"mode":"css"}` || polls != 1 {
		t.Errorf("recipe %s with %d good polls, want the new setup's recipe and only its poll", recipe, polls)
	}
	if row := readSource(t, env, id); row.health != string(HealthOK) {
		t.Errorf("health = %s, want the new setup's", row.health)
	}
	if _, ok := storedItems(t, env, id)["https://news.example.com/posts/fresh"]; ok {
		t.Error("the stale poll published its items over the new setup")
	}
	if rows := repairRows(t, env, id); len(rows) != 0 {
		t.Errorf("repairs = %+v, want the stale poll's repair dropped", rows)
	}
}

func TestCreateIgnoresAnAnchorForAHealthySource(t *testing.T) {
	env := newTestEnv(t)
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	env.advance(time.Hour)
	env.gen.generated.Recipe = json.RawMessage(`{"version":1,"mode":"css"}`)
	again, err := env.service.Create(context.Background(), "did:plc:other", "https://news.example.com/", "https://news.example.com/posts/elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	if env.gen.setupCount() != 1 || again.Source.ID != first.Source.ID || again.ItemCount != 3 {
		t.Errorf("setups = %d, preview = %+v; want a healthy source left as it is", env.gen.setupCount(), again)
	}
}

func TestCreateRejectsAnAnchorOffTheListingsSite(t *testing.T) {
	env := newTestEnv(t)
	for _, anchor := range []string{"https://elsewhere.example.org/posts/a", "https://example.com.evil.example/posts/a"} {
		if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", anchor); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("anchor %s: err = %v, want ErrInvalidURL", anchor, err)
		}
	}
	for _, anchor := range []string{"https://www.news.example.com/posts/a", "http://news.example.com/posts/a"} {
		if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", anchor); err != nil {
			t.Errorf("anchor %s on the listing's own site: %v", anchor, err)
		}
	}
}

func TestCreateKeepsTheOldSetupWhenTheNewOneIsNoBetter(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Health = HealthNeedsAttention
	env.gen.generated.HealthReason = "Morgenblau isn't sure which links are posts"
	env.gen.generated.NeedsAnchor = true
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}

	env.advance(11 * time.Minute)
	env.gen.generated = Generated{Recipe: json.RawMessage(`{}`), Health: HealthBlockedBySite, HealthReason: "this site blocks feed readers"}
	again, err := env.service.Create(context.Background(), "did:plc:other", "https://news.example.com/", "https://news.example.com/posts/third")
	if err != nil {
		t.Fatal(err)
	}
	if env.gen.setupCount() != 2 {
		t.Fatalf("setups = %d, want the anchored setup run", env.gen.setupCount())
	}
	if again.Source.Health != HealthNeedsAttention || again.ItemCount != 3 || !again.Source.NeedsAnchor {
		t.Errorf("preview = %+v, want the old setup kept over a blocked one", again)
	}
	if row := readSource(t, env, first.Source.ID); row.recipe != `{"version":1,"mode":"streams"}` {
		t.Errorf("recipe = %s, want the old one", row.recipe)
	}

	env.advance(5 * time.Minute)
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	if env.gen.setupCount() != 2 {
		t.Errorf("setups = %d, want a just-tried setup not rerun on reuse", env.gen.setupCount())
	}
}

func TestCreateDoesNotRegenerateAfterATransientOutage(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictBroken, Reason: "the site could not be reached"}}
	pollDue(t, env)
	if row := readSource(t, env, id); row.health != string(HealthBroken) {
		t.Fatalf("health = %s", row.health)
	}
	env.advance(time.Hour)
	if _, err := env.service.Create(context.Background(), "did:plc:other", "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	if env.gen.setupCount() != 1 {
		t.Errorf("setups = %d, want a healthy setup kept through an outage", env.gen.setupCount())
	}
}

func TestPollDueLeavesAnUnsureSetupAlone(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Health = HealthNeedsAttention
	env.gen.generated.HealthReason = "Morgenblau isn't sure which links are posts"
	env.gen.generated.NeedsAnchor = true
	id := followed(t, env)
	for range 3 {
		env.advance(31 * time.Minute)
		env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{{URL: "https://news.example.com/tags/design", Title: "Design"}}}}
		pollDue(t, env)
	}
	if len(env.gen.polls) != 0 {
		t.Errorf("polled an unsure setup %d times", len(env.gen.polls))
	}
	row := readSource(t, env, id)
	if row.health != string(HealthNeedsAttention) || row.reason != "Morgenblau isn't sure which links are posts" {
		t.Errorf("source = %+v, want the setup's doubt to stand until an anchored or healthy setup", row)
	}
	if _, ok := storedItems(t, env, id)["https://news.example.com/tags/design"]; ok {
		t.Error("published from a list the setup doubted")
	}
}

func TestPollDueRecurringTitlesStayNewPosts(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Items = []Item{
		{URL: "https://news.example.com/posts/weekly-roundup-41", Title: "Weekly roundup", Date: day(3)},
		{URL: "https://news.example.com/posts/release-notes-1", Title: "Release notes", Date: day(2)},
		{URL: "https://news.example.com/posts/first", Title: "First post", Date: day(1)},
	}
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{
		{URL: "https://news.example.com/posts/weekly-roundup-42", Title: "Weekly roundup", Date: day(10)},
		{URL: "https://news.example.com/posts/weekly-roundup-41", Title: "Weekly roundup", Date: day(3)},
		{URL: "https://news.example.com/posts/release-notes-2", Title: "Release Notes", Date: day(9)},
		{URL: "https://news.example.com/posts/first", Title: "First post", Date: day(1)},
	}}}
	pollDue(t, env)
	items := storedItems(t, env, id)
	for _, u := range []string{"https://news.example.com/posts/weekly-roundup-42", "https://news.example.com/posts/release-notes-2"} {
		if it, ok := items[u]; !ok || it.history {
			t.Errorf("%s = %+v, want a new post: its URL has the stream's shape, so a known title is a recurring one", u, it)
		}
	}
}

func TestCreateStoresOnlyWebLinksAndAtMostTwoHundred(t *testing.T) {
	env := newTestEnv(t)
	items := []Item{
		{URL: "javascript://news.example.com/%0aalert(1)", Title: "Click me"},
		{URL: "data:text/html,hi", Title: "Data"},
		{URL: "https:///no-host", Title: "No host"},
	}
	for i := range 250 {
		items = append(items, Item{URL: fmt.Sprintf("https://news.example.com/posts/post-%d", i), Title: fmt.Sprintf("Post %d", i)})
	}
	env.gen.generated.Items = items
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	stored := storedItems(t, env, preview.Source.ID)
	if len(stored) != maxItems {
		t.Errorf("stored %d items, want %d", len(stored), maxItems)
	}
	for u := range stored {
		if !strings.HasPrefix(u, "https://news.example.com/") {
			t.Errorf("stored %q", u)
		}
	}
}

func TestCreateCapsNewSourcesPerReaderPerDay(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	for i := range dailySources {
		if _, err := env.service.Create(ctx, testDID, fmt.Sprintf("https://site%d.example.com/", i), ""); err != nil {
			t.Fatal(err)
		}
	}
	_, err := env.service.Create(ctx, testDID, "https://one-more.example.com/", "")
	var limited *RateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 || limited.RetryAfter > 24*time.Hour {
		t.Fatalf("err = %v, want a rate limit until the oldest of today's sources is a day old", err)
	}
	if _, err := env.service.Create(ctx, testDID, "https://site0.example.com/", ""); err != nil {
		t.Errorf("reusing a source counted against the daily cap: %v", err)
	}
	if _, err := env.service.Create(ctx, "did:plc:other", "https://one-more.example.com/", ""); err != nil {
		t.Errorf("another reader was capped: %v", err)
	}
	env.advance(24 * time.Hour)
	if _, err := env.service.Create(ctx, testDID, "https://later.example.com/", ""); err != nil {
		t.Errorf("still capped a day later: %v", err)
	}
}

func spend(t *testing.T, env *testEnv) float64 {
	t.Helper()
	var usd float64
	if err := env.reader.QueryRow(`SELECT COALESCE(SUM(usd), 0) FROM web_model_spend`).Scan(&usd); err != nil {
		t.Fatal(err)
	}
	return usd
}

func TestModelStepsStopAtTheDailyBudget(t *testing.T) {
	env := newTestEnv(t)
	env.service.modelDailyUSD = 0.0075
	env.gen.generated.Cost = Cost{Stages: []string{"heuristic", "judge"}, JudgeCalls: 3, USD: 0.006}
	followed(t, env)
	if !env.gen.allowModel[0] {
		t.Fatal("the first setup of the day may not use the model")
	}
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, CheckUSD: 0.0004, Checked: true}, modelRepair(RepairUnresolved), {Verdict: VerdictOK}}
	pollDue(t, env)
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if got := spend(t, env); got < 0.0079 || got > 0.0081 {
		t.Errorf("recorded spend = %v, want setup, check and repair", got)
	}

	if _, err := env.service.Create(context.Background(), testDID, "https://other.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	if env.gen.allowModel[1] {
		t.Error("a setup used the model over the daily budget")
	}
	env.advance(25 * time.Hour)
	pollDue(t, env)
	last := env.gen.polls[len(env.gen.polls)-1].prev
	if !last.AllowModelRepair {
		t.Error("the model stayed off a day after the budget was spent")
	}
	env.advance(8 * 24 * time.Hour)
	if _, err := env.writer.Exec(`INSERT INTO web_model_spend (spent_at, kind, usd) VALUES (?, 'setup', 1)`, formatTime(env.now)); err != nil {
		t.Fatal(err)
	}
	pollDue(t, env)
	if last := env.gen.polls[len(env.gen.polls)-1].prev; last.AllowModelRepair || last.CheckDue {
		t.Errorf("poll state = %+v, want repair and check kept off the model over budget", last)
	}
}

func TestCreateBoundsSetupByItsOwnDeadline(t *testing.T) {
	env := newTestEnv(t)
	start := time.Now()
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	deadline, ok := env.gen.setupCtxs[0].Deadline()
	if !ok || deadline.After(start.Add(SetupTimeout+time.Second)) || deadline.Before(start.Add(SetupTimeout-time.Second)) {
		t.Errorf("setup deadline = %v (%v), want about %v from now", deadline.Sub(start), ok, SetupTimeout)
	}
}

func TestCreateRunsOneSetupPerReaderAndAFewAtOnce(t *testing.T) {
	env := newTestEnv(t)
	env.service.setupTimeout = time.Minute
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(setupSlots)
	env.gen.beforeSetup = func(ctx context.Context) {
		started.Done()
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	errs := make(chan error, setupSlots)
	var wg sync.WaitGroup
	for i := range setupSlots {
		wg.Go(func() {
			_, err := env.service.Create(context.Background(), fmt.Sprintf("did:plc:reader%d", i), fmt.Sprintf("https://site%d.example.com/", i), "")
			errs <- err
		})
	}
	started.Wait()
	env.gen.beforeSetup = nil

	if _, err := env.service.Create(context.Background(), "did:plc:reader0", "https://another.example.com/", ""); !errors.Is(err, ErrSetupInFlight) {
		t.Errorf("a second setup for one reader: err = %v, want ErrSetupInFlight", err)
	}
	env.service.setupTimeout = 100 * time.Millisecond
	if _, err := env.service.Create(context.Background(), "did:plc:waiting", "https://waiting.example.com/", ""); !errors.Is(err, ErrBusy) {
		t.Errorf("a setup past the slots: err = %v, want ErrBusy", err)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if _, err := env.service.Create(context.Background(), "did:plc:reader0", "https://another.example.com/", ""); err != nil {
		t.Errorf("a reader stayed locked out after their setup finished: %v", err)
	}
}

func TestWeeklyAlarmStaysOpenUntilARepairFindsADifferentList(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(checkEvery + 31*time.Minute)
	reason := "a weekly check found posts that no longer match the page"
	unresolved := func(checked bool) PollOutcome {
		return PollOutcome{Verdict: VerdictSuspect, Reason: reason, Checked: checked,
			Repair: &RepairReport{Cause: RepairCauseWeeklyCheck, Outcome: RepairUnresolved, Step: "heuristic", Reason: "weekly_check"}}
	}
	env.gen.outcomes = []PollOutcome{
		unresolved(true),
		unresolved(false),
		{Verdict: VerdictOK, Recipe: json.RawMessage(`{"version":1,"mode":"css"}`),
			Items:  []Item{{URL: "https://news.example.com/posts/a-real-post", Title: "A real post"}},
			Repair: &RepairReport{Cause: RepairCauseWeeklyCheck, Outcome: RepairAccepted, Step: "heuristic", Rule: "different_list", Reason: "weekly_check"}},
		{Verdict: VerdictOK},
	}
	pollDue(t, env)
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if prev := env.gen.polls[1].prev; !prev.WeeklyAlarm || prev.CheckDue {
		t.Errorf("poll state during the alarm = %+v, want the alarm passed on and the check not yet due", prev)
	}
	if row := readSource(t, env, id); row.health != string(HealthNeedsAttention) {
		t.Errorf("health = %s while the alarm stands", row.health)
	}
	env.advance(31 * time.Minute)
	pollDue(t, env)
	row := readSource(t, env, id)
	if row.health != string(HealthOK) || row.recipe != `{"version":1,"mode":"css"}` {
		t.Errorf("source = %+v, want a repair onto a different list to close the alarm", row)
	}
	if it, ok := storedItems(t, env, id)["https://news.example.com/posts/a-real-post"]; !ok || it.history {
		t.Errorf("item = %+v, want the repaired list published", it)
	}
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if env.gen.polls[3].prev.WeeklyAlarm {
		t.Error("the alarm stayed open after an accepted repair")
	}
}

func TestWeeklyCheckReturnsADayAfterItsAlarm(t *testing.T) {
	env := newTestEnv(t)
	followed(t, env)
	env.advance(checkEvery + 31*time.Minute)
	alarm := PollOutcome{Verdict: VerdictSuspect, Reason: "a weekly check found posts that no longer match the page", Checked: true,
		Repair: &RepairReport{Cause: RepairCauseWeeklyCheck, Outcome: RepairUnresolved, Step: "heuristic"}}
	retry := alarm
	retry.Checked = false
	env.gen.outcomes = []PollOutcome{alarm, retry}
	pollDue(t, env)
	env.advance(23 * time.Hour)
	pollDue(t, env)
	if env.gen.polls[1].prev.CheckDue {
		t.Error("check due again within a day of its alarm")
	}
	env.advance(time.Hour + time.Minute)
	pollDue(t, env)
	if !env.gen.polls[2].prev.CheckDue {
		t.Error("check not due a day after its alarm")
	}
}

package websource

import (
	"context"
	"strings"
	"testing"
	"time"
)

type repairRow struct {
	cause, outcome, step, rule, reason string
	usedModel                          bool
	usd                                float64
}

func repairRows(t *testing.T, env *testEnv, id string) []repairRow {
	t.Helper()
	rows, err := env.reader.Query(`SELECT cause, outcome, step, rule, reason, used_model, usd FROM web_source_repairs WHERE source_id = ? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []repairRow
	for rows.Next() {
		var r repairRow
		if err := rows.Scan(&r.cause, &r.outcome, &r.step, &r.rule, &r.reason, &r.usedModel, &r.usd); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func modelRepair(outcome RepairOutcome) PollOutcome {
	v := VerdictSuspect
	if outcome != RepairUnresolved {
		v = VerdictOK
	}
	return PollOutcome{Verdict: v, Reason: "the page changed and the post list could not be found again", Repair: &RepairReport{
		Cause: RepairCauseDetector, Outcome: outcome, Step: "judge", Reason: "empty", UsedModel: true, USD: 0.0016,
	}}
}

func TestPollDueRecordsRepairsAndSpendsTheModelOncePerDay(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if !env.gen.polls[0].prev.AllowModelRepair {
		t.Fatal("a source that never used the model may not use it")
	}

	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{modelRepair(RepairUnresolved), {Verdict: VerdictOK}}
	pollDue(t, env)
	rows := repairRows(t, env, id)
	if len(rows) != 1 || rows[0] != (repairRow{cause: "detector", outcome: "unresolved", step: "judge", reason: "empty", usedModel: true, usd: 0.0016}) {
		t.Fatalf("repairs = %+v", rows)
	}
	if row := readSource(t, env, id); row.health != string(HealthNeedsAttention) || row.reason != "the page changed and the post list could not be found again" {
		t.Errorf("source after an unresolved repair = %+v", row)
	}

	env.advance(23 * time.Hour)
	pollDue(t, env)
	if env.gen.polls[2].prev.AllowModelRepair {
		t.Error("a second model repair was allowed within a day")
	}
	env.advance(time.Hour + time.Minute)
	pollDue(t, env)
	if !env.gen.polls[3].prev.AllowModelRepair {
		t.Error("the model repair stayed spent after a day")
	}
}

func TestPollDueRecordsAFreeFalseAlarm(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{{URL: "https://news.example.com/posts/third", Title: "Third post"}},
		Repair: &RepairReport{Cause: RepairCauseDetector, Outcome: RepairFalseAlarm, Step: "known_urls", Rule: "known_url", Reason: "title_shape"}}}
	pollDue(t, env)
	rows := repairRows(t, env, id)
	if len(rows) != 1 || rows[0].outcome != "false_alarm" || rows[0].usedModel || rows[0].rule != "known_url" {
		t.Fatalf("repairs = %+v", rows)
	}
	if got := pollCount(t, env, id); got != 2 {
		t.Errorf("good polls = %d, want the repaired poll in history", got)
	}
}

func TestPollDueRaisesTheBudgetAlarmAboveThreeModelRepairsAWeek(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	for i := range 4 {
		env.advance(25 * time.Hour)
		env.gen.outcomes = []PollOutcome{modelRepair(RepairAccepted)}
		pollDue(t, env)
		row := readSource(t, env, id)
		alarmed := strings.Contains(row.reason, "changed often")
		if alarmed != (i == 3) {
			t.Fatalf("repair %d: reason %q", i+1, row.reason)
		}
		if row.health != string(HealthOK) {
			t.Fatalf("repair %d: health %s, want an accepted repair to stay ok", i+1, row.health)
		}
	}
}

func TestPollDueAsksForTheWeeklyCheck(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if env.gen.polls[0].prev.CheckDue {
		t.Fatal("check due right after setup")
	}
	env.advance(7 * 24 * time.Hour)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Checked: true}, {Verdict: VerdictOK}}
	pollDue(t, env)
	if !env.gen.polls[1].prev.CheckDue {
		t.Fatal("check not due a week after setup")
	}
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if env.gen.polls[2].prev.CheckDue {
		t.Error("check still due right after it ran")
	}
	var checked *string
	if err := env.reader.QueryRow(`SELECT last_checked_at FROM web_sources WHERE id = ?`, id).Scan(&checked); err != nil || checked == nil || *checked != "2026-10-17T09:31:00Z" {
		t.Errorf("last_checked_at = %v, %v", checked, err)
	}
}

func TestPollDueSiteThatBlocksUsReadsBlockedAndBacksOff(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictBroken, Blocked: true, Reason: "the site showed a bot check instead of the page"}}
	pollDue(t, env)
	row := readSource(t, env, id)
	if row.health != string(HealthBlockedBySite) || row.failures != 1 || row.nextPoll != "2026-10-10T10:31:00Z" {
		t.Errorf("source = %+v", row)
	}
}

func TestPollDueFailedPollReadsPlainly(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.pollErr = context.DeadlineExceeded
	pollDue(t, env)
	if row := readSource(t, env, id); row.reason != "the site could not be reached" {
		t.Errorf("reason = %q, want reader language", row.reason)
	}
}

func TestPollDueMovedPostsJoinAsHistory(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{
		{URL: "https://news.example.com/articles/brand-new", Title: "Brand new"},
		{URL: "https://news.example.com/articles/third", Title: "Third Post", Date: day(8)},
		{URL: "https://news.example.com/articles/second", Title: "Second post"},
	}, Repair: &RepairReport{Cause: RepairCauseDetector, Outcome: RepairAccepted, Step: "heuristic", Rule: "titles", Reason: "empty"}}}
	pollDue(t, env)
	items := storedItems(t, env, id)
	if it := items["https://news.example.com/articles/brand-new"]; it.history {
		t.Errorf("a new post was stored as history: %+v", it)
	}
	for _, u := range []string{"https://news.example.com/articles/third", "https://news.example.com/articles/second"} {
		if it, ok := items[u]; !ok || !it.history {
			t.Errorf("%s: moved post = %+v, want stored as history so it stays out of today's paper", u, it)
		}
	}
}

func TestPollDueKeepsAMonthOfRepairs(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	if _, err := env.writer.Exec(`INSERT INTO web_source_repairs (source_id, attempted_at, cause, outcome) VALUES (?, '2026-09-09T09:00:00Z', 'detector', 'unresolved'), (?, '2026-09-11T09:00:00Z', 'detector', 'unresolved')`, id, id); err != nil {
		t.Fatal(err)
	}
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{modelRepair(RepairUnresolved)}
	pollDue(t, env)
	if rows := repairRows(t, env, id); len(rows) != 2 {
		t.Errorf("repairs kept = %d, want the one from the last month plus today's", len(rows))
	}
}

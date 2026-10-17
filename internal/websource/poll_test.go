package websource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"
)

// followed creates a web source and puts its feed URL in the shared feeds catalog, as a subscription would.
func followed(t *testing.T, env *testEnv) string {
	t.Helper()
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, preview.Source.FeedURL); err != nil {
		t.Fatal(err)
	}
	return preview.Source.ID
}

type sourceRow struct {
	recipe, health, reason string
	nextPoll               string
	lastPolled, etag, hash *string
	failures               int
}

func readSource(t *testing.T, env *testEnv, id string) sourceRow {
	t.Helper()
	var r sourceRow
	var next *string
	if err := env.reader.QueryRow(`SELECT recipe, health, health_reason, next_poll_at, last_polled_at, etag, link_set_hash, consecutive_failures FROM web_sources WHERE id = ?`, id).
		Scan(&r.recipe, &r.health, &r.reason, &next, &r.lastPolled, &r.etag, &r.hash, &r.failures); err != nil {
		t.Fatal(err)
	}
	if next != nil {
		r.nextPoll = *next
	}
	return r
}

func pollCount(t *testing.T, env *testEnv, id string) int {
	t.Helper()
	var n int
	if err := env.reader.QueryRow(`SELECT COUNT(*) FROM web_source_polls WHERE source_id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func pollDue(t *testing.T, env *testEnv) int {
	t.Helper()
	n, err := env.service.PollDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPollDuePublishesNewItemsOnOK(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{
		Verdict: VerdictOK,
		Items: []Item{
			{URL: "https://news.example.com/posts/fresh-undated", Title: "Fresh undated"},
			{URL: "https://news.example.com/posts/fresh-dated", Title: "Fresh dated", Date: day(9)},
			{URL: "https://news.example.com/posts/third", Title: "Third post renamed"},
			{URL: "https://news.example.com/posts/decoy", Title: "Old decoy", Date: day(1)},
		},
		ETag: `"v2"`, LastModified: "Fri, 09 Oct 2026 08:00:00 GMT", LinkSetHash: "hash-2",
		Fingerprint: Fingerprint{URLTemplate: "news.example.com/posts/*", DateFrac: 0.5, TitleShape: 1},
	}}

	if n := pollDue(t, env); n != 1 {
		t.Fatalf("polled %d sources, want 1", n)
	}
	call := env.gen.polls[0]
	if string(call.recipe) != `{"version":1,"mode":"streams"}` {
		t.Errorf("recipe passed to poll = %s", call.recipe)
	}
	if len(call.prev.History) != 1 || len(call.prev.History[0].URLs) != 3 || call.prev.History[0].URLTemplate != "news.example.com/posts/*" {
		t.Errorf("history passed to poll = %+v, want the setup poll", call.prev.History)
	}

	items := storedItems(t, env, id)
	if _, ok := items["https://news.example.com/posts/decoy"]; ok {
		t.Error("an item dated before the oldest one in the feed was published")
	}
	fresh := items["https://news.example.com/posts/fresh-undated"]
	if fresh.history || fresh.firstSeen != "2026-10-10T09:31:00Z" || fresh.published != nil {
		t.Errorf("new undated item = %+v, want a non-history item first seen now", fresh)
	}
	if dated := items["https://news.example.com/posts/fresh-dated"]; dated.history || dated.published == nil || *dated.published != "2026-10-09T08:00:00Z" {
		t.Errorf("new dated item = %+v", dated)
	}
	if third := items["https://news.example.com/posts/third"]; third.title != "Third post" || !third.history {
		t.Errorf("known item changed on a later poll: %+v", third)
	}

	row := readSource(t, env, id)
	if row.health != string(HealthOK) || row.nextPoll != "2026-10-10T10:01:00Z" || row.failures != 0 {
		t.Errorf("source after ok poll = %+v", row)
	}
	if row.etag == nil || *row.etag != `"v2"` || row.hash == nil || *row.hash != "hash-2" {
		t.Errorf("validators after ok poll = etag %v, hash %v", row.etag, row.hash)
	}
	if got := pollCount(t, env, id); got != 2 {
		t.Errorf("good polls = %d, want the ok poll appended", got)
	}

	env.advance(31 * time.Minute)
	pollDue(t, env)
	if prev := env.gen.polls[1].prev; prev.ETag != `"v2"` || prev.LastModified != "Fri, 09 Oct 2026 08:00:00 GMT" || prev.LinkSetHash != "hash-2" || len(prev.History[0].URLs) != 4 {
		t.Errorf("next poll's state = %+v, want the stored validators and newest history first", prev)
	}
}

func TestPollDueSkipsSourcesNobodyFollowsOrNotYetDue(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); err != nil {
		t.Fatal(err)
	}
	env.advance(time.Hour)
	if n := pollDue(t, env); n != 0 || len(env.gen.polls) != 0 {
		t.Errorf("polled %d unfollowed sources", n)
	}

	env2 := newTestEnv(t)
	followed(t, env2)
	env2.advance(10 * time.Minute)
	if n := pollDue(t, env2); n != 0 {
		t.Errorf("polled %d sources before they were due", n)
	}
}

func TestPollDuePollsASourceFollowedOnlyAtItsLegacyURL(t *testing.T) {
	env := newTestEnv(t)
	preview, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, testBase+"/web/"+preview.Source.ID); err != nil {
		t.Fatal(err)
	}
	env.advance(time.Hour)
	if n := pollDue(t, env); n != 1 {
		t.Errorf("polled %d sources, want the one followed at /web/", n)
	}
}

func TestPollDueSuspectPublishesNothing(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{
		Verdict: VerdictSuspect, Reason: "count dropped",
		Items: []Item{{URL: "https://news.example.com/posts/other", Title: "Other"}},
		ETag:  `"v2"`, LinkSetHash: "hash-2",
	}}
	pollDue(t, env)

	if _, ok := storedItems(t, env, id)["https://news.example.com/posts/other"]; ok {
		t.Error("a suspect poll published an item")
	}
	row := readSource(t, env, id)
	if row.health != string(HealthNeedsAttention) || row.reason != "count dropped" {
		t.Errorf("health = %s (%s), want needs_attention", row.health, row.reason)
	}
	if row.etag != nil || row.hash != nil {
		t.Error("a suspect poll stored validators, so the next poll would skip the changed page")
	}
	if got := pollCount(t, env, id); got != 1 {
		t.Errorf("good polls = %d, want a suspect poll kept out of history", got)
	}
}

func TestPollDueBrokenBacksOffDoublingCappedAtADay(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictBroken, Reason: "challenge page", Items: []Item{{URL: "https://news.example.com/posts/x", Title: "X"}}}}

	var gaps []time.Duration
	env.advance(31 * time.Minute)
	for range 7 {
		pollDue(t, env)
		next, err := time.Parse(time.RFC3339, readSource(t, env, id).nextPoll)
		if err != nil {
			t.Fatal(err)
		}
		gaps = append(gaps, next.Sub(env.now))
		env.now = next
	}
	want := []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 16 * time.Hour, 24 * time.Hour, 24 * time.Hour}
	if !slices.Equal(gaps, want) {
		t.Errorf("backoff = %v, want %v", gaps, want)
	}
	row := readSource(t, env, id)
	if row.health != string(HealthBroken) || row.reason != "challenge page" || row.failures != 7 {
		t.Errorf("source = %+v", row)
	}
	if len(storedItems(t, env, id)) != 3 {
		t.Error("a broken poll changed the last good items")
	}

	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK}}
	pollDue(t, env)
	if row := readSource(t, env, id); row.health != string(HealthOK) || row.failures != 0 || row.reason != "" {
		t.Errorf("source after recovery = %+v", row)
	}
}

func TestPollDueTreatsAPollErrorAsBroken(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.pollErr = errors.New("dial tcp: i/o timeout")
	pollDue(t, env)
	row := readSource(t, env, id)
	if row.health != string(HealthBroken) || row.failures != 1 || row.nextPoll != "2026-10-10T10:31:00Z" {
		t.Errorf("source = %+v", row)
	}
}

func TestPollDueKeepsTheLastSixGoodPolls(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	for range 8 {
		env.advance(31 * time.Minute)
		pollDue(t, env)
	}
	if got := pollCount(t, env, id); got != historyPolls {
		t.Errorf("good polls kept = %d, want %d", got, historyPolls)
	}
	if got := len(env.gen.polls[len(env.gen.polls)-1].prev.History); got != historyPolls {
		t.Errorf("history passed to poll = %d, want %d", got, historyPolls)
	}
}

func TestPollDueStoresARepairedRecipe(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Recipe: json.RawMessage(`{"version":1,"mode":"css"}`)}}
	pollDue(t, env)
	if got := readSource(t, env, id).recipe; got != `{"version":1,"mode":"css"}` {
		t.Errorf("recipe = %s", got)
	}
}

func TestPollDueNotModifiedChangesNothingButTheSchedule(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, NotModified: true}}
	pollDue(t, env)
	row := readSource(t, env, id)
	if row.nextPoll != "2026-10-10T10:01:00Z" || row.lastPolled == nil || *row.lastPolled != "2026-10-10T09:31:00Z" {
		t.Errorf("schedule = %+v", row)
	}
	if got := pollCount(t, env, id); got != 1 {
		t.Errorf("good polls = %d, want an unchanged page left out of history", got)
	}
}

func TestPollDueDoesNotRepublishAVariantOfAStoredPost(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Items = append(env.gen.generated.Items, Item{URL: "http://www.news.example.com/posts/first/", Title: "First post"})
	id := followed(t, env)
	if n := len(storedItems(t, env, id)); n != 3 {
		t.Fatalf("setup stored %d items, want 3: a variant URL is the same post", n)
	}
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{Verdict: VerdictOK, Items: []Item{
		{URL: "http://news.example.com/posts/third", Title: "Third post", Date: day(8)},
		{URL: "https://www.news.example.com/posts/second/", Title: "Second post"},
		{URL: "https://news.example.com/posts/fourth", Title: "Fourth post", Date: day(9)},
	}}}
	pollDue(t, env)
	items := storedItems(t, env, id)
	if len(items) != 4 {
		t.Fatalf("stored = %v, want only the fourth post added", items)
	}
	if _, ok := items["https://news.example.com/posts/fourth"]; !ok {
		t.Fatal("the new post was not published")
	}
}

func TestPollDueBrokenPollStaysOutOfHistoryAndKeepsTheRecipe(t *testing.T) {
	env := newTestEnv(t)
	id := followed(t, env)
	before := readSource(t, env, id).recipe
	env.advance(31 * time.Minute)
	env.gen.outcomes = []PollOutcome{{
		Verdict: VerdictBroken, Reason: "the site returned an error",
		Recipe: json.RawMessage(`{"version":1,"mode":"css"}`), Items: []Item{{URL: "https://news.example.com/posts/x", Title: "X"}},
		Fingerprint: Fingerprint{URLTemplate: "news.example.com/posts/*", DateFrac: 1, TitleShape: 1},
	}}
	pollDue(t, env)
	if got := readSource(t, env, id).recipe; got != before {
		t.Errorf("recipe after a broken poll = %s, want %s kept", got, before)
	}
	if got := pollCount(t, env, id); got != 1 {
		t.Errorf("good polls = %d, want the broken poll kept out of history", got)
	}
	env.advance(2 * time.Hour)
	pollDue(t, env)
	if h := env.gen.polls[1].prev.History; len(h) != 1 || len(h[0].URLs) != 3 {
		t.Errorf("next poll's history = %+v, want only the setup's good poll", h)
	}
}

func TestPollDueOnARealSiteThatBreaksKeepsItsRecipeAndHistory(t *testing.T) {
	d := newDriftEnv(t, nil)
	recipe := readSource(t, d.testEnv, d.id).recipe
	d.web.setPage(driftListing, fakePage{status: http.StatusServiceUnavailable, body: "maintenance"})
	d.poll(t)
	row := readSource(t, d.testEnv, d.id)
	if row.health != string(HealthBroken) || row.recipe != recipe || pollCount(t, d.testEnv, d.id) != 1 {
		t.Fatalf("after a 503: health %s, recipe kept %v, good polls %d", row.health, row.recipe == recipe, pollCount(t, d.testEnv, d.id))
	}
	d.web.set(driftListing, driftPage("Blog | Example Publication", driftList(1, 10, "blog")))
	d.advance(2 * time.Hour)
	d.poll(t)
	if row := readSource(t, d.testEnv, d.id); row.health != string(HealthOK) || pollCount(t, d.testEnv, d.id) != 2 {
		t.Fatalf("after recovery: health %s, good polls %d", row.health, pollCount(t, d.testEnv, d.id))
	}
	if got := d.served(t); !slices.Contains(got, driftURL(1, "blog")) {
		t.Errorf("served = %v, want the post published after recovery", got)
	}
}

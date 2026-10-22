package websource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const otherReader = "did:plc:otherreader"

// unsureSource is a source whose setup doubted its list, followed by each of followers.
func unsureSource(t *testing.T, env *testEnv, followers ...string) Preview {
	t.Helper()
	env.gen.generated.Health = HealthNeedsAttention
	env.gen.generated.HealthReason = unsureReason
	first, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, first.Source.FeedURL); err != nil {
		t.Fatal(err)
	}
	for i, did := range followers {
		if _, err := env.writer.Exec(`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, created_at, updated_at) VALUES (?, ?, ?, ?, 't', 't')`,
			did, fmt.Sprintf("3example%d", i), "at://"+did+"/app.example.subscription/3example", first.Source.FeedURL); err != nil {
			t.Fatal(err)
		}
	}
	env.advance(time.Hour)
	return first
}

// confirmer stands in for the model's one look at an anchored list.
type confirmer struct {
	calls int
	ctx   context.Context
	reply Confirmation
	err   error
}

func (c *confirmer) confirm(ctx context.Context) (Confirmation, error) {
	c.calls++
	c.ctx = ctx
	return c.reply, c.err
}

func anchoredGenerated(onPage bool, c *confirmer) Generated {
	gen := Generated{
		Recipe: json.RawMessage(`{"version":1,"mode":"css"}`), Title: "Example Publication", SiteURL: "https://news.example.com/", Health: HealthOK,
		Items:        []Item{{URL: "https://news.example.com/related/a", Title: "Related A"}, {URL: "https://news.example.com/related/b", Title: "Related B"}},
		Fingerprint:  Fingerprint{URLTemplate: "news.example.com/related/*"},
		AnchorOnPage: onPage,
	}
	if c != nil {
		gen.Confirm = c.confirm
	}
	return gen
}

// sourceState is everything a reader or the poller could see of a source, so a rejected anchor can prove it moved nothing.
func sourceState(t *testing.T, env *testEnv, id string) string {
	t.Helper()
	var b strings.Builder
	dump := func(query string) {
		rows, err := env.reader.Query(query, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, c := range cols {
				// Offering the source to the anchoring reader is bookkeeping for the sweep, not a change to the source.
				if c == "offered_at" {
					continue
				}
				fmt.Fprintf(&b, "%s=%v ", c, vals[i])
			}
			b.WriteByte('\n')
		}
	}
	dump(`SELECT * FROM web_sources WHERE id = ?`)
	dump(`SELECT * FROM web_source_items WHERE source_id = ? ORDER BY url`)
	dump(`SELECT * FROM web_source_polls WHERE source_id = ? ORDER BY id`)
	dump(`SELECT * FROM web_source_repairs WHERE source_id = ? ORDER BY id`)
	return b.String()
}

func spendRows(t *testing.T, env *testEnv) []string {
	t.Helper()
	rows, err := env.reader.Query(`SELECT kind, usd FROM web_model_spend ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind string
		var usd float64
		if err := rows.Scan(&kind, &usd); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s %v", kind, usd))
	}
	return out
}

func anchoredBy(t *testing.T, env *testEnv, id string) (did, at *string) {
	t.Helper()
	if err := env.reader.QueryRow(`SELECT anchored_by_did, anchored_at FROM web_sources WHERE id = ?`, id).Scan(&did, &at); err != nil {
		t.Fatal(err)
	}
	return did, at
}

func TestAnAnchorOfASharedSourceThatFailsLeavesItAsItWas(t *testing.T) {
	for name, tc := range map[string]struct {
		onPage    bool
		confirm   *confirmer
		overspent bool
		want      AnchorRejection
		calls     int
		spend     []string
	}{
		"post not linked on the page": {onPage: false, confirm: &confirmer{reply: Confirmation{Agreed: true}}, want: AnchorNotOnPage},
		"the model doubts the list": {onPage: true, confirm: &confirmer{reply: Confirmation{Reason: "a related list", USD: 0.0002}},
			want: AnchorNotConfirmed, calls: 1, spend: []string{"anchor_check 0.0002"}},
		"the model call fails": {onPage: true, confirm: &confirmer{reply: Confirmation{USD: 0.0001}, err: errors.New("overloaded")},
			want: AnchorNotConfirmed, calls: 1, spend: []string{"anchor_check 0.0001"}},
		"no model is configured":    {onPage: true, want: AnchorNotConfirmed},
		"the model budget is spent": {onPage: true, confirm: &confirmer{reply: Confirmation{Agreed: true}}, overspent: true, want: AnchorNotConfirmed},
	} {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			first := unsureSource(t, env, otherReader, testDID)
			if tc.overspent {
				if _, err := env.writer.Exec(`INSERT INTO web_model_spend (spent_at, kind, usd) VALUES (?, 'setup', 1)`, formatTime(env.now)); err != nil {
					t.Fatal(err)
				}
			}
			before := sourceState(t, env, first.Source.ID)
			spentBefore := spendRows(t, env)

			env.gen.generated = anchoredGenerated(tc.onPage, tc.confirm)
			got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/related/a")
			if err != nil {
				t.Fatal(err)
			}
			if env.gen.setupCount() != 2 {
				t.Fatalf("setups = %d, want the anchored setup run", env.gen.setupCount())
			}
			if got.AnchorRejected != tc.want {
				t.Errorf("anchor rejected = %q, want %q", got.AnchorRejected, tc.want)
			}
			if got.Source != first.Source || got.ItemCount != first.ItemCount || len(got.Items) != len(first.Items) {
				t.Errorf("preview = %+v, want the unchanged %+v", got, first)
			}
			if after := sourceState(t, env, first.Source.ID); after != before {
				t.Errorf("source changed:\nbefore %s\nafter  %s", before, after)
			}
			if tc.confirm != nil && tc.confirm.calls != tc.calls {
				t.Errorf("model checks = %d, want %d", tc.confirm.calls, tc.calls)
			}
			if spent := spendRows(t, env); fmt.Sprint(spent) != fmt.Sprint(append(spentBefore, tc.spend...)) {
				t.Errorf("spend = %v, want %v then %v", spent, spentBefore, tc.spend)
			}
		})
	}
}

func TestAnAnchorCannotReplaceASourceAnotherReaderFollowsAtItsLegacyURL(t *testing.T) {
	env := newTestEnv(t)
	first := unsureSource(t, env)
	legacy := testBase + "/web/" + first.Source.ID
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := env.writer.Exec(`INSERT INTO user_subscriptions (did, rkey, at_uri, feed_url, created_at, updated_at) VALUES (?, '3example', ?, ?, 't', 't')`,
		otherReader, "at://"+otherReader+"/app.example.subscription/3example", legacy); err != nil {
		t.Fatal(err)
	}
	env.gen.generated = anchoredGenerated(false, &confirmer{reply: Confirmation{Agreed: true}})
	got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/related/a")
	if err != nil {
		t.Fatal(err)
	}
	if got.AnchorRejected != AnchorNotOnPage || got.Source != first.Source {
		t.Errorf("preview = %+v, want the shared source kept with %q", got, AnchorNotOnPage)
	}
}

func TestAConfirmedAnchorReplacesASharedSourceAndRecordsWhoPointed(t *testing.T) {
	env := newTestEnv(t)
	first := unsureSource(t, env, otherReader)
	c := &confirmer{reply: Confirmation{Agreed: true, USD: 0.0002}}
	env.gen.generated = anchoredGenerated(true, c)
	got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/related/a")
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 {
		t.Fatalf("model checks = %d, want 1", c.calls)
	}
	setupDeadline, _ := env.gen.setupCtxs[1].Deadline()
	if deadline, ok := c.ctx.Deadline(); !ok || !deadline.Equal(setupDeadline) {
		t.Errorf("check deadline = %v (%v), want the setup's %v so the answer reaches the reader in time", deadline, ok, setupDeadline)
	}
	if got.AnchorRejected != "" || got.Source.ID != first.Source.ID || got.Source.Health != HealthOK || got.Source.NeedsAnchor {
		t.Errorf("preview = %+v, want the anchored setup in force", got)
	}
	if row := readSource(t, env, first.Source.ID); row.recipe != `{"version":1,"mode":"css"}` {
		t.Errorf("recipe = %s, want the anchored one", row.recipe)
	}
	if spent := spendRows(t, env); fmt.Sprint(spent) != "[anchor_check 0.0002]" {
		t.Errorf("spend = %v", spent)
	}
	did, at := anchoredBy(t, env, first.Source.ID)
	if did == nil || *did != testDID || at == nil || *at != formatTime(env.now) {
		t.Errorf("anchored by %v at %v, want %s at %s", did, at, testDID, formatTime(env.now))
	}
}

func TestAnAnchorOfASourceNoOneElseFollowsAppliesWithoutAModelCheck(t *testing.T) {
	for name, followers := range map[string][]string{
		"nobody follows it":          nil,
		"only the reader follows it": {testDID},
	} {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			first := unsureSource(t, env, followers...)
			c := &confirmer{}
			env.gen.generated = anchoredGenerated(false, c)
			got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/related/a")
			if err != nil {
				t.Fatal(err)
			}
			if c.calls != 0 || got.AnchorRejected != "" || got.Source.Health != HealthOK {
				t.Errorf("model checks = %d, preview = %+v; want the anchored setup applied as before", c.calls, got)
			}
			if did, at := anchoredBy(t, env, first.Source.ID); did == nil || *did != testDID || at == nil {
				t.Errorf("anchored by %v at %v, want %s recorded", did, at, testDID)
			}
		})
	}
}

func TestAnAnchorThatWouldNotReplaceTheSourceSaysSoAndSpendsNoModelCall(t *testing.T) {
	for name, followers := range map[string][]string{
		"followed by others": {otherReader},
		"followed by no one": nil,
	} {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			first := unsureSource(t, env, followers...)
			c := &confirmer{reply: Confirmation{Agreed: true}}
			env.gen.generated = anchoredGenerated(true, c)
			env.gen.generated.Health = HealthNeedsAttention
			got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/related/a")
			if err != nil {
				t.Fatal(err)
			}
			if c.calls != 0 || got.AnchorRejected != AnchorNoBetter || got.Source != first.Source || got.ItemCount != first.ItemCount {
				t.Errorf("model checks = %d, preview = %+v; want the old setup kept and the reader told", c.calls, got)
			}
			if did, _ := anchoredBy(t, env, first.Source.ID); did != nil {
				t.Errorf("a kept setup recorded %s as its anchor", *did)
			}
		})
	}
}

func TestAnUnanchoredSetupThatKeepsTheOldOneRejectsNoAnchor(t *testing.T) {
	env := newTestEnv(t)
	first := unsureSource(t, env, otherReader)
	env.gen.generated.Recipe = json.RawMessage(`{"version":1,"mode":"css"}`)
	got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if env.gen.setupCount() != 2 || got.AnchorRejected != "" || got.Source != first.Source {
		t.Errorf("setups = %d, preview = %+v; want a stale rerun kept without an anchor verdict", env.gen.setupCount(), got)
	}
}

func TestANewSourceRecordsWhoAnchoredIt(t *testing.T) {
	env := newTestEnv(t)
	got, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", "https://news.example.com/posts/third")
	if err != nil {
		t.Fatal(err)
	}
	if did, at := anchoredBy(t, env, got.Source.ID); did == nil || *did != testDID || at == nil || *at != formatTime(env.now) {
		t.Errorf("anchored by %v at %v", did, at)
	}
	plain, err := env.service.Create(context.Background(), testDID, "https://other.example.com/", "")
	if err != nil {
		t.Fatal(err)
	}
	if did, at := anchoredBy(t, env, plain.Source.ID); did != nil || at != nil {
		t.Errorf("an unanchored setup recorded %v at %v", did, at)
	}
}

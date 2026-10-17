package websource

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	homeURL    = "https://news.example.com/"
	sectionURL = "https://news.example.com/blog"
)

// uncheckedRecipe is a homepage recipe stored before setup looked for a section page.
const uncheckedRecipe = `{"version":1,"listing_url":"https://news.example.com/","mode":"streams","setup":{"stages":["heuristic"]}}`

// movedGenerated is a homepage setup that moved to the section page listing all its posts.
func movedGenerated() Generated {
	return Generated{
		ListingURL: sectionURL,
		Recipe: json.RawMessage(`{"version":1,"listing_url":"https://news.example.com/blog","mode":"streams",` +
			`"setup":{"stages":["heuristic","section","heuristic"],"section":{"from":"https://news.example.com/"},"section_checked":true}}`),
		Title:   "Example Publication",
		SiteURL: homeURL,
		Health:  HealthOK,
		Items: []Item{
			{URL: "https://news.example.com/posts/fifth", Title: "Fifth post", Date: day(9)},
			{URL: "https://news.example.com/posts/third", Title: "Third post", Date: day(8)},
			{URL: "https://news.example.com/posts/second", Title: "Second post"},
			{URL: "https://news.example.com/posts/first", Title: "First post", Date: day(6)},
			{URL: "https://news.example.com/posts/older", Title: "Older post"},
		},
		Fingerprint: Fingerprint{URLTemplate: "news.example.com/posts/*", DateFrac: 0.67, TitleShape: 1},
	}
}

// sectionGenerated is a setup of the section page itself.
func sectionGenerated() Generated {
	g := movedGenerated()
	g.ListingURL = sectionURL
	g.Recipe = json.RawMessage(`{"version":1,"listing_url":"https://news.example.com/blog","mode":"streams","setup":{"stages":["heuristic"],"section_checked":true}}`)
	return g
}

type keyedRow struct {
	listingKey, listingURL, recipe string
	setupSeq                       int
}

func readKeyed(t *testing.T, env *testEnv, id string) keyedRow {
	t.Helper()
	var r keyedRow
	if err := env.reader.QueryRow(`SELECT listing_key, listing_url, recipe, setup_seq FROM web_sources WHERE id = ?`, id).
		Scan(&r.listingKey, &r.listingURL, &r.recipe, &r.setupSeq); err != nil {
		t.Fatal(err)
	}
	return r
}

func aliases(t *testing.T, env *testEnv) map[string]string {
	t.Helper()
	rows, err := env.reader.Query(`SELECT listing_key, source_id FROM web_source_aliases`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			t.Fatal(err)
		}
		out[key] = id
	}
	return out
}

func sourceCount(t *testing.T, env *testEnv) int {
	t.Helper()
	var n int
	if err := env.reader.QueryRow(`SELECT COUNT(*) FROM web_sources`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func create(t *testing.T, env *testEnv, did, listing string) Preview {
	t.Helper()
	p, err := env.service.Create(context.Background(), did, listing, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func follow(t *testing.T, env *testEnv, id string) {
	t.Helper()
	if _, err := env.writer.Exec(`INSERT INTO feeds (feed_url, created_at, updated_at) VALUES (?, 't', 't')`, env.service.FeedURL(id)); err != nil {
		t.Fatal(err)
	}
}

func TestCreateKeysAMovedSetupBySectionAndAliasesThePastedPage(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated = movedGenerated()
	first := create(t, env, testDID, homeURL)

	row := readKeyed(t, env, first.Source.ID)
	if row.listingKey != "news.example.com/blog" || row.listingURL != sectionURL || first.Source.ListingURL != sectionURL {
		t.Errorf("row = %+v, preview listing %q; want the source keyed and polled at the section", row, first.Source.ListingURL)
	}
	if got := aliases(t, env); len(got) != 1 || got["news.example.com"] != first.Source.ID {
		t.Errorf("aliases = %v, want the pasted homepage pointing at the source", got)
	}

	for _, paste := range []string{"https://news.example.com/blog/", "https://www.news.example.com/", "http://news.example.com/#top"} {
		if again := create(t, env, "did:plc:other", paste); again.Source.ID != first.Source.ID {
			t.Errorf("%s made %q, want the existing %q", paste, again.Source.ID, first.Source.ID)
		}
	}
	if n := env.gen.setupCount(); n != 1 || sourceCount(t, env) != 1 {
		t.Errorf("setups = %d, sources = %d; want both pastes served by the one source", n, sourceCount(t, env))
	}
}

func TestCreateFromAHomepageReusesTheSectionsSource(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generatedFor = map[string]Generated{sectionURL: sectionGenerated(), homeURL: movedGenerated()}
	section := create(t, env, testDID, sectionURL)
	home := create(t, env, "did:plc:other", homeURL)

	if home.Source.ID != section.Source.ID || sourceCount(t, env) != 1 {
		t.Fatalf("homepage paste made %q (%d sources), want the section's %q", home.Source.ID, sourceCount(t, env), section.Source.ID)
	}
	if got := aliases(t, env); len(got) != 1 || got["news.example.com"] != section.Source.ID {
		t.Errorf("aliases = %v, want the homepage aliased to the section's source", got)
	}
	env.advance(time.Hour)
	if again := create(t, env, "did:plc:third", "https://news.example.com"); again.Source.ID != section.Source.ID || env.gen.setupCount() != 2 {
		t.Errorf("a third paste made %q after %d setups, want the alias to answer without a setup", again.Source.ID, env.gen.setupCount())
	}
	var offered string
	if err := env.reader.QueryRow(`SELECT offered_at FROM web_sources WHERE id = ?`, section.Source.ID).Scan(&offered); err != nil {
		t.Fatal(err)
	}
	if want := formatTime(env.now.UTC()); offered != want {
		t.Errorf("offered_at = %q, want %q: a source offered through its alias must stay out of the sweep", offered, want)
	}
}

func TestRegenerateMovesAHomepageSourceToItsSection(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated.Health = HealthNeedsAttention
	first := create(t, env, testDID, homeURL)

	env.advance(staleSetup + time.Minute)
	env.gen.generated = movedGenerated()
	again := create(t, env, testDID, homeURL)

	if again.Source.ID != first.Source.ID || sourceCount(t, env) != 1 {
		t.Fatalf("regenerate made %q, want %q kept", again.Source.ID, first.Source.ID)
	}
	row := readKeyed(t, env, first.Source.ID)
	if row.listingKey != "news.example.com/blog" || row.listingURL != sectionURL || row.setupSeq != 1 {
		t.Errorf("row = %+v, want it rekeyed to the section with the new setup", row)
	}
	if got := aliases(t, env); len(got) != 1 || got["news.example.com"] != first.Source.ID {
		t.Errorf("aliases = %v, want the old key aliased", got)
	}
	items := storedItems(t, env, first.Source.ID)
	if len(items) != 5 || !items["https://news.example.com/posts/fifth"].history || !items["https://news.example.com/posts/second"].history {
		t.Errorf("items = %v, want the old items kept and the section's joined as history", items)
	}
	if again := create(t, env, "did:plc:other", sectionURL); again.Source.ID != first.Source.ID {
		t.Errorf("section paste made %q, want %q", again.Source.ID, first.Source.ID)
	}
}

func TestRegenerateKeepsItsKeyWhenAnotherSourceOwnsTheSection(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generatedFor = map[string]Generated{sectionURL: sectionGenerated()}
	section := create(t, env, testDID, sectionURL)
	env.gen.generated.Health = HealthNeedsAttention
	home := create(t, env, testDID, homeURL)
	if home.Source.ID == section.Source.ID {
		t.Fatal("precondition: the unsure homepage setup should be a source of its own")
	}

	env.advance(staleSetup + time.Minute)
	env.gen.generatedFor[homeURL] = movedGenerated()
	again, err := env.service.Create(context.Background(), testDID, homeURL, "")
	if err != nil {
		t.Fatal(err)
	}
	if again.Source.ID != home.Source.ID || sourceCount(t, env) != 2 {
		t.Fatalf("regenerate made %q (%d sources), want both sources kept", again.Source.ID, sourceCount(t, env))
	}
	row := readKeyed(t, env, home.Source.ID)
	if row.listingKey != "news.example.com" || row.listingURL != sectionURL || row.setupSeq != 1 {
		t.Errorf("homepage row = %+v, want its key kept and the new setup stored", row)
	}
	if got := readKeyed(t, env, section.Source.ID); got.listingKey != "news.example.com/blog" || got.setupSeq != 0 {
		t.Errorf("section row = %+v, want it untouched", got)
	}
	if got := aliases(t, env); len(got) != 0 {
		t.Errorf("aliases = %v, want none", got)
	}
}

// uncheckedSource is a followed homepage source stored before setup looked for a section page.
func uncheckedSource(t *testing.T, env *testEnv, listing string) string {
	t.Helper()
	gen := exampleGenerated()
	gen.Recipe = json.RawMessage(strings.ReplaceAll(uncheckedRecipe, homeURL, listing))
	env.gen.generatedFor = map[string]Generated{listing: gen}
	id := create(t, env, testDID, listing).Source.ID
	follow(t, env, id)
	env.gen.generatedFor = nil
	return id
}

func setupsOf(env *testEnv) []string {
	env.gen.mu.Lock()
	defer env.gen.mu.Unlock()
	return slices.Clone(env.gen.setups)
}

func TestPollDueMovesAnUncheckedHomepageSourceToItsSectionOnce(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.generated = movedGenerated()
	env.advance(31 * time.Minute)

	if n := pollDue(t, env); n != 1 || len(env.gen.polls) != 1 {
		t.Fatalf("polled %d sources with %d polls, want the source polled", n, len(env.gen.polls))
	}
	if got := setupsOf(env); !slices.Equal(got, []string{homeURL, homeURL}) || env.gen.allowModel[1] {
		t.Fatalf("setups = %v, allow model %v; want one free check of the homepage", got, env.gen.allowModel)
	}
	row := readKeyed(t, env, id)
	if row.listingKey != "news.example.com/blog" || row.listingURL != sectionURL || row.setupSeq != 1 {
		t.Errorf("row = %+v, want the section setup in force under the same id", row)
	}
	if got := aliases(t, env); got["news.example.com"] != id {
		t.Errorf("aliases = %v, want the homepage aliased", got)
	}
	items := storedItems(t, env, id)
	if len(items) != 5 || !items["https://news.example.com/posts/fifth"].history || !items["https://news.example.com/posts/older"].history {
		t.Errorf("items = %v, want the old items kept and the section's added as history", items)
	}
	if got := pollCount(t, env, id); got != 1 {
		t.Errorf("good polls = %d, want only the section setup's", got)
	}

	env.advance(31 * time.Minute)
	pollDue(t, env)
	if got := setupsOf(env); len(got) != 2 || len(env.gen.polls) != 2 {
		t.Errorf("setups = %v after a second sweep with %d polls, want no second check", got, len(env.gen.polls))
	}
}

func TestPollDueMarksASourceWhoseSectionCheckStays(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.generated.Recipe = json.RawMessage(`{"version":1,"mode":"streams","setup":{"stages":["replacement"],"section_checked":true}}`)
	env.advance(31 * time.Minute)
	pollDue(t, env)

	if got := setupsOf(env); len(got) != 2 {
		t.Fatalf("setups = %v, want one check", got)
	}
	row := readKeyed(t, env, id)
	var recipe struct {
		ListingURL string `json:"listing_url"`
		Setup      struct {
			Stages         []string `json:"stages"`
			SectionChecked bool     `json:"section_checked"`
		} `json:"setup"`
	}
	if err := json.Unmarshal([]byte(row.recipe), &recipe); err != nil {
		t.Fatal(err)
	}
	if !recipe.Setup.SectionChecked || !slices.Equal(recipe.Setup.Stages, []string{"heuristic"}) || recipe.ListingURL != homeURL {
		t.Errorf("recipe = %s, want the stored one marked checked and otherwise kept", row.recipe)
	}
	if row.listingKey != "news.example.com" || row.setupSeq != 0 || len(aliases(t, env)) != 0 || len(storedItems(t, env, id)) != 3 {
		t.Errorf("row = %+v, want the source as it was", row)
	}

	env.advance(31 * time.Minute)
	pollDue(t, env)
	if got := setupsOf(env); len(got) != 2 || len(env.gen.polls) != 2 {
		t.Errorf("setups = %v after a second sweep with %d polls, want no second check", got, len(env.gen.polls))
	}
}

func TestPollDueMarksAMoveThatIsNotHealthy(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.generated = movedGenerated()
	env.gen.generated.Health = HealthNeedsAttention
	env.advance(31 * time.Minute)
	pollDue(t, env)

	row := readKeyed(t, env, id)
	if row.listingKey != "news.example.com" || row.setupSeq != 0 || !strings.Contains(row.recipe, `"section_checked":true`) {
		t.Errorf("row = %+v, want the source kept and marked checked", row)
	}
}

func TestPollDueSectionCheckNeverOverwritesANewerSetup(t *testing.T) {
	for _, tc := range []struct {
		name string
		gen  func() Generated
	}{
		{"stays", exampleGenerated},
		{"moved", movedGenerated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			id := uncheckedSource(t, env, homeURL)
			env.gen.generated = tc.gen()
			env.advance(31 * time.Minute)
			env.gen.beforeSetup = func(context.Context) {
				if _, err := env.writer.Exec(`UPDATE web_sources SET setup_seq = setup_seq + 1, recipe = '{"newer":true}' WHERE id = ?`, id); err != nil {
					t.Error(err)
				}
			}
			pollDue(t, env)

			if row := readKeyed(t, env, id); row.recipe != `{"newer":true}` || row.listingKey != "news.example.com" || row.setupSeq != 1 {
				t.Errorf("row = %+v, want the newer setup left alone", row)
			}
		})
	}
}

func TestPollDueMarksTheSectionCheckOfAFailingSite(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.setupErr = fmt.Errorf("%w: connection refused", ErrUnreachable)
	env.advance(31 * time.Minute)

	if n := pollDue(t, env); n != 1 || len(env.gen.polls) != 1 {
		t.Fatalf("polled %d with %d polls, want the source polled despite the failed check", n, len(env.gen.polls))
	}
	if row := readKeyed(t, env, id); !strings.Contains(row.recipe, `"section_checked":true`) {
		t.Errorf("recipe = %s, want a failed check marked so it never holds a setup slot again", row.recipe)
	}
	env.gen.setupErr = nil
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if got := setupsOf(env); len(got) != 2 {
		t.Errorf("setups = %v, want no second check", got)
	}
}

func TestPollDueKeepsASourceWhoseSectionDropsAPostReadersSee(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.generated = movedGenerated()
	env.gen.generated.Items = slices.DeleteFunc(env.gen.generated.Items, func(it Item) bool { return strings.HasSuffix(it.URL, "/second") })
	env.advance(31 * time.Minute)
	pollDue(t, env)

	row := readKeyed(t, env, id)
	if row.listingKey != "news.example.com" || row.setupSeq != 0 || !strings.Contains(row.recipe, `"section_checked":true`) {
		t.Errorf("row = %+v, want the source kept on its list and marked checked", row)
	}
}

func TestCreateNeverKeysASourceByAnotherSitesPage(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated = movedGenerated()
	env.gen.generated.ListingURL = "https://other.example.org/blog"

	id := create(t, env, testDID, homeURL).Source.ID
	if row := readKeyed(t, env, id); row.listingKey != "news.example.com" || row.listingURL != homeURL {
		t.Errorf("row = %+v, want the pasted site's key", row)
	}
	if got := aliases(t, env); len(got) != 0 {
		t.Errorf("aliases = %v, want none", got)
	}
}

func TestRegenerateMovesASectionSourceBackToItsHomepage(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated = movedGenerated()
	env.gen.generated.Health = HealthNeedsAttention
	id := create(t, env, testDID, homeURL).Source.ID
	env.advance(staleSetup + time.Minute)
	env.gen.generated = exampleGenerated()

	if got := create(t, env, testDID, homeURL).Source.ID; got != id {
		t.Fatalf("regenerated into %s, want %s", got, id)
	}
	if row := readKeyed(t, env, id); row.listingKey != "news.example.com" || row.listingURL != homeURL {
		t.Errorf("row = %+v, want it keyed by the homepage again", row)
	}
	if got := aliases(t, env); len(got) != 1 || got["news.example.com/blog"] != id {
		t.Errorf("aliases = %v, want only the section aliased", got)
	}
	if got := create(t, env, testDID, sectionURL).Source.ID; got != id {
		t.Errorf("section paste found %s, want %s", got, id)
	}
}

func TestPollDueNeverSectionChecksAnchoredOrSitemapSources(t *testing.T) {
	for _, tc := range []struct{ name, recipe, anchor string }{
		{"anchor in recipe", `{"version":1,"mode":"css","setup":{"anchor":"https://news.example.com/posts/third"}}`, ""},
		{"anchored row", `{"version":1,"mode":"css","setup":{}}`, "https://news.example.com/posts/third"},
		{"sitemap", `{"version":1,"mode":"sitemap","setup":{}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.gen.generated.Recipe = json.RawMessage(tc.recipe)
			p, err := env.service.Create(context.Background(), testDID, homeURL, tc.anchor)
			if err != nil {
				t.Fatal(err)
			}
			follow(t, env, p.Source.ID)
			env.advance(31 * time.Minute)
			pollDue(t, env)
			if got := setupsOf(env); len(got) != 1 || len(env.gen.polls) != 1 {
				t.Errorf("setups = %v with %d polls, want the source polled and never checked", got, len(env.gen.polls))
			}
		})
	}
}

func TestPollDueSectionChecksAFewSourcesPerSweep(t *testing.T) {
	env := newTestEnv(t)
	for i := range sectionChecksPerSweep + 2 {
		uncheckedSource(t, env, fmt.Sprintf("https://site%d.example.com/", i))
	}
	created := env.gen.setupCount()
	env.advance(31 * time.Minute)

	if n := pollDue(t, env); n != sectionChecksPerSweep+2 {
		t.Fatalf("polled %d, want every due source polled", n)
	}
	if got := env.gen.setupCount() - created; got != sectionChecksPerSweep {
		t.Errorf("checks = %d in one sweep, want %d", got, sectionChecksPerSweep)
	}
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if got := env.gen.setupCount() - created; got != sectionChecksPerSweep+2 {
		t.Errorf("checks = %d after two sweeps, want the rest checked", got)
	}
}

func TestPollDueLeavesTheCheckForALaterSweepWhenReadersHoldTheSlots(t *testing.T) {
	env := newTestEnv(t)
	id := uncheckedSource(t, env, homeURL)
	env.gen.generated = movedGenerated()
	env.advance(31 * time.Minute)
	env.service.setupTimeout = 200 * time.Millisecond
	for range setupSlots {
		env.service.slots <- struct{}{}
	}
	// Readers finish just before the check's deadline, so a check that waited would start a setup it cannot finish.
	freed := make(chan struct{})
	time.AfterFunc(150*time.Millisecond, func() {
		for range setupSlots {
			<-env.service.slots
		}
		close(freed)
	})
	env.gen.beforeSetup = func(ctx context.Context) { <-ctx.Done() }
	env.gen.setupErr = context.DeadlineExceeded
	pollDue(t, env)
	<-freed

	env.gen.beforeSetup, env.gen.setupErr = nil, nil
	if row := readKeyed(t, env, id); strings.Contains(row.recipe, `"section_checked":true`) {
		t.Fatalf("row = %+v, want the check left for a later sweep", row)
	}
	env.advance(31 * time.Minute)
	pollDue(t, env)
	if row := readKeyed(t, env, id); row.listingKey != "news.example.com/blog" {
		t.Errorf("row = %+v, want the source moved once the slots were free", row)
	}
}

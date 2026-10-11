package webfeed

import (
	"context"
	"testing"
)

func TestDoubtedRepairAcceptsAHealthyInducedRebuild(t *testing.T) {
	s := newTestSite(t)
	s.addBlog(8)
	res := setupRecipe(t, s, "/blog")
	tonedBlog(s)
	body, err := s.get(t, "/blog")
	if err != nil {
		t.Fatal(err)
	}
	polled := doubtedPoll(t, s, res.Recipe, string(body))
	opts := s.opts()
	opts.Model = inducer(t, tonedRecipe)
	got, err := Repair(context.Background(), polled, RepairInput{Recipe: res.Recipe, History: historyFrom(res), Doubted: true, AllowModel: true}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Accepted || got.Step != RepairJudge || got.Rule != AcceptDifferent || got.Recipe.Mode != ModeCSS || len(got.Items) != len(tones) {
		t.Fatalf("repair accepted %v step %q rule %q mode %s items %v (tried %v), want the induced list to close the doubt",
			got.Accepted, got.Step, got.Rule, got.Recipe.Mode, itemURLs(got.Items), got.Tried)
	}
	if want := []string{s.host() + "/blog/*/*"}; len(got.Recipe.Fingerprint.URLTemplates) != 1 || got.Recipe.Fingerprint.URLTemplates[0] != want[0] {
		t.Errorf("fingerprint templates %v, want the induced recipe's %v kept", got.Recipe.Fingerprint.URLTemplates, want)
	}
}

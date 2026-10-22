package webfeed

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSetupAnchoredReportsWhetherThePageLinksThePost(t *testing.T) {
	s := newTestSite(t)
	s.set("/resources", ambiguousPage())
	onPage := "/stories/" + slugify(postTitles[6])
	for name, tc := range map[string]struct {
		anchor string
		want   bool
	}{
		"linked":                     {s.url(onPage), true},
		"linked, tracking and slash": {s.url(onPage + "/?utm_source=newsletter"), true},
		"same shape, not linked":     {s.url("/stories/a-post-this-page-never-links"), false},
		"the listing itself":         {s.url("/resources"), false},
		"a section it does not link": {s.url("/stories"), false},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := SetupAnchored(context.Background(), s.url("/resources"), tc.anchor, s.opts())
			if err != nil {
				t.Fatal(err)
			}
			if res.AnchorOnPage != tc.want {
				t.Errorf("anchor on page = %v, want %v", res.AnchorOnPage, tc.want)
			}
		})
	}
}

func TestSetupAnchoredOnARefusedListingFindsNoAnchor(t *testing.T) {
	s := newTestSite(t)
	s.set("/resources", ambiguousPage())
	s.set("/robots.txt", "User-agent: *\nDisallow: /resources\n")
	res, err := SetupAnchored(context.Background(), s.url("/resources"), s.url("/stories/"+slugify(postTitles[6])), s.opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.AnchorOnPage {
		t.Error("a listing we never read cannot vouch for the anchor")
	}
	if _, err := ConfirmAnchored(context.Background(), &fakeModel{}, res); err == nil {
		t.Error("confirming without the page = nil error, want one")
	}
}

func TestConfirmAnchoredAsksTheWeeklyCheckOnce(t *testing.T) {
	s := newTestSite(t)
	s.set("/resources", ambiguousPage())
	anchor := s.url("/stories/" + slugify(postTitles[6]))
	res, err := SetupAnchored(context.Background(), s.url("/resources"), anchor, s.opts())
	if err != nil {
		t.Fatal(err)
	}

	m := &fakeModel{replies: map[Effort][]string{EffortLow: {`{"verdict":"healthy","reason":"stories"}`}}}
	rec, err := ConfirmAnchored(context.Background(), m, res)
	if err != nil {
		t.Fatal(err)
	}
	calls := m.calls(EffortLow)
	if len(m.prompts) != 1 || len(calls) != 1 || calls[0].System != watchPrompt {
		t.Fatalf("prompts = %+v, want one weekly-check call at low effort", m.prompts)
	}
	user := calls[0].User
	if !strings.HasPrefix(user, "Page: "+s.url("/resources")+"\nTitle: Resources") || !strings.Contains(user, "Last good items (0):") ||
		!strings.Contains(user, "Current items (5):") || !strings.Contains(user, postTitles[6]) {
		t.Errorf("user message = %q, want the page digest and the anchored items with no trusted reference", user)
	}
	if rec.Broken || rec.USD != 0.00021 || rec.InputTokens != 1500 {
		t.Errorf("record = %+v", rec)
	}

	m = &fakeModel{replies: map[Effort][]string{EffortLow: {`{"verdict":"broken","reason":"a related list"}`}}}
	if rec, err := ConfirmAnchored(context.Background(), m, res); err != nil || !rec.Broken || rec.Reason != "a related list" {
		t.Errorf("broken verdict = %+v, %v", rec, err)
	}
	m = &fakeModel{errs: map[Effort][]error{EffortLow: {errors.New("overloaded")}}}
	if rec, err := ConfirmAnchored(context.Background(), m, res); err == nil || rec.USD != 0.00021 {
		t.Errorf("failed call = %+v, %v; want the error and its cost", rec, err)
	}
}

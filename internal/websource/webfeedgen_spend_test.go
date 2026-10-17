package websource

import (
	"context"
	"testing"

	"morgenblau/internal/webfeed"
)

// deadlineJudge answers the judge but lets the setup's deadline pass while it does, as a slow model would.
type deadlineJudge struct{ cancel context.CancelFunc }

func (m deadlineJudge) Complete(context.Context, webfeed.Prompt) (string, webfeed.Usage, error) {
	m.cancel()
	return `{"pick":[0],"order":"date","none":false}`, webfeed.Usage{InputTokens: 2500, OutputTokens: 600, USD: 0.00054}, nil
}

func TestWebfeedGeneratorSetupCutByItsDeadlineReportsWhatItPaid(t *testing.T) {
	web := driftSite()
	web.set(driftListing, twoListsPage())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen := NewWebfeedGenerator(webfeed.Options{Client: web.client(), Model: deadlineJudge{cancel: cancel}})
	got, err := gen.Setup(ctx, driftListing, "", true)
	if err == nil {
		t.Fatal("want the cut setup to fail")
	}
	if got.Cost.JudgeCalls != 3 || got.Cost.USD <= 0 {
		t.Errorf("cost = %+v, want the three judge calls the setup paid for", got.Cost)
	}
}

func TestSetupCostCountsTheJudgeOfAPageASectionMoveLeftBehind(t *testing.T) {
	got := setupCost(webfeed.SetupRecord{
		Stages:  []string{"heuristic", "verify", "judge", "section", "heuristic", "verify"},
		Section: &webfeed.SectionRecord{From: "https://www.example.com/", Judge: &webfeed.JudgeRecord{Calls: 3, InputTokens: 7500, OutputTokens: 1800, USD: 0.00162}},
	})
	if got.JudgeCalls != 3 || got.InputTokens != 7500 || got.OutputTokens != 1800 || got.USD != 0.00162 {
		t.Fatalf("cost = %+v, want the left-behind judge's three calls", got)
	}
}

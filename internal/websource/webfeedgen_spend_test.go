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

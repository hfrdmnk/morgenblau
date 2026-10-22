package websource

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollRecordsTheSpendOfARepairCutShortByShutdown(t *testing.T) {
	env := newTestEnv(t)
	followed(t, env)
	env.advance(31 * time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	env.gen.duringPoll = cancel
	env.gen.outcomes = []PollOutcome{{Repair: &RepairReport{Cause: RepairCauseDetector, Outcome: RepairUnresolved, Step: "judge", UsedModel: true, USD: 0.003}}}
	env.gen.pollErr = context.Canceled

	if _, err := env.service.PollDue(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("PollDue err = %v, want context.Canceled", err)
	}
	if got := modelSpendUSD(t, env); got != 0.003 {
		t.Errorf("recorded spend = %v, want the 0.003 the cut-short repair paid", got)
	}
}

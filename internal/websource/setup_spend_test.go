package websource

import (
	"context"
	"errors"
	"testing"
)

func modelSpendUSD(t *testing.T, env *testEnv) float64 {
	t.Helper()
	var usd float64
	if err := env.reader.QueryRow(`SELECT COALESCE(SUM(usd), 0) FROM web_model_spend`).Scan(&usd); err != nil {
		t.Fatal(err)
	}
	return usd
}

func TestCreateRecordsTheSpendOfAFailedSetup(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated = Generated{Cost: Cost{JudgeCalls: 3, USD: 0.25}}
	env.gen.setupErr = errors.New("induce reply cut off")
	if _, err := env.service.Create(context.Background(), testDID, "https://news.example.com/", ""); !errors.Is(err, ErrGeneration) {
		t.Fatalf("err = %v, want ErrGeneration", err)
	}
	if got := modelSpendUSD(t, env); got != 0.25 {
		t.Errorf("model spend = %v, want the failed setup's 0.25", got)
	}
}

func TestCreateRecordsTheSpendOfASetupCutByItsDeadline(t *testing.T) {
	env := newTestEnv(t)
	env.gen.generated = Generated{Cost: Cost{JudgeCalls: 3, USD: 0.4}}
	ctx, cancel := context.WithCancel(context.Background())
	env.gen.beforeSetup = func(context.Context) { cancel() }
	env.gen.setupErr = context.Canceled
	if _, err := env.service.Create(ctx, testDID, "https://news.example.com/", ""); err == nil {
		t.Fatal("want the cut setup to fail")
	}
	if got := modelSpendUSD(t, env); got != 0.4 {
		t.Errorf("model spend = %v, want the cut setup's 0.4", got)
	}
}

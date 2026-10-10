package server

import (
	"net/http"
	"testing"

	"morgenblau/internal/websource"
)

func TestWebGeneratorAlwaysGeneratesAndUsesTheModelOnlyWithAKey(t *testing.T) {
	gen, model, err := newWebGenerator(http.DefaultClient, "")
	if err != nil || gen == nil || model {
		t.Fatalf("without a key: generator %v, model %v, err %v", gen, model, err)
	}
	gen, model, err = newWebGenerator(http.DefaultClient, "sk-ant-example")
	if err != nil || gen == nil || !model {
		t.Fatalf("with a key: generator %v, model %v, err %v", gen, model, err)
	}
}

func TestModelCallsFitTheSetupDeadline(t *testing.T) {
	// One round of parallel judge votes plus two induction rounds run one after another inside a setup.
	if worst := 3 * modelCallTimeout; worst >= websource.SetupTimeout {
		t.Errorf("model calls can take %v, want well under the %v setup deadline", worst, websource.SetupTimeout)
	}
}

func TestWebModelDailyUSD(t *testing.T) {
	for raw, want := range map[string]float64{"": 1, "2.50": 2.5, "0": 0} {
		got, err := webModelDailyUSD(func(string) string { return raw })
		if err != nil || got != want {
			t.Errorf("WEB_MODEL_DAILY_USD=%q: %v, %v; want %v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"-1", "lots", "NaN", "Inf"} {
		if _, err := webModelDailyUSD(func(string) string { return raw }); err == nil {
			t.Errorf("WEB_MODEL_DAILY_USD=%q accepted", raw)
		}
	}
}

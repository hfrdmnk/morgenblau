package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"morgenblau/internal/claude"
	"morgenblau/internal/websource"
)

func TestWebGeneratorAlwaysGeneratesAndUsesTheModelOnlyWithAKey(t *testing.T) {
	gen, model, err := newWebGenerator(http.DefaultClient, claude.Config{})
	if err != nil || gen == nil || model {
		t.Fatalf("without a key: generator %v, model %v, err %v", gen, model, err)
	}
	gen, model, err = newWebGenerator(http.DefaultClient, claude.Config{APIKey: "sk-ant-example"})
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

func TestEnvInt(t *testing.T) {
	env := map[string]string{"SET": "45", "ZERO": "0", "BAD": "soon"}
	getenv := func(k string) string { return env[k] }
	for name, want := range map[string]int{"SET": 45, "ZERO": 0, "UNSET": 30} {
		if got, err := envInt(getenv, name, 30); err != nil || got != want {
			t.Errorf("envInt(%s) = %d, %v; want %d", name, got, err, want)
		}
	}
	if _, err := envInt(getenv, "BAD", 30); err == nil || !strings.Contains(err.Error(), "BAD") {
		t.Errorf("envInt(BAD) err = %v, want one naming the setting", err)
	}
}

// fakeMessagesAPI answers the Messages API by effort as each webfeed step expects, counting the calls per effort.
type fakeMessagesAPI struct {
	mu    sync.Mutex
	calls map[string]int
}

func (f *fakeMessagesAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.calls[body.OutputConfig.Effort]++
	f.mu.Unlock()
	reply := map[string]string{
		"high":   `{"pick":[0],"order":"date","none":false,"reason":"posts"}`,
		"medium": `{"item":"article.post","link":"h2 a","title":"h2","date":"time","reason":"cards"}`,
		"low":    `{"verdict":"healthy","reason":"posts"}`,
	}[body.OutputConfig.Effort]
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-haiku-5-5",
		"content":     []map[string]any{{"type": "text", "text": reply}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 100, "output_tokens": 10},
	})
}

func (f *fakeMessagesAPI) count(effort string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[effort]
}

// scatteredListing defeats the extractor, so setup both asks the judge and induces a recipe: each post link has its
// own class and category path.
func scatteredListing() string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><title>Blog | Example Publication</title></head><body><main><div class="feed">`)
	for i, tone := range []string{"ocean", "forest", "desert", "glacier", "meadow", "canyon"} {
		fmt.Fprintf(&b, `<article class="post"><h2><a class="tone-%s" href="/blog/%s/story-number-%d">Story number %d from the example desk</a></h2><time datetime="2026-09-%02d"></time></article>`, tone, tone, i, i, 20-i)
	}
	b.WriteString(`</div></main></body></html>`)
	return b.String()
}

func TestWebGeneratorWiresEveryModelStepToClaude(t *testing.T) {
	api := &fakeMessagesAPI{calls: map[string]int{}}
	apiSrv := httptest.NewServer(api)
	t.Cleanup(apiSrv.Close)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blog" {
			_, _ = io.WriteString(w, scatteredListing())
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(site.Close)

	gen, model, err := newWebGenerator(site.Client(), claude.Config{APIKey: "sk-ant-example", BaseURL: apiSrv.URL})
	if err != nil || !model {
		t.Fatalf("generator: model %v, err %v", model, err)
	}
	setup, err := gen.Setup(context.Background(), site.URL+"/blog", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if api.count("high") != 3 || api.count("medium") == 0 || setup.Cost.USD == 0 {
		t.Fatalf("judge calls %d, induce calls %d, cost %+v; want the judge's three votes and induction through Claude",
			api.count("high"), api.count("medium"), setup.Cost)
	}
	if _, err := gen.Poll(context.Background(), setup.Recipe, websource.PollState{CheckDue: true}); err != nil {
		t.Fatal(err)
	}
	if api.count("low") != 1 {
		t.Fatalf("weekly check calls = %d, want 1", api.count("low"))
	}
}

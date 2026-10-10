package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"morgenblau/internal/webfeed"
)

// fakeAPI stands in for the Messages API: it records each request body and answers with a canned reply.
type fakeAPI struct {
	mu       sync.Mutex
	bodies   []map[string]any
	headers  []http.Header
	paths    []string
	status   int
	reply    string
	stop     string
	usageIn  int
	usageOut int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.headers = append(f.headers, r.Header.Clone())
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"temperature is not supported"}}`)
		return
	}
	stop := f.stop
	if stop == "" {
		stop = "end_turn"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": Model,
		"content": []map[string]any{
			{"type": "thinking", "thinking": "", "signature": "sig"},
			{"type": "text", "text": f.reply},
		},
		"stop_reason": stop,
		"usage":       map[string]any{"input_tokens": f.usageIn, "output_tokens": f.usageOut},
	})
}

func (f *fakeAPI) last(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.bodies) == 0 {
		t.Fatal("no request reached the API")
	}
	return f.bodies[len(f.bodies)-1]
}

func newTestClient(t *testing.T, api *fakeAPI) *Client {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	c, err := New(Config{APIKey: "test-key", HTTPClient: srv.Client(), BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertRequestShape(t *testing.T, body map[string]any, effort, system, user string) {
	t.Helper()
	if body["model"] != "claude-haiku-5-5" {
		t.Errorf("model = %v", body["model"])
	}
	if !reflect.DeepEqual(body["thinking"], map[string]any{"type": "adaptive"}) {
		t.Errorf("thinking = %v", body["thinking"])
	}
	if !reflect.DeepEqual(body["output_config"], map[string]any{"effort": effort}) {
		t.Errorf("output_config = %v", body["output_config"])
	}
	for _, banned := range []string{"temperature", "top_p", "top_k"} {
		if _, ok := body[banned]; ok {
			t.Errorf("request sets %s, which Haiku 5.5 rejects", banned)
		}
	}
	blocks, _ := body["system"].([]any)
	if len(blocks) != 1 || blocks[0].(map[string]any)["text"] != system {
		t.Errorf("system = %v", body["system"])
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v; want one user turn and no assistant prefill", body["messages"])
	}
	m := msgs[0].(map[string]any)
	content, _ := m["content"].([]any)
	if m["role"] != "user" || len(content) != 1 || content[0].(map[string]any)["text"] != user {
		t.Errorf("message = %v", m)
	}
}

func TestJudgeSendsTheContractRequestAndParsesTheVerdict(t *testing.T) {
	api := &fakeAPI{reply: `{"pick":[1],"order":"date","mixed":false,"none":false,"reason":"posts"}`, usageIn: 2500, usageOut: 600}
	c := newTestClient(t, api)
	in := webfeed.JudgeInput{System: webfeed.SystemPrompt, User: "Page: https://news.example.com/\nTitle: News\n", Candidates: 3}
	v, u, err := c.Judge(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Pick, []int{1}) || v.Order != webfeed.OrderDate || v.Reason != "posts" {
		t.Fatalf("verdict = %+v", v)
	}
	if u.InputTokens != 2500 || u.OutputTokens != 600 || math.Abs(u.USD-0.00055) > 1e-12 {
		t.Fatalf("usage = %+v", u)
	}
	assertRequestShape(t, api.last(t), "high", webfeed.SystemPrompt, in.User)
	if api.paths[0] != "/v1/messages" || api.headers[0].Get("X-Api-Key") != "test-key" {
		t.Fatalf("path %s, key header %q", api.paths[0], api.headers[0].Get("X-Api-Key"))
	}
}

func TestUnparsableReplyStillReportsUsage(t *testing.T) {
	api := &fakeAPI{reply: "I think the second list.", usageIn: 100, usageOut: 20}
	c := newTestClient(t, api)
	_, u, err := c.Judge(context.Background(), webfeed.JudgeInput{System: webfeed.SystemPrompt, User: "Page: x", Candidates: 2})
	if err == nil {
		t.Fatal("prose reply parsed as a verdict")
	}
	if u.InputTokens != 100 || u.OutputTokens != 20 || u.USD == 0 {
		t.Fatalf("usage = %+v; tokens spent must be metered even when the answer is unusable", u)
	}
}

func TestAPIErrorsAndRefusalsBubbleUp(t *testing.T) {
	for name, api := range map[string]*fakeAPI{
		"400":        {status: http.StatusBadRequest},
		"refusal":    {reply: "", stop: "refusal"},
		"max_tokens": {reply: `{"pick":[0`, stop: "max_tokens"},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, api)
			if _, _, err := c.Judge(context.Background(), webfeed.JudgeInput{System: "s", User: "u", Candidates: 1}); err == nil {
				t.Fatal("error swallowed")
			}
		})
	}
}

func TestEachCallIsBoundedInTime(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	c, err := New(Config{APIKey: "test-key", HTTPClient: srv.Client(), BaseURL: srv.URL, CallTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, _, err := c.Judge(context.Background(), webfeed.JudgeInput{System: "s", User: "u", Candidates: 1}); err == nil {
		t.Fatal("hung call returned no error")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("call took %s with a 100ms budget", d)
	}
}

func TestInduceUsesMediumEffortAndParsesSelectors(t *testing.T) {
	api := &fakeAPI{reply: "```json\n{\"item\":\"li.post\",\"link\":\"h2 a\",\"title\":\"h2\",\"date\":\"time\",\"reason\":\"main list\"}\n```", usageIn: 20000, usageOut: 900}
	c := newTestClient(t, api)
	in := webfeed.InduceInput{System: webfeed.InduceSystemPrompt, User: "Page: https://news.example.com/"}
	css, u, err := c.Induce(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if css != (webfeed.CSSRecipe{Item: "li.post", Link: "h2 a", Title: "h2", Date: "time"}) {
		t.Fatalf("css = %+v", css)
	}
	if u.InputTokens != 20000 || u.OutputTokens != 900 {
		t.Fatalf("usage = %+v", u)
	}
	assertRequestShape(t, api.last(t), "medium", webfeed.InduceSystemPrompt, in.User)
}

func TestWatchUsesLowEffortAndParsesTheVerdict(t *testing.T) {
	api := &fakeAPI{reply: `{"verdict":"broken","reason":"the items are category pages"}`, usageIn: 1500, usageOut: 120}
	c := newTestClient(t, api)
	in := webfeed.WatchInput{System: webfeed.WatchSystemPrompt, User: "Page: https://news.example.com/"}
	v, u, err := c.Watch(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Broken || v.Reason != "the items are category pages" || u.OutputTokens != 120 {
		t.Fatalf("verdict %+v usage %+v", v, u)
	}
	assertRequestShape(t, api.last(t), "low", webfeed.WatchSystemPrompt, in.User)
}

// recordingTransport answers every request itself, so a test can see where the client would have gone.
type recordingTransport struct {
	mu   sync.Mutex
	reqs []*http.Request
}

func (rt *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.reqs = append(rt.reqs, r)
	rt.mu.Unlock()
	body := `{"id":"m","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"text","text":"{\"verdict\":\"healthy\",\"reason\":\"ok\"}"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader([]byte(body))), Request: r}, nil
}

func TestEnvironmentCannotRedirectTheKeyOrSwapTheClient(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "https://elsewhere.example.com")
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	rt := &recordingTransport{}
	c, err := New(Config{APIKey: "configured-key", HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Watch(context.Background(), webfeed.WatchInput{System: "s", User: "u"}); err != nil {
		t.Fatal(err)
	}
	if len(rt.reqs) != 1 {
		t.Fatalf("requests through the passed client = %d", len(rt.reqs))
	}
	r := rt.reqs[0]
	if r.URL.Host != "api.anthropic.com" || r.Header.Get("X-Api-Key") != "configured-key" {
		t.Fatalf("request went to %s with key %q", r.URL.Host, r.Header.Get("X-Api-Key"))
	}
}

func TestNewRequiresAKeyAndAClient(t *testing.T) {
	if _, err := New(Config{HTTPClient: http.DefaultClient}); err == nil {
		t.Error("empty key accepted")
	}
	if _, err := New(Config{APIKey: "k"}); err == nil {
		t.Error("nil HTTP client accepted; the SDK default has no overall timeout")
	}
}

func TestErrorsNeverCarryTheKey(t *testing.T) {
	api := &fakeAPI{status: http.StatusBadRequest}
	c := newTestClient(t, api)
	_, _, err := c.Judge(context.Background(), webfeed.JudgeInput{System: "s", User: "u", Candidates: 1})
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("err = %v", err)
	}
}

func TestCostUsesHaikuPricesAndTheLongPromptTier(t *testing.T) {
	cases := []struct {
		in, out int
		want    float64
	}{
		{2500, 600, 0.00055},
		{50_000, 0, 0.005},
		{0, 1_000_000, 0.50},
		{100_000, 1000, 0.0105},
		{100_001, 1000, 100_001*0.50/1e6 + 1000*2.50/1e6},
	}
	for _, c := range cases {
		if got := Cost(c.in, c.out); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("Cost(%d, %d) = %v, want %v", c.in, c.out, got, c.want)
		}
	}
}

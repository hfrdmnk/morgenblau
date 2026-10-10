package webfeed

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// fakeModel answers each step from its own script of raw replies: the judge runs at effort high, induction at medium
// and the weekly check at low. The last reply repeats; a step without a script fails like an unusable answer.
type fakeModel struct {
	mu      sync.Mutex
	replies map[Effort][]string
	errs    map[Effort][]error
	prompts []Prompt
	// before runs ahead of each answer, outside the lock, so a test can hold calls or cancel.
	before func(ctx context.Context, p Prompt)
}

var stepUsage = map[Effort]Usage{
	EffortHigh:   {InputTokens: 2500, OutputTokens: 600, USD: 0.00054},
	EffortMedium: {InputTokens: 20000, OutputTokens: 900, USD: 0.00245},
	EffortLow:    {InputTokens: 1500, OutputTokens: 120, USD: 0.00021},
}

func (f *fakeModel) Complete(ctx context.Context, p Prompt) (string, Usage, error) {
	if f.before != nil {
		f.before(ctx, p)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := len(f.byEffort(p.Effort))
	f.prompts = append(f.prompts, p)
	u := stepUsage[p.Effort]
	if errs := f.errs[p.Effort]; i < len(errs) && errs[i] != nil {
		return "", u, errs[i]
	}
	rs := f.replies[p.Effort]
	if len(rs) == 0 {
		return "", u, errors.New("no reply scripted for this step")
	}
	return rs[min(i, len(rs)-1)], u, nil
}

func (f *fakeModel) byEffort(e Effort) []Prompt {
	var out []Prompt
	for _, p := range f.prompts {
		if p.Effort == e {
			out = append(out, p)
		}
	}
	return out
}

// calls is how many prompts reached the model at effort e.
func (f *fakeModel) calls(e Effort) []Prompt {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byEffort(e)
}

func judgeReply(order Order, pick ...int) string {
	raw, _ := json.Marshal(map[string]any{"pick": pick, "order": order, "none": false, "reason": "posts"})
	return string(raw)
}

func cssReply(t *testing.T, r CSSRecipe) string {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDecodeReplyToleratesFencesAndProse(t *testing.T) {
	for _, text := range []string{
		`{"a":1}`,
		"```json\n{\"a\":1}\n```",
		"Here is my answer:\n{\"a\":1}\nThanks.",
	} {
		var v struct{ A int }
		if err := decodeReply(text, &v); err != nil || v.A != 1 {
			t.Errorf("decodeReply(%q) = %+v, %v", text, v, err)
		}
	}
	var v struct{}
	if err := decodeReply("no json here", &v); err == nil {
		t.Error("prose without JSON decoded")
	}
	if err := decodeReply("{broken", &v); err == nil {
		t.Error("unterminated object decoded")
	}
}

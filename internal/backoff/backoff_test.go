package backoff

import (
	"reflect"
	"testing"
	"time"
)

func TestPolicy_Delay(t *testing.T) {
	p := Policy{Steps: []time.Duration{time.Minute, 5 * time.Minute, time.Hour}}

	cases := []struct {
		name                string
		consecutiveFailures int
		want                time.Duration
	}{
		{"zero failures", 0, 0},
		{"negative failures", -1, 0},
		{"first failure hits first step", 1, time.Minute},
		{"second failure hits second step", 2, 5 * time.Minute},
		{"beyond ladder length clamps to last step", len(p.Steps) + 5, time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Delay(tc.consecutiveFailures); got != tc.want {
				t.Errorf("Delay(%d) = %v, want %v", tc.consecutiveFailures, got, tc.want)
			}
		})
	}
}

func TestPolicy_Delay_EmptyLadderAllZero(t *testing.T) {
	var p Policy
	for _, failures := range []int{-1, 0, 1, 10} {
		if got := p.Delay(failures); got != 0 {
			t.Errorf("Delay(%d) = %v, want 0 for empty ladder", failures, got)
		}
	}
}

func TestPolicy_Cap(t *testing.T) {
	p := Policy{Steps: []time.Duration{time.Minute, 5 * time.Minute, time.Hour}}
	if got := p.Cap(); got != time.Hour {
		t.Errorf("Cap() = %v, want %v", got, time.Hour)
	}
}

func TestPolicy_Cap_EmptyLadderIsZero(t *testing.T) {
	var p Policy
	if got := p.Cap(); got != 0 {
		t.Errorf("Cap() = %v, want 0 for empty ladder", got)
	}
}

func TestExponential(t *testing.T) {
	got := Exponential(time.Hour, 2, 7*24*time.Hour)
	want := []time.Duration{
		time.Hour,
		2 * time.Hour,
		4 * time.Hour,
		8 * time.Hour,
		16 * time.Hour,
		32 * time.Hour,
		64 * time.Hour,
		128 * time.Hour,
		168 * time.Hour,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Exponential = %v, want %v", got, want)
	}
}

func TestExponential_BaseExceedsMax(t *testing.T) {
	max := 7 * 24 * time.Hour
	got := Exponential(200*time.Hour, 2, max)
	want := []time.Duration{max}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Exponential = %v, want %v", got, want)
	}
}

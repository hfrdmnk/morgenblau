// Package backoff maps consecutive-failure counts to retry delays and parses upstream retry hints.
package backoff

import (
	"net/http"
	"strconv"
	"time"
)

// ParseRetryAfter accepts delta-seconds and HTTP-date forms; ok is false for absent, garbage, or negative values.
func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	d := t.Sub(now)
	if d < 0 {
		return 0, false
	}
	return d, true
}

// Policy maps a consecutive-failure count to a delay ladder; the last step is the cap and repeats forever.
type Policy struct {
	Steps []time.Duration
}

// Delay returns 0 for failures <= 0 or an empty ladder, Steps[failures-1] otherwise, clamped to the last step.
func (p Policy) Delay(consecutiveFailures int) time.Duration {
	if consecutiveFailures <= 0 || len(p.Steps) == 0 {
		return 0
	}
	idx := consecutiveFailures - 1
	if idx >= len(p.Steps) {
		idx = len(p.Steps) - 1
	}
	return p.Steps[idx]
}

// Cap returns the last step, 0 for an empty ladder.
func (p Policy) Cap() time.Duration {
	if len(p.Steps) == 0 {
		return 0
	}
	return p.Steps[len(p.Steps)-1]
}

// Exponential builds a ladder base, base*factor, ...; the final step equals max exactly and the ladder never overshoots it.
func Exponential(base time.Duration, factor float64, max time.Duration) []time.Duration {
	var steps []time.Duration
	cur := base
	for cur < max {
		steps = append(steps, cur)
		cur = time.Duration(float64(cur) * factor)
	}
	return append(steps, max)
}

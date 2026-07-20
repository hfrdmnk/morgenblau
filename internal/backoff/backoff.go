// Package backoff maps consecutive-failure counts to retry delays.
package backoff

import "time"

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

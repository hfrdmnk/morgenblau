package backoff

import (
	"net/http"
	"strconv"
	"time"
)

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

type Policy struct {
	Steps []time.Duration
}

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

func (p Policy) Cap() time.Duration {
	if len(p.Steps) == 0 {
		return 0
	}
	return p.Steps[len(p.Steps)-1]
}

func Exponential(base time.Duration, factor float64, max time.Duration) []time.Duration {
	var steps []time.Duration
	cur := base
	for cur < max {
		steps = append(steps, cur)
		cur = time.Duration(float64(cur) * factor)
	}
	return append(steps, max)
}

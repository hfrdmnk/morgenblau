package api

import (
	"strings"
	"testing"
	"time"
)

func TestFrequencyBucket(t *testing.T) {
	now := mustParseTime(t, "2026-05-21T12:00:00Z")
	long := now.AddDate(-1, 0, 0).Format(time.RFC3339) // first post a year ago, "New" doesn't apply
	young := now.AddDate(0, 0, -10).Format(time.RFC3339)

	cases := []struct {
		name              string
		first             string
		c7, c28, c56, c84 int64
		want              string
	}{
		{"no posts at all", "", 0, 0, 0, 0, "noPosts"},
		{"cadence wins over recency", young, 99, 99, 99, 99, "daily"},
		{"new is fallback when no cadence fires", young, 0, 0, 0, 0, "new"},
		{"new is fallback when below all thresholds", young, 0, 0, 0, 1, "new"},
		{"daily ≥5/7d", long, 5, 5, 5, 5, "daily"},
		{"weekly ≥3/28d", long, 0, 3, 3, 3, "weekly"},
		{"biweekly ≥3/56d", long, 0, 0, 3, 3, "biweekly"},
		{"monthly ≥2/84d", long, 0, 0, 0, 2, "monthly"},
		{"irregular below every threshold", long, 0, 0, 0, 1, "irregular"},
		{"highest-cadence bucket wins", long, 5, 1, 1, 1, "daily"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := frequencyBucket(c.first, c.c7, c.c28, c.c56, c.c84, now)
			if got != c.want {
				t.Errorf("frequencyBucket = %q, want %q", got, c.want)
			}
		})
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestNormalizeTags(t *testing.T) {
	long := strings.Repeat("é", 65) // 65 graphemes/runes, must drop
	in := []string{" a ", "A", "", "  ", "b", "B", long, "c"}
	got := normalizeTags(in)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("normalizeTags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// Cap at 10.
	many := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		many = append(many, string(rune('a'+i)))
	}
	if n := len(normalizeTags(many)); n != 10 {
		t.Errorf("cap: len = %d, want 10", n)
	}

	// Empty in → nil/empty out.
	if got := normalizeTags(nil); len(got) != 0 {
		t.Errorf("nil input → %v", got)
	}
}

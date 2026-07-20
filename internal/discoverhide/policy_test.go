package discoverhide

import (
	"testing"
	"time"
)

func TestNextSnooze_FirstHide_30Days(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	until, count := NextSnooze(0, now)

	if want := now.AddDate(0, 0, 30); !until.Equal(want) {
		t.Errorf("hiddenUntil = %v, want %v", until, want)
	}
	if count != 1 {
		t.Errorf("hideCount = %d, want 1", count)
	}
}

func TestNextSnooze_RepeatHide_180Days(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	until, count := NextSnooze(1, now)

	if want := now.AddDate(0, 0, 180); !until.Equal(want) {
		t.Errorf("hiddenUntil = %v, want %v", until, want)
	}
	if count != 2 {
		t.Errorf("hideCount = %d, want 2", count)
	}
}

func TestNextSnooze_TableDriven(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		existingCount int64
		wantDays      int
		wantCount     int64
	}{
		{"never hidden", 0, 30, 1},
		{"first repeat", 1, 180, 2},
		{"third hide stays at 180", 2, 180, 3},
		{"many repeats stay at 180", 9, 180, 10},
		// Defensive: a corrupt/negative count must not escalate past first tier.
		{"negative count treated as never hidden", -1, 30, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			until, count := NextSnooze(tt.existingCount, now)
			if want := now.AddDate(0, 0, tt.wantDays); !until.Equal(want) {
				t.Errorf("hiddenUntil = %v, want %v", until, want)
			}
			if count != tt.wantCount {
				t.Errorf("hideCount = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

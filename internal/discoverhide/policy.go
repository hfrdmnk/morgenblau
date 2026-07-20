// Package discoverhide computes hide/snooze durations; no I/O.
// SPEC <discovery> Hiding and rotation.
package discoverhide

import "time"

// TargetKind distinguishes the two hide-target kinds sharing one mechanism. SPEC <discovery>.
type TargetKind string

const (
	TargetSource TargetKind = "source"
	TargetPerson TargetKind = "person"
)

const (
	firstSnoozeDays  = 30
	repeatSnoozeDays = 180
)

// NextSnooze computes hidden_until and hide_count for a hide action. Escalation
// has exactly two tiers, first hide then repeat; it never escalates further. SPEC <discovery>.
func NextSnooze(existingHideCount int64, now time.Time) (hiddenUntil time.Time, hideCount int64) {
	if existingHideCount <= 0 {
		return now.AddDate(0, 0, firstSnoozeDays), 1
	}
	return now.AddDate(0, 0, repeatSnoozeDays), existingHideCount + 1
}

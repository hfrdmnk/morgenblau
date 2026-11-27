package sync

import "time"

func updatedAfterSnapshot(updatedAt string, snapshotAt time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return false
	}
	return t.After(snapshotAt)
}

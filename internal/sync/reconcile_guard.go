package sync

import "time"

// updatedAfterSnapshot parses rather than string-compares, since a stored numeric zone offset sorts wrong against a Z-suffixed snapshot.
func updatedAfterSnapshot(updatedAt string, snapshotAt time.Time) bool {
	t, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return false
	}
	return t.After(snapshotAt)
}

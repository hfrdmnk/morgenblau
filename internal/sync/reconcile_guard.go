package sync

import "time"

// createdAfterSnapshot guards in-flight writes: a row newer than the PDS listing is missing from it by timing, not by remote delete.
// Parsed rather than string-compared, since a stored numeric zone offset sorts wrong against a Z-suffixed snapshot; unparseable keeps the old delete behavior.
func createdAfterSnapshot(createdAt string, snapshotAt time.Time) bool {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return false
	}
	return t.After(snapshotAt)
}

// Package tags owns the on-disk serialization for a subscription's tag list:
// a JSON array string in the user_subscriptions.tags column, NULL when empty.
// Both the API write path and the sync reconcile path store tags, so the format
// lives here to keep them from drifting.
package tags

import "encoding/json"

// Marshal renders tags as a JSON array string for storage, or nil when empty.
func Marshal(tags []string) *string {
	if len(tags) == 0 {
		return nil
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// Unmarshal parses a stored JSON array string back into a slice. Returns nil on
// a nil pointer, blank string, or parse error.
func Unmarshal(s *string) []string {
	if s == nil || *s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(*s), &out); err != nil {
		return nil
	}
	return out
}

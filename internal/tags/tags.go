// Package tags serializes a subscription's tag list as a JSON array string
// (NULL when empty) for the user_subscriptions.tags column.
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

// Unmarshal parses a stored tags string, returning nil on any parse failure.
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

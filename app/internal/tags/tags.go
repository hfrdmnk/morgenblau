package tags

import "encoding/json"

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

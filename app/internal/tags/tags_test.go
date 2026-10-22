package tags

import "testing"

func TestMarshalUnmarshal(t *testing.T) {
	if Marshal(nil) != nil {
		t.Errorf("Marshal(nil) should be nil")
	}
	if Marshal([]string{}) != nil {
		t.Errorf("Marshal(empty) should be nil")
	}
	p := Marshal([]string{"a", "b"})
	if p == nil || *p != `["a","b"]` {
		t.Errorf("Marshal = %v", p)
	}
	if got := Unmarshal(p); len(got) != 2 || got[0] != "a" {
		t.Errorf("Unmarshal round-trip = %v", got)
	}
	if got := Unmarshal(nil); len(got) != 0 {
		t.Errorf("Unmarshal(nil) = %v", got)
	}
	bad := "not json"
	if got := Unmarshal(&bad); len(got) != 0 {
		t.Errorf("Unmarshal(bad) = %v", got)
	}
}

package discoverperson

import "testing"

func TestPage(t *testing.T) {
	seq := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}

	tests := []struct {
		name     string
		items    []int
		cursor   string
		limit    int
		wantLen  int
		wantNext string
	}{
		{name: "empty", items: seq(0), cursor: "", limit: 10, wantLen: 0, wantNext: ""},
		{name: "exactly one page", items: seq(10), cursor: "", limit: 10, wantLen: 10, wantNext: ""},
		{name: "first of two pages", items: seq(11), cursor: "", limit: 10, wantLen: 10, wantNext: "10"},
		{name: "second page remainder", items: seq(11), cursor: "10", limit: 10, wantLen: 1, wantNext: ""},
		{name: "invalid cursor is first page", items: seq(11), cursor: "abc", limit: 10, wantLen: 10, wantNext: "10"},
		{name: "negative cursor is first page", items: seq(11), cursor: "-5", limit: 10, wantLen: 10, wantNext: "10"},
		{name: "cursor past end", items: seq(11), cursor: "100", limit: 10, wantLen: 0, wantNext: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, next := Page(tt.items, tt.cursor, tt.limit)
			if len(page) != tt.wantLen {
				t.Errorf("page len = %d, want %d", len(page), tt.wantLen)
			}
			if next != tt.wantNext {
				t.Errorf("next = %q, want %q", next, tt.wantNext)
			}
		})
	}
}

func TestPageSecondPageContents(t *testing.T) {
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	page, next := Page(items, "10", 10)
	if len(page) != 1 || page[0] != 10 {
		t.Errorf("second page = %v, want [10]", page)
	}
	if next != "" {
		t.Errorf("next = %q, want empty", next)
	}
}

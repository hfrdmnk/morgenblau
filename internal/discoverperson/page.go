package discoverperson

import "strconv"

// Page returns the slice of items at the opaque cursor plus the cursor for the next page
// (empty when exhausted). The cursor is a base-10 offset; an empty or invalid cursor starts
// at the first page. limit is the page size (10 at profile call sites, SPEC <discovery>).
func Page[T any](items []T, cursor string, limit int) ([]T, string) {
	if limit <= 0 {
		return nil, ""
	}
	start := 0
	if n, err := strconv.Atoi(cursor); err == nil && n > 0 {
		start = n
	}
	if start >= len(items) {
		return nil, ""
	}
	end := start + limit
	if end >= len(items) {
		return items[start:], ""
	}
	return items[start:end], strconv.Itoa(end)
}

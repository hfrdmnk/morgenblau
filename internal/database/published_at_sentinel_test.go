package database

import (
	"regexp"
	"strings"
	"testing"
)

var (
	publishedUpperBound = regexp.MustCompile(`(?i)\bpublished_at\s*<[^>]`)
	publishedLowerBound = regexp.MustCompile(`(?i)\bpublished_at\s*>`)
	publishedMin        = regexp.MustCompile(`(?i)\bMIN\(\s*(\w+\.)?published_at\b`)
)

// An undated generated-feed entry stores published_at = ”, which sorts before every date: a window needs a lower
// bound and a minimum needs NULLIF, or the history entry lands in a digest day or hides the feed's first real date.
func TestEmptyPublishedAtNeverReachesADigestDay(t *testing.T) {
	for _, q := range loadSQLQueries(t) {
		sql := q.sql
		if !strings.Contains(sql, "feed_entries") || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
			continue
		}
		if publishedUpperBound.MatchString(sql) && !publishedLowerBound.MatchString(sql) {
			t.Errorf("%s/%s bounds published_at from above only, so undated history (published_at = '') falls inside", q.file, q.name)
		}
		if publishedMin.MatchString(sql) {
			t.Errorf("%s/%s takes MIN(published_at); use MIN(NULLIF(published_at, '')) so undated history is not the oldest date", q.file, q.name)
		}
	}
}

func TestPublishedAtGuardCatchesAnUnboundedWindow(t *testing.T) {
	if !publishedUpperBound.MatchString("WHERE e.published_at < ?") || publishedLowerBound.MatchString("WHERE e.published_at < ?") {
		t.Fatal("the guard misses a window bounded from above only")
	}
	if !publishedMin.MatchString("SELECT MIN(fe.published_at) FROM feed_entries fe") || publishedMin.MatchString("SELECT MIN(NULLIF(published_at, ''))") {
		t.Fatal("the guard misreads MIN")
	}
}

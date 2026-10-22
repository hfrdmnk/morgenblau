package webfeed

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var monthNames = func() map[string]time.Month {
	byMonth := [12][]string{
		{"jan", "january", "januar", "jänner", "janvier", "janv", "enero", "ene", "gennaio", "gen", "janeiro", "januari"},
		{"feb", "february", "februar", "février", "fevrier", "févr", "fevr", "febrero", "febbraio", "fevereiro", "fev", "februari"},
		{"mar", "march", "märz", "maerz", "mär", "mrz", "mars", "marzo", "março", "marco", "maart", "mrt"},
		{"apr", "april", "avril", "avr", "abril", "abr", "aprile"},
		{"may", "mai", "mayo", "maggio", "mag", "maio", "mei", "maj"},
		{"jun", "june", "juni", "juin", "junio", "giugno", "giu", "junho"},
		{"jul", "july", "juli", "juillet", "juil", "julio", "luglio", "lug", "julho"},
		{"aug", "august", "août", "aout", "agosto", "ago", "augustus", "augusti"},
		{"sep", "sept", "september", "septembre", "septiembre", "setiembre", "settembre", "set", "setembro"},
		{"oct", "october", "oktober", "okt", "octobre", "octubre", "ottobre", "ott", "outubro", "out"},
		{"nov", "november", "novembre", "noviembre", "novembro"},
		{"dec", "december", "dezember", "dez", "décembre", "decembre", "déc", "diciembre", "dic", "dicembre", "dezembro"},
	}
	m := map[string]time.Month{}
	for i, names := range byMonth {
		for _, n := range names {
			m[n] = time.Month(i + 1)
		}
	}
	return m
}()

var (
	reISODate   = regexp.MustCompile(`\b(\d{4})-(\d{1,2})-(\d{1,2})(?:\D|$)`)
	reCJKDate   = regexp.MustCompile(`(\d{4})\s*[年년]\s*(\d{1,2})\s*[月월]\s*(\d{1,2})`)
	reYMDDate   = regexp.MustCompile(`\b(\d{4})[/.](\d{1,2})[/.](\d{1,2})\b`)
	reMonthDay  = regexp.MustCompile(`(?i)\b(\p{L}{3,})\.?\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(\d{4})\b`)
	reDayMonth  = regexp.MustCompile(`(?i)\b(\d{1,2})(?:\.|st|nd|rd|th|er|º)?\s+(?:de\s+)?(\p{L}{3,})\.?,?\s+(?:de\s+)?(\d{4})\b`)
	reDMYDots   = regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})\.(\d{4})\b`)
	reSlashDate = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{4})\b`)
	reMonthYear = regexp.MustCompile(`(?i)\b(\p{L}{3,})\.?\s+(\d{4})\b`)
	reURLDate   = regexp.MustCompile(`/((?:19|20)\d{2})/(\d{1,2})(?:/(\d{1,2}))?/`)
)

// parseDateText finds the first calendar date in visible text, in the formats listing pages use across languages.
func parseDateText(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 400 {
		return time.Time{}, false
	}
	if m := reISODate.FindStringSubmatch(s); m != nil {
		if t, ok := ymd(m[1], m[2], m[3]); ok {
			return t, true
		}
	}
	if m := reCJKDate.FindStringSubmatch(s); m != nil {
		if t, ok := ymd(m[1], m[2], m[3]); ok {
			return t, true
		}
	}
	if m := reYMDDate.FindStringSubmatch(s); m != nil {
		if t, ok := ymd(m[1], m[2], m[3]); ok {
			return t, true
		}
	}
	if m := reMonthDay.FindStringSubmatch(s); m != nil {
		if mon, ok := monthNames[strings.ToLower(m[1])]; ok {
			if t, ok := ymdMonth(m[3], mon, m[2]); ok {
				return t, true
			}
		}
	}
	for _, m := range reDayMonth.FindAllStringSubmatch(s, -1) {
		if mon, ok := monthNames[strings.ToLower(m[2])]; ok {
			if t, ok := ymdMonth(m[3], mon, m[1]); ok {
				return t, true
			}
		}
	}
	if m := reDMYDots.FindStringSubmatch(s); m != nil {
		if t, ok := ymd(m[3], m[2], m[1]); ok {
			return t, true
		}
	}
	if m := reSlashDate.FindStringSubmatch(s); m != nil {
		a, _ := strconv.Atoi(m[1])
		// US month-first unless the first number cannot be a month.
		if a > 12 {
			if t, ok := ymd(m[3], m[2], m[1]); ok {
				return t, true
			}
		} else if t, ok := ymd(m[3], m[1], m[2]); ok {
			return t, true
		}
	}
	for _, m := range reMonthYear.FindAllStringSubmatch(s, -1) {
		if mon, ok := monthNames[strings.ToLower(m[1])]; ok {
			if t, ok := ymdMonth(m[2], mon, "1"); ok {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

var attrLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02",
	time.RFC1123,
	time.RFC1123Z,
	time.RFC822,
	time.RFC822Z,
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04:05 -0700",
}

// parseDateAttr parses machine-readable dates from datetime, content and JSON-LD values.
func parseDateAttr(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range attrLayouts {
		if t, err := time.Parse(layout, s); err == nil && plausibleYear(t.Year()) {
			return t, true
		}
	}
	return parseDateText(s)
}

// dateFromURL reads /YYYY/MM(/DD)/ permalinks; a month-only path dates the item to the 1st.
func dateFromURL(raw string) (time.Time, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return time.Time{}, false
	}
	m := reURLDate.FindStringSubmatch(u.Path + "/")
	if m == nil {
		return time.Time{}, false
	}
	day := m[3]
	if day == "" {
		day = "1"
	}
	return ymd(m[1], m[2], day)
}

func ymd(y, m, d string) (time.Time, bool) {
	mi, err := strconv.Atoi(m)
	if err != nil || mi < 1 || mi > 12 {
		return time.Time{}, false
	}
	return ymdMonth(y, time.Month(mi), d)
}

func ymdMonth(y string, m time.Month, d string) (time.Time, bool) {
	yi, err1 := strconv.Atoi(y)
	di, err2 := strconv.Atoi(d)
	if err1 != nil || err2 != nil || !plausibleYear(yi) || di < 1 || di > 31 {
		return time.Time{}, false
	}
	t := time.Date(yi, m, di, 0, 0, 0, 0, time.UTC)
	if t.Month() != m {
		return time.Time{}, false
	}
	return t, true
}

func plausibleYear(y int) bool { return y >= 1990 && y <= 2100 }

package webfeed

import (
	"testing"
	"time"
)

func TestParseDateText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2024-03-05", "2024-03-05"},
		{"2024-03-05T10:00:00Z", "2024-03-05"},
		{"2024-03-05T10:00:00+09:00", "2024-03-05"},
		{"March 5, 2024", "2024-03-05"},
		{"Mar 5, 2024", "2024-03-05"},
		{"Sept 12, 2023", "2023-09-12"},
		{"5 March 2024", "2024-03-05"},
		{"Published on 5th March 2024 by Example Author", "2024-03-05"},
		{"Tuesday, March 5, 2024", "2024-03-05"},
		{"5. März 2024", "2024-03-05"},
		{"12. Dezember 2023", "2023-12-12"},
		{"05.03.2024", "2024-03-05"},
		{"5 mars 2024", "2024-03-05"},
		{"le 1er février 2024", "2024-02-01"},
		{"5 de marzo de 2024", "2024-03-05"},
		{"5 maggio 2024", "2024-05-05"},
		{"2024年3月5日", "2024-03-05"},
		{"2024년 3월 5일", "2024-03-05"},
		{"2024/03/05", "2024-03-05"},
		{"2024.03.05", "2024-03-05"},
		{"03/25/2024", "2024-03-25"},
		{"25/03/2024", "2024-03-25"},
		{"Mar 2024", "2024-03-01"},
		{"Oct 10", ""},
		{"Read more", ""},
		{"Version 2.0.1", ""},
		{"1234", ""},
		{"99/99/2024", ""},
	}
	for _, c := range cases {
		got, ok := parseDateText(c.in)
		if c.want == "" {
			if ok {
				t.Errorf("parseDateText(%q) = %v, want no date", c.in, got)
			}
			continue
		}
		if !ok || got.Format("2006-01-02") != c.want {
			t.Errorf("parseDateText(%q) = %v (%v), want %s", c.in, got, ok, c.want)
		}
	}
}

func TestParseDateAttr(t *testing.T) {
	for in, want := range map[string]string{
		"2024-03-05":                    "2024-03-05",
		"2024-03-05T10:00:00.000Z":      "2024-03-05",
		"2024-03-05 10:00:00":           "2024-03-05",
		"Tue, 05 Mar 2024 10:00:00 GMT": "2024-03-05",
	} {
		got, ok := parseDateAttr(in)
		if !ok || got.Format("2006-01-02") != want {
			t.Errorf("parseDateAttr(%q) = %v, %v", in, got, ok)
		}
	}
	if _, ok := parseDateAttr("soon"); ok {
		t.Error("parseDateAttr(soon) parsed")
	}
}

func TestDateFromURLPath(t *testing.T) {
	cases := map[string]string{
		"https://news.example.com/2024/03/05/hello":   "2024-03-05",
		"https://news.example.com/blog/2023/11/hello": "2023-11-01",
		"https://news.example.com/blog/hello":         "",
		"https://news.example.com/item/2024":          "",
	}
	for in, want := range cases {
		got, ok := dateFromURL(in)
		if want == "" {
			if ok {
				t.Errorf("dateFromURL(%q) = %v", in, got)
			}
			continue
		}
		if !ok || got.Format("2006-01-02") != want {
			t.Errorf("dateFromURL(%q) = %v, %v want %s", in, got, ok, want)
		}
	}
}

func TestParsedDatesAreUTCMidnightOrInstant(t *testing.T) {
	got, _ := parseDateText("March 5, 2024")
	if !got.Equal(time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v", got)
	}
}

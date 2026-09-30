package api

import (
	"reflect"
	"strings"
	"testing"
)

func TestOPMLFoldersAndDuplicateFeeds(t *testing.T) {
	input := `<?xml version="1.0"?><opml version="2.0"><head><title>Example</title></head><body>
	<outline text="Technology"><outline title="AI"><outline type="rss" text="Example &amp; News" xmlUrl="https://example.com/feed?a=1&amp;b=2" htmlUrl="https://example.com/"/></outline></outline>
	<outline text="Favorites"><outline title="Other title" xmlUrl="https://example.com/feed?a=1&amp;b=2"/></outline>
	<outline text="technology"><outline xmlUrl="https://example.com/feed?a=1&amp;b=2"/></outline>
	<outline title="Unfiled" xmlUrl="https://other.example.com/feed"/>
	</body></opml>`
	got, err := parseOPML(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 || len(got.Warnings) != 0 {
		t.Fatalf("got %#v", got)
	}
	want := importSource{FeedURL: "https://example.com/feed?a=1&b=2", Title: "Example & News", SiteURL: "https://example.com/", Tags: []string{"Technology", "AI", "Favorites"}}
	if !reflect.DeepEqual(got.Sources[0], want) {
		t.Fatalf("got %#v, want %#v", got.Sources[0], want)
	}
	if len(got.Sources[1].Tags) != 0 {
		t.Fatalf("unfiled tags: %v", got.Sources[1].Tags)
	}
	encoded, err := encodeOPML(got.Sources)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := parseOPML(encoded)
	if err != nil {
		t.Fatal(err)
	}
	byURL := map[string]importSource{}
	for _, s := range roundtrip.Sources {
		byURL[s.FeedURL] = s
	}
	for _, s := range got.Sources {
		r := byURL[s.FeedURL]
		if r.Title != s.Title || r.SiteURL != s.SiteURL || !sameTagSet(r.Tags, s.Tags) {
			t.Fatalf("roundtrip %#v != %#v", r, s)
		}
	}
}

func sameTagSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if !set[s] {
			return false
		}
	}
	return true
}

func TestOPMLRejectsMalformedDocument(t *testing.T) {
	for _, input := range []string{`<html/>`, `<opml><body>`, `<opml/>`, `<opml><body/></opml><opml/>`, `<!DOCTYPE opml [<!ENTITY x SYSTEM "file:///etc/passwd">]><opml><body><outline text="&x;"/></body></opml>`} {
		if _, err := parseOPML(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestOPMLBoundsFolderDepth(t *testing.T) {
	input := `<opml><body>` + strings.Repeat(`<outline text="Folder">`, 65) + `<outline xmlUrl="https://example.com/feed"/>` + strings.Repeat(`</outline>`, 65) + `</body></opml>`
	if _, err := parseOPML(input); err == nil {
		t.Fatal("accepted excessive folder nesting")
	}
}

func TestOPMLReportsInvalidSourcesWithoutLosingValidOnes(t *testing.T) {
	got, err := parseOPML(`<opml version="1.1"><body><outline text="Valid" xmlUrl="https://example.com/feed"/><outline text="Invalid" xmlUrl="javascript:alert(1)"/><outline text="` + strings.Repeat("x", 65) + `"><outline xmlUrl="https://other.example.com/feed"/></outline></body></opml>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 || len(got.Warnings) != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestImportTagsDoNotTruncate(t *testing.T) {
	input := []string{"One", " one ", "TWO", "", "three"}
	got, err := mergeImportTags(nil, input)
	if err != nil || !reflect.DeepEqual(got, []string{"One", "TWO", "three"}) {
		t.Fatalf("%v, %v", got, err)
	}
	for _, invalid := range [][]string{{strings.Repeat("x", 65)}, {"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}} {
		if _, err := mergeImportTags(nil, invalid); err == nil {
			t.Fatalf("accepted %v", invalid)
		}
	}
}

package discoverrank

import (
	"testing"

	"morgenblau/internal/discoverlang"
)

func TestFilterByLanguage_UnknownLanguagePasses(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://unknown-lang", Language: ""},
	}
	reader := map[discoverlang.Language]struct{}{"en": {}}

	got := FilterByLanguage(candidates, reader)

	if len(got) != 1 {
		t.Fatalf("got = %+v, want the unknown-language candidate to pass", got)
	}
}

func TestFilterByLanguage_EnglishOnlyReaderExcludesJapaneseDetectedSource(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://japanese-source", Language: "ja"},
		{Key: "https://english-source", Language: "en"},
	}
	reader := map[discoverlang.Language]struct{}{"en": {}}

	got := FilterByLanguage(candidates, reader)

	if len(got) != 1 || got[0].Key != "https://english-source" {
		t.Fatalf("got = %+v, want only the English source", got)
	}
}

func TestFilterByLanguage_EmptyReaderSetExcludesEveryKnownLanguage(t *testing.T) {
	candidates := []Candidate{
		{Key: "https://known", Language: "en"},
		{Key: "https://unknown", Language: ""},
	}

	got := FilterByLanguage(candidates, map[discoverlang.Language]struct{}{})

	if len(got) != 1 || got[0].Key != "https://unknown" {
		t.Fatalf("got = %+v, want only the unknown-language candidate", got)
	}
}

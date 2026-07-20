package discoverlang

import "testing"

func TestNormalizeTag(t *testing.T) {
	cases := []struct {
		in   string
		want Language
		ok   bool
	}{
		{"en", "en", true},
		{"en-US", "en", true},
		{"fr_FR", "fr", true},
		{"EN", "en", true},
		{"  ja \t", "ja", true},
		{"", "", false},
		{"x", "", false}, // too short to be a real subtag
	}
	for _, c := range cases {
		got, ok := NormalizeTag(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeTag(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

type stubDetector struct {
	lang Language
	ok   bool
}

func (s stubDetector) Detect(string) (Language, bool) { return s.lang, s.ok }

func TestSourceLanguage_ContentWinsOverTagOnDisagreement(t *testing.T) {
	detector := stubDetector{lang: "fr", ok: true}

	got, ok := SourceLanguage(detector, "some french content", "en")

	if !ok || got != "fr" {
		t.Fatalf("SourceLanguage = (%q, %v), want (fr, true) — content must win over a disagreeing tag", got, ok)
	}
}

func TestSourceLanguage_FallsBackToTagWhenContentInconclusive(t *testing.T) {
	detector := stubDetector{ok: false}

	got, ok := SourceLanguage(detector, "too short", "fr-FR")

	if !ok || got != "fr" {
		t.Fatalf("SourceLanguage = (%q, %v), want (fr, true) from the tag hint", got, ok)
	}
}

func TestSourceLanguage_UnknownWhenNeitherContentNorTagResolve(t *testing.T) {
	detector := stubDetector{ok: false}

	got, ok := SourceLanguage(detector, "", "")

	if ok || got != "" {
		t.Fatalf("SourceLanguage = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestReaderLanguages_NoSubscriptionsFallsBackToLocale(t *testing.T) {
	got := ReaderLanguages(nil, "de")

	if len(got) != 1 {
		t.Fatalf("ReaderLanguages = %v, want exactly {de}", got)
	}
	if _, ok := got["de"]; !ok {
		t.Fatalf("ReaderLanguages = %v, want the locale fallback present", got)
	}
}

func TestReaderLanguages_FirstFrenchSubscriptionAdmitsFrench(t *testing.T) {
	before := ReaderLanguages([]Language{"en", "en"}, "en")
	if _, ok := before["fr"]; ok {
		t.Fatalf("before = %v, French must not be admitted yet", before)
	}

	after := ReaderLanguages([]Language{"en", "en", "fr"}, "en")

	if _, ok := after["fr"]; !ok {
		t.Fatalf("after = %v, want French admitted once a French subscription exists", after)
	}
	if _, ok := after["en"]; !ok {
		t.Fatalf("after = %v, want English still present", after)
	}
}

func TestReaderLanguages_UnknownSubscriptionLanguagesIgnored(t *testing.T) {
	got := ReaderLanguages([]Language{"", "en", ""}, "de")

	if len(got) != 1 {
		t.Fatalf("ReaderLanguages = %v, want only {en} (blank entries ignored, no fallback needed)", got)
	}
	if _, ok := got["en"]; !ok {
		t.Fatalf("ReaderLanguages = %v, want en", got)
	}
}

func TestPasses_UnknownSourceLanguageAlwaysPasses(t *testing.T) {
	reader := map[Language]struct{}{"en": {}}

	if !Passes("", reader) {
		t.Fatal("Passes(\"\", ...) = false, want true (unknown always passes)")
	}
}

func TestPasses_EnglishOnlyReaderExcludesJapanese(t *testing.T) {
	reader := map[Language]struct{}{"en": {}}

	if Passes("ja", reader) {
		t.Fatal("Passes(ja, {en}) = true, want false")
	}
	if !Passes("en", reader) {
		t.Fatal("Passes(en, {en}) = false, want true")
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	cases := []struct {
		header string
		want   Language
	}{
		{"", "en"},
		{"fr", "fr"},
		{"fr-FR,fr;q=0.9,en;q=0.8", "fr"},
		{"  de-DE  ", "de"},
		{"garbage!!!", "en"},
	}
	for _, c := range cases {
		if got := ParseAcceptLanguage(c.header); got != c.want {
			t.Errorf("ParseAcceptLanguage(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

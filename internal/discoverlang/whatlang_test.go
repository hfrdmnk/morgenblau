package discoverlang

import "testing"

func TestWhatlangDetector_DetectsEnglish(t *testing.T) {
	d := NewDetector()
	text := "The quick brown fox jumps over the lazy dog while the sun sets slowly behind the distant hills, painting the sky in shades of orange and red."

	got, ok := d.Detect(text)

	if !ok || got != "en" {
		t.Fatalf("Detect(english) = (%q, %v), want (en, true)", got, ok)
	}
}

func TestWhatlangDetector_DetectsFrench(t *testing.T) {
	d := NewDetector()
	text := "Le rapide renard brun saute par-dessus le chien paresseux pendant que le soleil se couche lentement derriere les collines lointaines."

	got, ok := d.Detect(text)

	if !ok || got != "fr" {
		t.Fatalf("Detect(french) = (%q, %v), want (fr, true)", got, ok)
	}
}

func TestWhatlangDetector_DetectsJapanese(t *testing.T) {
	d := NewDetector()
	text := "吾輩は猫である。名前はまだ無い。どこで生れたかとんと見当がつかぬ。何でも薄暗いじめじめした所でニャーニャー泣いていた事だけは記憶している。"

	got, ok := d.Detect(text)

	if !ok || got != "ja" {
		t.Fatalf("Detect(japanese) = (%q, %v), want (ja, true)", got, ok)
	}
}

func TestWhatlangDetector_TooShortIsInconclusive(t *testing.T) {
	d := NewDetector()

	if _, ok := d.Detect("hi"); ok {
		t.Fatal("Detect(\"hi\") ok = true, want false (too short to be reliable)")
	}
}

func TestWhatlangDetector_EmptyIsInconclusive(t *testing.T) {
	d := NewDetector()

	if _, ok := d.Detect(""); ok {
		t.Fatal("Detect(\"\") ok = true, want false")
	}
}

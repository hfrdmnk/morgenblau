// Package discoverlang infers a source's language and the reader's languages from subscriptions.
// Content detection is primary; a feed's language tag is only a fallback hint. SPEC <discovery>.
package discoverlang

import "strings"

// Language is a normalized ISO 639-1 subtag; empty means unknown.
type Language string

// Detector reports ok=false rather than guess when text is too short or ambiguous.
type Detector interface {
	Detect(text string) (Language, bool)
}

// NormalizeTag extracts the lowercase primary subtag from a BCP-47/RSS tag like "en-US" or "fr_FR".
func NormalizeTag(tag string) (Language, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", false
	}
	primary := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })
	if len(primary) == 0 {
		return "", false
	}
	code := strings.ToLower(primary[0])
	if len(code) < 2 || len(code) > 8 || !isAlpha(code) {
		return "", false
	}
	return Language(code), true
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// SourceLanguage prefers content detection over the feed's tag hint, even when they disagree.
func SourceLanguage(detector Detector, content, tagHint string) (Language, bool) {
	if lang, ok := detector.Detect(content); ok {
		return lang, true
	}
	return NormalizeTag(tagHint)
}

// ReaderLanguages derives the reader's languages from subscriptions, falling back to localeFallback when none are known (cold start).
func ReaderLanguages(subscriptionLanguages []Language, localeFallback Language) map[Language]struct{} {
	out := make(map[Language]struct{})
	for _, l := range subscriptionLanguages {
		if l == "" {
			continue
		}
		out[l] = struct{}{}
	}
	if len(out) == 0 && localeFallback != "" {
		out[localeFallback] = struct{}{}
	}
	return out
}

// Passes always lets an unknown source language through: a wrong-language hit beats silently dropping a good source.
func Passes(sourceLang Language, reader map[Language]struct{}) bool {
	if sourceLang == "" {
		return true
	}
	_, ok := reader[sourceLang]
	return ok
}

// defaultLocale is "en" because Morgenblau's UI copy is English-only today.
const defaultLocale Language = "en"

// ParseAcceptLanguage takes the first tag from a header like "fr-FR,fr;q=0.9" (-> "fr").
func ParseAcceptLanguage(header string) Language {
	header = strings.TrimSpace(header)
	if header == "" {
		return defaultLocale
	}
	first := strings.SplitN(header, ",", 2)[0]
	first = strings.SplitN(first, ";", 2)[0]
	if lang, ok := NormalizeTag(first); ok {
		return lang
	}
	return defaultLocale
}

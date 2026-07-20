package discoverlang

import (
	"strings"

	"github.com/abadojack/whatlanggo"
)

// minSampleLength avoids trusting whatlanggo on short text, which produces noisy overconfident guesses.
const minSampleLength = 30

// whatlangDetector wraps whatlanggo: pure Go, ~400KB, no per-language data files, chosen
// over lingua-go (higher accuracy but ~120MB of embedded n-gram data) to keep CGO_ENABLED=0 builds lean.
type whatlangDetector struct{}

// NewDetector returns the production Detector.
func NewDetector() Detector {
	return whatlangDetector{}
}

func (whatlangDetector) Detect(text string) (Language, bool) {
	text = strings.TrimSpace(text)
	if len(text) < minSampleLength {
		return "", false
	}
	info := whatlanggo.Detect(text)
	if !info.IsReliable() {
		return "", false
	}
	code := info.Lang.Iso6391()
	if code == "" {
		return "", false
	}
	return Language(code), true
}

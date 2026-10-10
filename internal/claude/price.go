package claude

// Haiku 5.5 list prices in USD per million tokens, checked 9 October 2026; prompts above 100k input tokens bill at the long-prompt rate.
const (
	inputPerMTok         = 0.10
	outputPerMTok        = 0.50
	longInputPerMTok     = 0.50
	longOutputPerMTok    = 2.50
	longPromptFromTokens = 100_000
)

// cost prices one call's tokens at Haiku 5.5 rates.
func cost(inputTokens, outputTokens int) float64 {
	in, out := inputPerMTok, outputPerMTok
	if inputTokens > longPromptFromTokens {
		in, out = longInputPerMTok, longOutputPerMTok
	}
	return (float64(inputTokens)*in + float64(outputTokens)*out) / 1e6
}

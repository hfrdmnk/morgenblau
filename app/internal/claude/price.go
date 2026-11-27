package claude

// Claude Haiku 5.5 list prices in USD per million tokens; a prompt over 100,000 input tokens pays the higher prices for the whole request.
// https://platform.claude.com/docs/en/about-claude/pricing#long-context-pricing
const (
	inputPerMTok         = 0.10
	outputPerMTok        = 0.50
	longInputPerMTok     = 0.50
	longOutputPerMTok    = 2.50
	longPromptFromTokens = 100_000
)

func cost(inputTokens, outputTokens int) float64 {
	in, out := inputPerMTok, outputPerMTok
	if inputTokens > longPromptFromTokens {
		in, out = longInputPerMTok, longOutputPerMTok
	}
	return (float64(inputTokens)*in + float64(outputTokens)*out) / 1e6
}

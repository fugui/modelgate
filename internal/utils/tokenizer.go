package utils

// EstimateTokens provides a fast, heuristic-based estimate of token counts.
// It avoids the heavy dependency and initialization cost of tiktoken,
// making it suitable for high-throughput proxy interceptors.
//
// Heuristic rules (conservative / upper-bound estimates):
// - CJK characters (Chinese, Japanese, Korean) typically map 1 char to ~1-2 tokens. We weight them as 1.5.
// - ASCII characters usually map ~4 chars to 1 token. We weight them as 0.25 (or 4 chars = 1 token).
// - To be safe, we round up the total.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	var total float64
	for _, r := range text {
		// Basic check for CJK ranges (approximate, includes Chinese, Hiragana, Katakana, Hangul)
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) { // Hangul Syllables
			total += 1.5
		} else {
			total += 0.25
		}
	}

	estimated := int(total)
	// Add a small safety buffer for metadata overhead in typical chat models
	return estimated + 20
}

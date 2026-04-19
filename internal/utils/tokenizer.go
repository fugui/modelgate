package utils

import (
	"encoding/json"
	"strings"
)

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

// EstimateTokensFromOpenAIRequest extracts string content from a standard OpenAI request JSON body
// (including messages, system prompts, and tools) and estimates the total token count.
func EstimateTokensFromOpenAIRequest(body []byte) int {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}

	var contentBuilder strings.Builder

	// Traverse messages (which may contain the system prompt in OpenAI format)
	if messages, ok := payload["messages"].([]interface{}); ok {
		for _, msgObj := range messages {
			if msgMap, ok := msgObj.(map[string]interface{}); ok {
				if content, ok := msgMap["content"]; ok {
					switch v := content.(type) {
					case string:
						contentBuilder.WriteString(v)
					case []interface{}:
						// Handle complex content arrays (like vision/multi-modal text blocks)
						for _, blockObj := range v {
							if blockMap, ok := blockObj.(map[string]interface{}); ok {
								if bType, _ := blockMap["type"].(string); bType == "text" {
									if bText, _ := blockMap["text"].(string); bText != "" {
										contentBuilder.WriteString(bText)
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Traverse tools/functions to account for their schema token cost
	if tools, ok := payload["tools"].([]interface{}); ok {
		for _, toolObj := range tools {
			if b, err := json.Marshal(toolObj); err == nil {
				contentBuilder.Write(b)
			}
		}
	}

	return EstimateTokens(contentBuilder.String())
}

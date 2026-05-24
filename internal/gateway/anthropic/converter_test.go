package anthropic

import (
	"encoding/json"
	"testing"
)

func TestThoughtSignatureCaching(t *testing.T) {
	// Reset global state
	ClearThoughtSignatureCache()

	toolID := "toolu_test123"
	extraContent := map[string]interface{}{
		"google": map[string]interface{}{
			"thought_signature": "signature_abc_123",
		},
	}

	// Test in-memory only
	CacheThoughtSignature(toolID, extraContent)
	retrieved, found := GetThoughtSignature(toolID)
	if !found {
		t.Fatalf("Expected thought signature to be found in memory cache")
	}

	retrievedMap, ok := retrieved.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected retrieved signature to be map[string]interface{}")
	}

	google, ok := retrievedMap["google"].(map[string]interface{})
	if !ok || google["thought_signature"] != "signature_abc_123" {
		t.Errorf("Retrieved signature does not match: %v", retrieved)
	}

	// Test padding/trimming toolu_ prefix in-memory
	trimmedID := "test123"
	if _, ok := GetThoughtSignature(trimmedID); !ok {
		t.Errorf("Expected GetThoughtSignature to resolve trimmed tool_call ID")
	}
}

func TestStreamingThoughtSignatureExtraction(t *testing.T) {
	// Reset global state
	ClearThoughtSignatureCache()

	originalReq := &MessagesRequest{Model: "gemini-3-flash-preview"}
	state := make(map[string]interface{})

	// Chunk 1: Send tool call definition (index 0, id stream_tool_123, function.name Bash)
	chunk1 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "stream_tool_123",
							"type":  "function",
							"function": map[string]interface{}{
								"name": "Bash",
							},
						},
					},
				},
			},
		},
		"created": 1776667061,
	}
	data1, _ := json.Marshal(chunk1)
	line1 := "data: " + string(data1)
	_, err := ConvertStreamLine(line1, originalReq, state)
	if err != nil {
		t.Fatalf("Failed to convert stream line 1: %v", err)
	}
	t.Logf("After Chunk 1, state: %+v", state)

	// Chunk 2: Send arguments delta
	chunk2 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"function": map[string]interface{}{
								"arguments": `{"command":"ls"}`,
							},
						},
					},
				},
			},
		},
		"created": 1776667061,
	}
	data2, _ := json.Marshal(chunk2)
	line2 := "data: " + string(data2)
	_, err = ConvertStreamLine(line2, originalReq, state)
	if err != nil {
		t.Fatalf("Failed to convert stream line 2: %v", err)
	}
	t.Logf("After Chunk 2, state: %+v", state)

	// Chunk 3: Send extra_content with thought_signature (no function or name)
	chunk3 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"extra_content": map[string]interface{}{
								"google": map[string]interface{}{
									"thought_signature": "sig_stream_xyz_789",
								},
							},
						},
					},
				},
			},
		},
		"created": 1776667062,
	}
	data3, _ := json.Marshal(chunk3)
	line3 := "data: " + string(data3)
	_, err = ConvertStreamLine(line3, originalReq, state)
	if err != nil {
		t.Fatalf("Failed to convert stream line 3: %v", err)
	}
	t.Logf("After Chunk 3, state: %+v", state)

	RangeThoughtSignatureCache(func(key, value interface{}) bool {
		t.Logf("Cache Key: %v, Value: %+v", key, value)
		return true
	})

	// Verify it was cached under toolID padded with toolu_ prefix: "toolu_stream_tool_123"
	retrieved, found := GetThoughtSignature("toolu_stream_tool_123")
	if !found {
		t.Fatalf("Expected streaming thought signature to be cached and found")
	}

	retrievedMap, ok := retrieved.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected retrieved signature to be map[string]interface{}")
	}

	google, ok := retrievedMap["google"].(map[string]interface{})
	if !ok || google["thought_signature"] != "sig_stream_xyz_789" {
		t.Errorf("Retrieved streaming signature does not match: %v", retrieved)
	}
}

func TestStreamingThoughtSignatureDelayedID(t *testing.T) {
	// Reset global state
	ClearThoughtSignatureCache()

	originalReq := &MessagesRequest{Model: "gemini-3-flash-preview"}
	state := make(map[string]interface{})

	// Chunk 1: Send tool call definition with name only (no ID)
	chunk1 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"type":  "function",
							"function": map[string]interface{}{
								"name": "Bash",
							},
						},
					},
				},
			},
		},
		"created": 1776667061,
	}
	data1, _ := json.Marshal(chunk1)
	line1 := "data: " + string(data1)
	clientLine1, err := ConvertStreamLine(line1, originalReq, state)
	if err != nil {
		t.Fatalf("Failed to convert stream line 1: %v", err)
	}
	t.Logf("clientLine1: %s", clientLine1)
	t.Logf("After Chunk 1, state: %+v", state)

	// Extract the generated random ID from state
	generatedID := state["last_tool_id"].(string)
	if generatedID == "" {
		t.Fatalf("Expected a random ID to be generated and set in last_tool_id")
	}

	// Chunk 2: Send the real backend ID
	chunk2 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "stream_tool_delayed",
						},
					},
				},
			},
		},
		"created": 1776667061,
	}
	data2, _ := json.Marshal(chunk2)
	line2 := "data: " + string(data2)
	clientLine2, err := ConvertStreamLine(line2, originalReq, state)
	if err != nil {
		t.Fatalf("Failed to convert stream line 2: %v", err)
	}
	t.Logf("clientLine2: %s", clientLine2)
	t.Logf("After Chunk 2, state: %+v", state)

	// Chunk 3: Send extra_content with thought_signature
	chunk3 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"extra_content": map[string]interface{}{
								"google": map[string]interface{}{
									"thought_signature": "sig_stream_delayed_789",
								},
							},
						},
					},
				},
			},
		},
		"created": 1776667062,
	}
	data3, _ := json.Marshal(chunk3)
	line3 := "data: " + string(data3)
	clientLine3, err := ConvertStreamLine(line3, originalReq, state)
	if err != nil {
		t.Fatalf("Failed to convert stream line 3: %v", err)
	}
	t.Logf("clientLine3: %s", clientLine3)
	t.Logf("After Chunk 3, state: %+v", state)

	// Verify the thought signature is cached under the generated random ID, not the backend ID!
	retrieved, found := GetThoughtSignature(generatedID)
	if !found {
		t.Fatalf("Expected streaming thought signature to be cached under the generated ID %s", generatedID)
	}

	retrievedMap, ok := retrieved.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected retrieved signature to be map[string]interface{}")
	}

	google, ok := retrievedMap["google"].(map[string]interface{})
	if !ok || google["thought_signature"] != "sig_stream_delayed_789" {
		t.Errorf("Retrieved streaming signature does not match: %v", retrieved)
	}
}


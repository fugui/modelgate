package proxy

import (
	"encoding/json"
	"os"
	"testing"
)

func TestOpenAIThoughtSignatureCachingAndInjection(t *testing.T) {
	// Reset global state
	ClearThoughtSignatureCache()

	toolID := "toolu_openai123"
	extraContent := map[string]interface{}{
		"google": map[string]interface{}{
			"thought_signature": "sig_openai_abc_123",
		},
	}

	// 1. Test manual cache and injection
	CacheThoughtSignature(toolID, extraContent)

	payload := &OpenAIRequestHeader{
		Messages: []ChatMessage{
			{
				Role: "assistant",
				ToolCalls: []map[string]interface{}{
					{
						"id": "openai123",
						"type": "function",
						"function": map[string]interface{}{
							"name": "Bash",
							"arguments": `{"command":"ls"}`,
						},
					},
				},
			},
		},
	}

	injectOpenAIThoughtSignatures(payload)

	// Verify that extra_content was successfully injected
	msg := payload.Messages[0]
	if len(msg.ToolCalls) == 0 {
		t.Fatalf("Expected tool calls to be present")
	}

	tc := msg.ToolCalls[0]
	ec, exists := tc["extra_content"]
	if !exists {
		t.Fatalf("Expected extra_content to be injected into tool call")
	}

	ecMap, ok := ec.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected extra_content to be map[string]interface{}")
	}

	google, ok := ecMap["google"].(map[string]interface{})
	if !ok || google["thought_signature"] != "sig_openai_abc_123" {
		t.Errorf("Injected extra_content mismatch: %v", ec)
	}

	// Verify that thought_signature was injected to root and function
	if tc["thought_signature"] != "sig_openai_abc_123" {
		t.Errorf("Expected thought_signature in tool call root to be sig_openai_abc_123")
	}
	if fn, ok := tc["function"].(map[string]interface{}); ok {
		if fn["thought_signature"] != "sig_openai_abc_123" {
			t.Errorf("Expected thought_signature in function to be sig_openai_abc_123")
		}
	}

	// 2. Test response caching (non-streaming)
	ClearThoughtSignatureCache()
	respBody := []byte(`{
		"choices": [
			{
				"message": {
					"tool_calls": [
						{
							"id": "openai_normal_789",
							"type": "function",
							"extra_content": {
								"google": {
									"thought_signature": "sig_normal_response_xyz"
								}
							}
						}
					]
				}
			}
		]
	}`)

	cacheThoughtSignaturesFromResponse(respBody)

	cached, found := GetThoughtSignature("openai_normal_789")
	if !found {
		t.Fatalf("Expected signature to be cached from non-streaming response")
	}

	cachedMap, _ := cached.(map[string]interface{})
	google2, _ := cachedMap["google"].(map[string]interface{})
	if google2["thought_signature"] != "sig_normal_response_xyz" {
		t.Errorf("Cached signature from normal response mismatch: %v", cached)
	}

	// 3. Test stream response caching
	ClearThoughtSignatureCache()
	streamState := make(map[string]interface{})

	// Line 1: Tool call start (Index 0, ID openai_stream_456)
	line1 := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"openai_stream_456"}]}}]}`
	cacheThoughtSignaturesFromStreamLine(line1, streamState)

	// Line 2: Delta extra_content
	line2 := `data: {"choices":[{"delta":{"extra_content":{"google":{"thought_signature":"sig_stream_xyz_456"}},"tool_calls":[{"index":0}]}}]}`
	cacheThoughtSignaturesFromStreamLine(line2, streamState)

	cached3, found3 := GetThoughtSignature("openai_stream_456")
	if !found3 {
		t.Fatalf("Expected signature to be cached from stream lines")
	}

	cachedMap3, _ := cached3.(map[string]interface{})
	google3, _ := cachedMap3["google"].(map[string]interface{})
	if google3["thought_signature"] != "sig_stream_xyz_456" {
		t.Errorf("Cached streaming signature mismatch: %v", cached3)
	}
}

func TestUserClientRequestFailure(t *testing.T) {
	ClearThoughtSignatureCache()

	// Cache the thought signature under toolID QV62V45e
	extraContent := map[string]interface{}{
		"google": map[string]interface{}{
			"thought_signature": "sig_opencode_test_999",
		},
	}
	CacheThoughtSignature("QV62V45e", extraContent)

	// Read docs/1_client_request.json
	data, err := os.ReadFile("../../../docs/1_client_request.json")
	if err != nil {
		t.Fatalf("Failed to read user request file: %v", err)
	}

	var payload OpenAIRequestHeader
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Failed to unmarshal user request: %v", err)
	}

	injectOpenAIThoughtSignatures(&payload)

	// Verify that extra_content was successfully injected
	foundTarget := false
	for _, msg := range payload.Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				id, _ := tc["id"].(string)
				if id == "QV62V45e" {
					foundTarget = true
					ec, exists := tc["extra_content"]
					if !exists {
						t.Errorf("Expected extra_content to be injected for tool QV62V45e")
					} else {
						ecMap, _ := ec.(map[string]interface{})
						google, _ := ecMap["google"].(map[string]interface{})
						if google["thought_signature"] != "sig_opencode_test_999" {
							t.Errorf("Injected extra_content mismatch: %v", ec)
						}
					}
				}
			}
		}
	}

	if !foundTarget {
		t.Errorf("Expected to find assistant message with tool ID QV62V45e")
	}
}

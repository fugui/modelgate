package proxy

import (
	"encoding/json"
	"testing"
)

// Create a large mock payload to simulate real-world usage with many messages
func createMockPayload(numMessages int) []byte {
	messages := make([]map[string]interface{}, numMessages)
	for i := 0; i < numMessages; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages[i] = map[string]interface{}{
			"role":    role,
			"content": "This is some dummy message content that will be used to simulate a large prompt in the modelgate proxy.",
		}
	}
	req := map[string]interface{}{
		"model":       "gpt-4",
		"stream":      true,
		"max_tokens":  2048,
		"temperature": 0.7,
		"messages":    messages,
	}
	b, _ := json.Marshal(req)
	return b
}

func BenchmarkOldMapApproach(b *testing.B) {
	data := createMockPayload(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var payload map[string]interface{}
		_ = json.Unmarshal(data, &payload)

		// Simulate parameter injection
		if _, exists := payload["temperature"]; !exists {
			payload["temperature"] = 0.7
		}

		// Simulate model name mapping
		payload["model"] = "gpt-4o-new"

		// Simulate marshaling
		_, _ = json.Marshal(payload)
	}
}

func BenchmarkNewOptimizedApproach(b *testing.B) {
	data := createMockPayload(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var payload OpenAIRequestHeader
		_ = json.Unmarshal(data, &payload)

		// Simulate parameter injection
		payload.InjectParams(map[string]interface{}{
			"temperature": 0.7,
		})

		// Simulate model name mapping
		payload.Model = "gpt-4o-new"

		// Simulate marshaling
		_, _ = json.Marshal(&payload)
	}
}

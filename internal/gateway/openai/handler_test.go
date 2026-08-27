package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProtocol(t *testing.T) {
	proto := &Protocol{}

	// Test BackendPath
	assert.Equal(t, "/v1/chat/completions", proto.BackendPath())

	// Test PingMessage
	assert.Equal(t, "", proto.PingMessage())

	// Test BuildErrorResponse
	errBytes := proto.BuildErrorResponse("authentication_error", "invalid api key")
	var errObj map[string]interface{}
	err := json.Unmarshal(errBytes, &errObj)
	require.NoError(t, err)
	errBody, ok := errObj["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "authentication_error", errBody["type"])
	assert.Equal(t, "invalid api key", errBody["message"])

	// Test ExtractUsage normal response
	respJSON := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4o",
		"choices": [{"message": {"role": "assistant", "content": "hello"}}],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 25,
			"total_tokens": 125
		}
	}`
	inTokens, outTokens := proto.ExtractUsage([]byte(respJSON))
	assert.Equal(t, 100, inTokens)
	assert.Equal(t, 25, outTokens)

	// Test ExtractUsage with no usage
	inTokens, outTokens = proto.ExtractUsage([]byte(`{"id": "chatcmpl-123"}`))
	assert.Equal(t, 0, inTokens)
	assert.Equal(t, 0, outTokens)

	// Test ExtractStreamUsage with usage in chunk
	streamLine := `data: {"id":"chatcmpl-123","choices":[],"usage":{"prompt_tokens":80,"completion_tokens":40,"total_tokens":120}}`
	inTokens, outTokens = proto.ExtractStreamUsage(streamLine)
	assert.Equal(t, 80, inTokens)
	assert.Equal(t, 40, outTokens)

	// Test ExtractStreamUsage with delta chunk (no usage)
	deltaLine := `data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"hi"}}]}`
	inTokens, outTokens = proto.ExtractStreamUsage(deltaLine)
	assert.Equal(t, 0, inTokens)
	assert.Equal(t, 0, outTokens)

	// Test ExtractStreamUsage with [DONE]
	inTokens, outTokens = proto.ExtractStreamUsage("data: [DONE]")
	assert.Equal(t, 0, inTokens)
	assert.Equal(t, 0, outTokens)
}

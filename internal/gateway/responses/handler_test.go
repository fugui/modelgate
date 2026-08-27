package responses

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesProtocol(t *testing.T) {
	proto := &Protocol{}

	// Test BackendPath
	assert.Equal(t, "/v1/responses", proto.BackendPath())

	// Test PingMessage
	assert.Equal(t, ": ping\n\n", proto.PingMessage())

	// Test BuildErrorResponse
	errBytes := proto.BuildErrorResponse("invalid_request_error", "missing model")
	var errObj map[string]interface{}
	err := json.Unmarshal(errBytes, &errObj)
	require.NoError(t, err)
	errBody, ok := errObj["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "invalid_request_error", errBody["type"])
	assert.Equal(t, "missing model", errBody["message"])

	// Test ExtractUsage
	respJSON := `{
		"id": "resp_123",
		"object": "response",
		"status": "completed",
		"model": "gpt-4o",
		"usage": {
			"input_tokens": 150,
			"output_tokens": 42,
			"total_tokens": 192
		}
	}`
	inTokens, outTokens := proto.ExtractUsage([]byte(respJSON))
	assert.Equal(t, 150, inTokens)
	assert.Equal(t, 42, outTokens)

	// Test ExtractUsage with no usage
	inTokens, outTokens = proto.ExtractUsage([]byte(`{"id": "resp_123"}`))
	assert.Equal(t, 0, inTokens)
	assert.Equal(t, 0, outTokens)

	// Test ExtractStreamUsage with response.completed event
	streamLine := `data: {"type":"response.completed","response":{"id":"resp_123","status":"completed","usage":{"input_tokens":120,"output_tokens":35}}}`
	inTokens, outTokens = proto.ExtractStreamUsage(streamLine)
	assert.Equal(t, 120, inTokens)
	assert.Equal(t, 35, outTokens)

	// Test ExtractStreamUsage with intermediate delta event (no usage)
	deltaLine := `data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hello"}`
	inTokens, outTokens = proto.ExtractStreamUsage(deltaLine)
	assert.Equal(t, 0, inTokens)
	assert.Equal(t, 0, outTokens)

	// Test ExtractStreamUsage with [DONE]
	inTokens, outTokens = proto.ExtractStreamUsage("data: [DONE]")
	assert.Equal(t, 0, inTokens)
	assert.Equal(t, 0, outTokens)
}

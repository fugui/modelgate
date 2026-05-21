package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseStreamResponseOpenAI(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"Hello","role":"assistant"},"index":0}]}
data: {"choices":[{"delta":{"content":" world","role":"assistant"},"index":0}]}
data: {"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"index":0}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\""}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"Paris\"}"}}]}}]}
data: [DONE]
`

	result := parseStreamResponse([]byte(body))
	expected := "Hello world\n\n[Tool Call]: get_weather({\"city\":\"Paris\"})"
	assert.Equal(t, expected, result)
}

func TestParseStreamResponseAnthropic(t *testing.T) {
	body := `event: message_start
data: {"message":{"id":"msg_1","model":"gemini-3-flash-preview","role":"assistant","type":"message","usage":{"input_tokens":0,"output_tokens":0}},"type":"message_start"}

event: content_block_start
data: {"content_block":{"text":"","type":"text"},"index":0,"type":"content_block_start"}

event: content_block_delta
data: {"delta":{"text":"我来帮你看看这个目录的情况。","type":"text_delta"},"index":0,"type":"content_block_delta"}

event: content_block_stop
data: {"index":0,"type":"content_block_stop"}

event: content_block_start
data: {"content_block":{"id":"toolu_zlh3gm1h","input":{},"name":"Bash","type":"tool_use"},"index":1,"type":"content_block_start"}

event: content_block_delta
data: {"delta":{"partial_json":"{\"command\":\"ls -R\"}","type":"input_json_delta"},"index":1,"type":"content_block_delta"}

event: content_block_stop
data: {"index":1,"type":"content_block_stop"}

event: message_stop
data: {"type":"message_stop"}
`

	result := parseStreamResponse([]byte(body))
	expected := "我来帮你看看这个目录的情况。\n\n[Tool Call]: Bash({\"command\":\"ls -R\"})"
	assert.Equal(t, expected, result)
}

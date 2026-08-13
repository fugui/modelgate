package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"modelgate/internal/config"
)

func collectStreamTexts(t *testing.T, output string) string {
	t.Helper()
	var sb strings.Builder
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") || strings.HasPrefix(line, "data: [DONE]") {
			continue
		}
		var chunk CompletionStreamResponse
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			t.Fatalf("Unmarshal chunk failed: %v (line: %s)", err, line)
		}
		if len(chunk.Choices) > 0 {
			sb.WriteString(chunk.Choices[0].Text)
		}
	}
	return sb.String()
}

func TestConvertStreamLineSticky(t *testing.T) {
	req := &CompletionRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	line := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Chunk 1\"},\"finish_reason\":null}]}\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" Chunk 2\"},\"finish_reason\":null}]}\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}"

	out, err := ConvertStreamLine(line, req, state, nil)
	if err != nil {
		t.Fatalf("ConvertStreamLine failed: %v", err)
	}

	text := collectStreamTexts(t, out)
	if text != "Chunk 1 Chunk 2" {
		t.Errorf("Expected 'Chunk 1 Chunk 2', got %q", text)
	}
}

func TestConvertStreamLineEcho(t *testing.T) {
	req := &CompletionRequest{Model: "qwen3.5-coder", Prompt: "def f():", Echo: true}
	state := make(map[string]interface{})

	line := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"return 1\"},\"finish_reason\":null}]}"
	out, err := ConvertStreamLine(line, req, state, nil)
	if err != nil {
		t.Fatalf("ConvertStreamLine failed: %v", err)
	}

	text := collectStreamTexts(t, out)
	if !strings.HasPrefix(text, "def f():") {
		t.Errorf("Expected echo prompt prefix, got %q", text)
	}
}

func TestConvertStreamLineTrailingFence(t *testing.T) {
	req := &CompletionRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	chunk1 := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"```python\\n\"},\"finish_reason\":null}]}"
	chunk2 := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"def f():\\n    pass\\n\"},\"finish_reason\":null}]}"
	chunk3 := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"```\"},\"finish_reason\":\"stop\"}]}"

	var all strings.Builder
	for _, chunk := range []string{chunk1, chunk2, chunk3} {
		out, err := ConvertStreamLine(chunk, req, state, nil)
		if err != nil {
			t.Fatalf("ConvertStreamLine failed: %v", err)
		}
		all.WriteString(out)
	}

	text := collectStreamTexts(t, all.String())
	if strings.Contains(text, "```") {
		t.Errorf("Trailing fence not stripped, got %q", text)
	}
	if text != "def f():\n    pass" {
		t.Errorf("Expected cleaned text, got %q", text)
	}
}

func TestConvertStreamLineDoneFallbackFlushesTail(t *testing.T) {
	req := &CompletionRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	chunk := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"abc\\n```\"},\"finish_reason\":null}]}"
	out, err := ConvertStreamLine(chunk, req, state, nil)
	if err != nil {
		t.Fatalf("ConvertStreamLine failed: %v", err)
	}

	doneOut, err := ConvertStreamLine("data: [DONE]", req, state, nil)
	if err != nil {
		t.Fatalf("ConvertStreamLine [DONE] failed: %v", err)
	}
	out += doneOut

	text := collectStreamTexts(t, out)
	if strings.Contains(text, "```") {
		t.Errorf("Tail fence should be stripped on [DONE], got %q", text)
	}
	if text != "abc" {
		t.Errorf("Expected 'abc', got %q", text)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Errorf("Expected [DONE] line, got:\n%s", out)
	}
}

func TestConvertStreamLineConfigTrimWhitespace(t *testing.T) {
	req := &CompletionRequest{Model: "qwen3.5-coder"}
	modelCfg := &config.ModelConfig{FIM: config.FIMConfig{TrimWhitespace: true}}
	state := make(map[string]interface{})

	chunk := "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"qwen3.5-coder\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"  hello  \"},\"finish_reason\":\"stop\"}]}"
	out, err := ConvertStreamLine(chunk, req, state, modelCfg)
	if err != nil {
		t.Fatalf("ConvertStreamLine failed: %v", err)
	}

	text := collectStreamTexts(t, out)
	if text != "hello" {
		t.Errorf("Expected trimmed 'hello', got %q", text)
	}
}

package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"modelgate/internal/config"
)

func TestConvertToOpenAINormal(t *testing.T) {
	req := &CompletionRequest{
		Model:  "qwen3.5-coder",
		Prompt: "def fib(n):",
	}

	openaiBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(openaiBody, &parsed); err != nil {
		t.Fatalf("Unmarshal result failed: %v", err)
	}

	if parsed["model"] != "qwen3.5-coder" {
		t.Errorf("Expected model 'qwen3.5-coder', got %v", parsed["model"])
	}

	msgs, ok := parsed["messages"].([]interface{})
	if !ok || len(msgs) != 1 {
		t.Fatalf("Expected 1 user message, got %v", msgs)
	}

	msgMap := msgs[0].(map[string]interface{})
	if msgMap["role"] != "user" || msgMap["content"] != "def fib(n):" {
		t.Errorf("Unexpected user message: %v", msgMap)
	}
}

func TestConvertToOpenAIFIM(t *testing.T) {
	req := &CompletionRequest{
		Model:  "qwen3.5-coder",
		Prompt: "def add(a, b):\n",
		Suffix: "\n    return result",
	}
	modelCfg := &config.ModelConfig{
		ID:  "qwen3.5-coder",
		FIM: config.FIMConfig{Enabled: true, Mode: "auto"},
	}

	openaiBody, err := ConvertToOpenAI(req, modelCfg)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(openaiBody, &parsed)
	msgs := parsed["messages"].([]interface{})
	msgMap := msgs[0].(map[string]interface{})
	content := msgMap["content"].(string)

	if !strings.Contains(content, "<|fim_prefix|>") || !strings.Contains(content, "<|fim_suffix|>") {
		t.Errorf("Expected FIM tags in content, got: %s", content)
	}
}

func TestConvertToOpenAIFIMDisabledByDefault(t *testing.T) {
	req := &CompletionRequest{
		Model:  "qwen3.5-coder",
		Prompt: "def add(a, b):\n",
		Suffix: "\n    return result",
	}

	// 未配置 fim.enabled（缺省 false）：suffix 忽略，退化为普通前缀补全
	openaiBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(openaiBody, &parsed)
	msgs := parsed["messages"].([]interface{})
	msgMap := msgs[0].(map[string]interface{})
	content := msgMap["content"].(string)

	if strings.Contains(content, "<|fim_prefix|>") {
		t.Errorf("FIM should be disabled by default, got FIM tags: %s", content)
	}
	if len(msgs) != 1 || msgMap["role"] != "user" {
		t.Errorf("Expected single user message, got: %v", msgs)
	}
}

func TestConvertToOpenAIFIMNativeForced(t *testing.T) {
	req := &CompletionRequest{
		Model:  "glm4.7",
		Prompt: "def add(a, b):\n",
		Suffix: "\n    return result",
	}
	modelCfg := &config.ModelConfig{
		ID:  "glm4.7",
		FIM: config.FIMConfig{Enabled: true, Mode: "native"},
	}

	openaiBody, err := ConvertToOpenAI(req, modelCfg)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(openaiBody, &parsed)
	msgs := parsed["messages"].([]interface{})
	msgMap := msgs[0].(map[string]interface{})
	content := msgMap["content"].(string)

	if !strings.Contains(content, "<|fim_prefix|>") {
		t.Errorf("Expected forced native FIM tags, got: %s", content)
	}
	if len(msgs) != 1 {
		t.Errorf("Expected single message in native mode, got %d", len(msgs))
	}
}

func TestConvertToOpenAIFIMCustomTags(t *testing.T) {
	req := &CompletionRequest{
		Model:  "custom-coder",
		Prompt: "def add(a, b):\n",
		Suffix: "\n    return result",
	}
	modelCfg := &config.ModelConfig{
		ID: "custom-coder",
		FIM: config.FIMConfig{
			Enabled: true,
			Mode:    "native",
			Prefix:  "<P>",
			Suffix:  "<S>",
			Middle:  "<M>",
		},
	}

	openaiBody, err := ConvertToOpenAI(req, modelCfg)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(openaiBody, &parsed)
	msgs := parsed["messages"].([]interface{})
	content := msgs[0].(map[string]interface{})["content"].(string)

	if !strings.Contains(content, "<P>") || !strings.Contains(content, "<S>") || !strings.Contains(content, "<M>") {
		t.Errorf("Expected custom FIM tags, got: %s", content)
	}
}

func TestConvertToOpenAIBestOf(t *testing.T) {
	req := &CompletionRequest{
		Model:  "qwen3.5-coder",
		Prompt: "def f():",
		BestOf: intPtr(4),
	}

	if _, err := ConvertToOpenAI(req, nil); err == nil {
		t.Fatalf("Expected error for best_of > 1, got nil")
	} else if !strings.Contains(err.Error(), "best_of") {
		t.Errorf("Expected best_of in error message, got %v", err)
	}

	// best_of == 1 忽略，正常转换
	req.BestOf = intPtr(1)
	if _, err := ConvertToOpenAI(req, nil); err != nil {
		t.Fatalf("best_of == 1 should be ignored, got error: %v", err)
	}
}

func TestConvertFromOpenAI(t *testing.T) {
	backendResp := []byte(`{
		"id": "chatcmpl-12345",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "qwen3.5-coder",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "` + "```python\\n    return a + b\\n```" + `"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`)

	req := &CompletionRequest{Model: "qwen3.5-coder", Prompt: "def add(a,b):"}
	clientRespBytes, err := ConvertFromOpenAI(backendResp, req, nil)
	if err != nil {
		t.Fatalf("ConvertFromOpenAI failed: %v", err)
	}

	var compResp CompletionResponse
	if err := json.Unmarshal(clientRespBytes, &compResp); err != nil {
		t.Fatalf("Unmarshal CompletionResponse failed: %v", err)
	}

	if compResp.Object != "text_completion" {
		t.Errorf("Expected object 'text_completion', got %s", compResp.Object)
	}

	if len(compResp.Choices) == 0 || compResp.Choices[0].Text != "    return a + b" {
		t.Errorf("Expected cleaned text '    return a + b', got %q", compResp.Choices[0].Text)
	}
}

func intPtr(v int) *int {
	return &v
}

func TestConvertFromOpenAITrimWhitespace(t *testing.T) {
	backendResp := []byte(`{
		"id": "chatcmpl-1",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "qwen3.5-coder",
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "  \n  return a + b  \n  "}, "finish_reason": "stop"}],
		"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
	}`)

	req := &CompletionRequest{Model: "qwen3.5-coder", Prompt: "def add(a,b):"}
	modelCfg := &config.ModelConfig{FIM: config.FIMConfig{TrimWhitespace: true}}
	clientRespBytes, err := ConvertFromOpenAI(backendResp, req, modelCfg)
	if err != nil {
		t.Fatalf("ConvertFromOpenAI failed: %v", err)
	}

	var compResp CompletionResponse
	_ = json.Unmarshal(clientRespBytes, &compResp)
	if compResp.Choices[0].Text != "return a + b" {
		t.Errorf("Expected trimmed text 'return a + b', got %q", compResp.Choices[0].Text)
	}
}

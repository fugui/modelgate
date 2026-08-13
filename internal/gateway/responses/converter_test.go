package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"modelgate/internal/config"
)

func TestConvertToOpenAINormal(t *testing.T) {
	req := &ResponsesRequest{
		Model:        "qwen3.5-coder",
		Instructions: "You are a coding agent.",
		Input:        "Fix the bug in main.go",
	}

	openaiBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(openaiBody, &parsed); err != nil {
		t.Fatalf("Unmarshal result failed: %v", err)
	}

	msgs, ok := parsed["messages"].([]interface{})
	if !ok || len(msgs) != 2 {
		t.Fatalf("Expected 2 messages (system + user), got %v", msgs)
	}

	sysMsg := msgs[0].(map[string]interface{})
	if sysMsg["role"] != "system" || sysMsg["content"] != "You are a coding agent." {
		t.Errorf("Unexpected system message: %v", sysMsg)
	}

	userMsg := msgs[1].(map[string]interface{})
	if userMsg["role"] != "user" || userMsg["content"] != "Fix the bug in main.go" {
		t.Errorf("Unexpected user message: %v", userMsg)
	}
}

func TestConvertToOpenAIExtendedFields(t *testing.T) {
	req := &ResponsesRequest{
		Model:      "qwen3.5-coder",
		Input:      "Generate JSON output",
		ToolChoice: "required",
		Reasoning:  &ReasoningConfig{Effort: "high"},
		Text:       &TextConfig{Format: "json_object"},
	}

	openaiBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	_ = json.Unmarshal(openaiBody, &parsed)

	if parsed["tool_choice"] != "required" {
		t.Errorf("Expected tool_choice 'required', got %v", parsed["tool_choice"])
	}
	if parsed["reasoning_effort"] != "high" {
		t.Errorf("Expected reasoning_effort 'high', got %v", parsed["reasoning_effort"])
	}

	rf, ok := parsed["response_format"].(map[string]interface{})
	if !ok || rf["type"] != "json_object" {
		t.Errorf("Expected response_format json_object, got %v", parsed["response_format"])
	}
}

func TestInputFileRejection(t *testing.T) {
	req := &ResponsesRequest{
		Model: "qwen3.5-coder",
		Input: []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "input_file",
						"file_id": "file_123",
					},
				},
			},
		},
	}

	_, err := ConvertToOpenAI(req, nil)
	if err == nil {
		t.Fatalf("Expected error for input_file content part, got nil")
	}
	if !strings.Contains(err.Error(), "input_file") {
		t.Errorf("Expected input_file in error message, got %v", err)
	}
}

func TestConvertFromOpenAI(t *testing.T) {
	backendResp := []byte(`{
		"id": "chatcmpl-responses-123",
		"object": "chat.completion",
		"created": 1755100000,
		"model": "qwen3.5-coder",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "I have fixed main.go."
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 5,
			"total_tokens": 17
		}
	}`)

	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	clientRespBytes, err := ConvertFromOpenAI(backendResp, req)
	if err != nil {
		t.Fatalf("ConvertFromOpenAI failed: %v", err)
	}

	var resp ResponsesResponse
	if err := json.Unmarshal(clientRespBytes, &resp); err != nil {
		t.Fatalf("Unmarshal ResponsesResponse failed: %v", err)
	}

	if resp.Object != "response" || resp.Status != "completed" {
		t.Errorf("Unexpected response metadata: %s, %s", resp.Object, resp.Status)
	}

	if len(resp.Output) == 0 || resp.Output[0].Content[0].Text != "I have fixed main.go." {
		t.Errorf("Unexpected output text: %+v", resp.Output)
	}
}

func TestConvertToOpenAIMaxOutputTokensField(t *testing.T) {
	req := &ResponsesRequest{
		Model:           "gpt-5",
		Input:           "hi",
		MaxOutputTokens: intPtr(2048),
	}

	// 默认映射 max_tokens
	defaultBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}
	var parsedDefault map[string]interface{}
	_ = json.Unmarshal(defaultBody, &parsedDefault)
	if _, ok := parsedDefault["max_tokens"]; !ok {
		t.Errorf("Expected max_tokens by default, got %v", parsedDefault)
	}
	if _, ok := parsedDefault["max_completion_tokens"]; ok {
		t.Errorf("Did not expect max_completion_tokens by default, got %v", parsedDefault)
	}

	// 配置切换为 max_completion_tokens
	modelCfg := &config.ModelConfig{
		ID:        "gpt-5",
		Responses: config.ResponsesConfig{MaxOutputTokensField: "max_completion_tokens"},
	}
	cfgBody, err := ConvertToOpenAI(req, modelCfg)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}
	var parsedCfg map[string]interface{}
	_ = json.Unmarshal(cfgBody, &parsedCfg)
	if v, ok := parsedCfg["max_completion_tokens"].(float64); !ok || v != 2048 {
		t.Errorf("Expected max_completion_tokens=2048, got %v", parsedCfg)
	}
	if _, ok := parsedCfg["max_tokens"]; ok {
		t.Errorf("Did not expect max_tokens with config override, got %v", parsedCfg)
	}
}

func intPtr(v int) *int {
	return &v
}

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

func TestConvertToOpenAIToolsWithBuiltInAndFlatFunctions(t *testing.T) {
	req := &ResponsesRequest{
		Model: "qwen3.5-coder",
		Input: "Search and run shell",
		Tools: []interface{}{
			map[string]interface{}{
				"type":        "function",
				"name":        "shell",
				"description": "Run shell command",
				"parameters": map[string]interface{}{
					"type":       "object",
					"$schema":    "http://json-schema.org/draft-07/schema#",
					"properties": map[string]interface{}{"command": map[string]interface{}{"type": "string"}},
				},
				"strict": true,
			},
			map[string]interface{}{
				"type":                 "web_search",
				"external_web_access": true,
			},
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "read_file",
					"description": "Read a file",
				},
			},
		},
		ToolChoice: map[string]interface{}{
			"type": "function",
			"name": "shell",
		},
	}

	openaiBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(openaiBody, &parsed); err != nil {
		t.Fatalf("Unmarshal result failed: %v", err)
	}

	tools, ok := parsed["tools"].([]interface{})
	if !ok || len(tools) != 2 {
		t.Fatalf("Expected exactly 2 tools after filtering out web_search, got %v", tools)
	}

	tool0 := tools[0].(map[string]interface{})
	if tool0["type"] != "function" {
		t.Errorf("Expected tool0 type 'function', got %v", tool0["type"])
	}
	fn0 := tool0["function"].(map[string]interface{})
	if fn0["name"] != "shell" || fn0["description"] != "Run shell command" || fn0["strict"] != true {
		t.Errorf("Unexpected fn0 structure: %v", fn0)
	}
	// Verify $schema was cleaned
	params0 := fn0["parameters"].(map[string]interface{})
	if _, hasSchema := params0["$schema"]; hasSchema {
		t.Errorf("Expected $schema to be cleaned from parameters, got %v", params0)
	}
	// Verify no nested type: function inside function
	if _, hasNestedType := fn0["type"]; hasNestedType {
		t.Errorf("Nested type field should not exist in function definition: %v", fn0)
	}

	tool1 := tools[1].(map[string]interface{})
	fn1 := tool1["function"].(map[string]interface{})
	if fn1["name"] != "read_file" {
		t.Errorf("Expected tool1 name 'read_file', got %v", fn1["name"])
	}

	tc, ok := parsed["tool_choice"].(map[string]interface{})
	if !ok || tc["type"] != "function" {
		t.Errorf("Unexpected tool_choice: %v", parsed["tool_choice"])
	}
	tcFn := tc["function"].(map[string]interface{})
	if tcFn["name"] != "shell" {
		t.Errorf("Expected tool_choice function name 'shell', got %v", tcFn["name"])
	}
}

func TestConvertToOpenAIOnlyBuiltInTools(t *testing.T) {
	req := &ResponsesRequest{
		Model: "qwen3.5-coder",
		Input: "Search web",
		Tools: []interface{}{
			map[string]interface{}{
				"type":                 "web_search",
				"external_web_access": true,
			},
		},
	}

	openaiBody, err := ConvertToOpenAI(req, nil)
	if err != nil {
		t.Fatalf("ConvertToOpenAI failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(openaiBody, &parsed); err != nil {
		t.Fatalf("Unmarshal result failed: %v", err)
	}

	if _, hasTools := parsed["tools"]; hasTools {
		t.Errorf("Expected tools to be omitted when only built-in tools are provided, got %v", parsed["tools"])
	}
	if _, hasTC := parsed["tool_choice"]; hasTC {
		t.Errorf("Expected tool_choice to be omitted when tools is empty, got %v", parsed["tool_choice"])
	}
}

func TestConvertToOpenAIParallelToolCalls(t *testing.T) {
	req := &ResponsesRequest{
		Model: "qwen3.5-coder",
		Input: []interface{}{
			map[string]interface{}{
				"type": "message",
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "input_text",
						"text": "Run two commands",
					},
				},
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "cmd1",
				"arguments": "{\"arg\": 1}",
			},
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_2",
				"name":      "cmd2",
				"arguments": "{\"arg\": 2}",
			},
			map[string]interface{}{
				"type":    "reasoning",
				"summary": "Thinking about output",
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output":  map[string]interface{}{"status": "ok"},
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_2",
				"output":  "done",
			},
		},
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
	if !ok || len(msgs) != 4 {
		t.Fatalf("Expected 4 messages (user, assistant with 2 tool_calls, tool1, tool2), got %d: %v", len(msgs), msgs)
	}

	// 1. user
	userMsg := msgs[0].(map[string]interface{})
	if userMsg["role"] != "user" || userMsg["content"] != "Run two commands" {
		t.Errorf("Unexpected user message: %v", userMsg)
	}

	// 2. assistant with 2 tool calls
	assistantMsg := msgs[1].(map[string]interface{})
	if assistantMsg["role"] != "assistant" {
		t.Errorf("Expected role assistant, got %v", assistantMsg["role"])
	}
	tcList, ok := assistantMsg["tool_calls"].([]interface{})
	if !ok || len(tcList) != 2 {
		t.Fatalf("Expected 2 tool calls merged in assistant message, got %v", assistantMsg)
	}

	// 3. tool 1 output (JSON serialized)
	toolMsg1 := msgs[2].(map[string]interface{})
	if toolMsg1["role"] != "tool" || toolMsg1["tool_call_id"] != "call_1" || toolMsg1["content"] != `{"status":"ok"}` {
		t.Errorf("Unexpected toolMsg1: %v", toolMsg1)
	}

	// 4. tool 2 output
	toolMsg2 := msgs[3].(map[string]interface{})
	if toolMsg2["role"] != "tool" || toolMsg2["tool_call_id"] != "call_2" || toolMsg2["content"] != "done" {
		t.Errorf("Unexpected toolMsg2: %v", toolMsg2)
	}
}

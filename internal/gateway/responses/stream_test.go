package responses

import (
	"strings"
	"testing"

	"modelgate/internal/config"
	"modelgate/internal/gateway/proxy"
)

func TestConvertStreamLineToResponsesEvents(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	// Chunk 1: 首包
	chunk1 := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`
	events1, err := ConvertStreamLineToResponsesEvents(chunk1, req, state, nil)
	if err != nil {
		t.Fatalf("Chunk 1 conversion failed: %v", err)
	}

	if !strings.Contains(events1, "event: response.created") {
		t.Errorf("Expected response.created event in chunk 1, got:\n%s", events1)
	}
	if !strings.Contains(events1, "event: response.output_text.delta") {
		t.Errorf("Expected response.output_text.delta event in chunk 1, got:\n%s", events1)
	}
	if !strings.Contains(events1, `"delta":"Hello"`) {
		t.Errorf("Expected delta 'Hello' in chunk 1, got:\n%s", events1)
	}

	// Chunk 2: 第二增量
	chunk2 := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"content":" World!"},"finish_reason":null}]}`
	events2, err := ConvertStreamLineToResponsesEvents(chunk2, req, state, nil)
	if err != nil {
		t.Fatalf("Chunk 2 conversion failed: %v", err)
	}

	if strings.Contains(events2, "event: response.created") {
		t.Errorf("Did not expect response.created again in chunk 2, got:\n%s", events2)
	}
	if !strings.Contains(events2, `"delta":" World!"`) {
		t.Errorf("Expected delta ' World!' in chunk 2, got:\n%s", events2)
	}

	// Chunk 3: 结束 [DONE]
	chunk3 := `data: [DONE]`
	events3, err := ConvertStreamLineToResponsesEvents(chunk3, req, state, nil)
	if err != nil {
		t.Fatalf("Chunk 3 conversion failed: %v", err)
	}

	if !strings.Contains(events3, "event: response.completed") {
		t.Errorf("Expected response.completed event in chunk 3, got:\n%s", events3)
	}
}

func TestConvertStreamLineToolAndReasoningDoneChain(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	// Reasoning & Tool calls chunk
	chunk := `data: {"id":"chatcmpl-tool-123","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"reasoning_content":"Thinking...","tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}}]},"finish_reason":"tool_calls"}]}`
	events, err := ConvertStreamLineToResponsesEvents(chunk, req, state, nil)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if !strings.Contains(events, "event: response.reasoning_summary_text.done") {
		t.Errorf("Expected reasoning summary text done event, got:\n%s", events)
	}
	if !strings.Contains(events, "event: response.function_call_arguments.done") {
		t.Errorf("Expected function call arguments done event, got:\n%s", events)
	}
	if !strings.Contains(events, "event: response.completed") {
		t.Errorf("Expected response.completed event, got:\n%s", events)
	}
}

func TestConvertStreamLineErrorEvent(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	errChunk := `data: {"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`
	events, err := ConvertStreamLineToResponsesEvents(errChunk, req, state, nil)
	if err != nil {
		t.Fatalf("Error conversion failed: %v", err)
	}

	if !strings.Contains(events, "event: response.failed") {
		t.Errorf("Expected response.failed event, got:\n%s", events)
	}
	if !strings.Contains(events, "event: error") {
		t.Errorf("Expected error event, got:\n%s", events)
	}
}

func TestConvertStreamLineStickyPackets(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	stickyChunk := "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"Chunk 1\"}}]}\ndata: {\"id\":\"chatcmpl-2\",\"choices\":[{\"delta\":{\"content\":\" Chunk 2\"}}]}"
	events, err := ConvertStreamLineToResponsesEvents(stickyChunk, req, state, nil)
	if err != nil {
		t.Fatalf("Sticky conversion failed: %v", err)
	}

	if !strings.Contains(events, `"delta":"Chunk 1"`) || !strings.Contains(events, `"delta":" Chunk 2"`) {
		t.Errorf("Expected both deltas from sticky packets, got:\n%s", events)
	}
}

func TestConvertStreamLineReasoningSummaryContent(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	state := make(map[string]interface{})

	chunk1 := `data: {"id":"chatcmpl-r1","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"reasoning_content":"Think"},"finish_reason":null}]}`
	events1, err := ConvertStreamLineToResponsesEvents(chunk1, req, state, nil)
	if err != nil {
		t.Fatalf("Chunk 1 conversion failed: %v", err)
	}
	if !strings.Contains(events1, `"delta":"Think"`) {
		t.Errorf("Expected reasoning delta, got:\n%s", events1)
	}

	chunk2 := `data: {"id":"chatcmpl-r1","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"reasoning_content":"ing..."},"finish_reason":"stop"}]}`
	events2, err := ConvertStreamLineToResponsesEvents(chunk2, req, state, nil)
	if err != nil {
		t.Fatalf("Chunk 2 conversion failed: %v", err)
	}

	if !strings.Contains(events2, `"summary":"Thinking..."`) {
		t.Errorf("Expected accumulated summary 'Thinking...' in done event, got:\n%s", events2)
	}
}

func TestConvertStreamLineReasoningTextEvent(t *testing.T) {
	req := &ResponsesRequest{Model: "qwen3.5-coder"}
	modelCfg := &config.ModelConfig{
		ID:        "qwen3.5-coder",
		Responses: config.ResponsesConfig{ReasoningEvent: "text"},
	}
	state := make(map[string]interface{})

	chunk := `data: {"id":"chatcmpl-r1","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"reasoning_content":"full reasoning"},"finish_reason":"stop"}]}`
	events, err := ConvertStreamLineToResponsesEvents(chunk, req, state, modelCfg)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if !strings.Contains(events, "event: response.reasoning_text.delta") {
		t.Errorf("Expected reasoning_text.delta event, got:\n%s", events)
	}
	if !strings.Contains(events, "event: response.reasoning_text.done") {
		t.Errorf("Expected reasoning_text.done event, got:\n%s", events)
	}
	if !strings.Contains(events, `"text":"full reasoning"`) {
		t.Errorf("Expected full reasoning text in done event, got:\n%s", events)
	}
}

func TestBuildCompletionEventsToolDoneOrder(t *testing.T) {
	// 构造乱序的 tc_state_map（map 迭代本身无序），验证 done 链按 index 升序输出
	state := map[string]interface{}{
		"resp_id":     "resp_x",
		"msg_item_id": "msg_x",
		"tc_state_map": map[int]map[string]string{
			1: {"item_id": "fc_1", "call_id": "call_1", "name": "f1", "arguments": "{}"},
			0: {"item_id": "fc_0", "call_id": "call_0", "name": "f0", "arguments": "{}"},
		},
	}

	events := buildCompletionEvents(state, nil)
	pos0 := strings.Index(events, `"item_id":"fc_0"`)
	pos1 := strings.Index(events, `"item_id":"fc_1"`)
	if pos0 == -1 || pos1 == -1 {
		t.Fatalf("Expected both tool done chains, got:\n%s", events)
	}
	if pos0 > pos1 {
		t.Errorf("Tool done chains should be ordered by index (fc_0 before fc_1), got:\n%s", events)
	}
}

func TestParseOpenAISSE(t *testing.T) {
	line := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1755100000,"model":"qwen3.5-coder","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`
	content, _, _ := proxy.ParseOpenAISSE(line)
	if content != "Hi" {
		t.Errorf("Expected 'Hi', got %q", content)
	}
}

package anthropic

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// thoughtSignatureCache 缓存 Gemini 返回的 thought_signature（按 tool_call_id 索引）
var thoughtSignatureCache sync.Map

// CacheThoughtSignature 存储 tool_call 的 thought_signature
func CacheThoughtSignature(toolCallID string, extraContent interface{}) {
	if toolCallID != "" && extraContent != nil {
		thoughtSignatureCache.Store(toolCallID, extraContent)
	}
}

// GetThoughtSignature 获取缓存的 thought_signature
func GetThoughtSignature(toolCallID string) (interface{}, bool) {
	return thoughtSignatureCache.Load(toolCallID)
}

// ToolUse 表示 Anthropic 的 tool_use 块
type ToolUse struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult 表示 Anthropic 的 tool_result 块
type ToolResult struct {
	Type       string      `json:"type"`
	ToolUseID  string      `json:"tool_use_id"`
	Content    interface{} `json:"content"`
	IsError    bool        `json:"is_error,omitempty"`
	Name       string      `json:"-"` // Internal use for OpenAI tool message name
}

// ConvertToOpenAI 将 Anthropic 请求转换为 OpenAI 格式
func ConvertToOpenAI(req *MessagesRequest) ([]byte, error) {
	var messages []map[string]interface{}

	// 添加 system 消息（如果有）
	if sysMsgs := parseSystemMessage(req.System); sysMsgs != nil {
		messages = append(messages, sysMsgs...)
	}

	toolNameMap := make(map[string]string)

	// 转换 messages
	for _, msg := range req.Messages {
		if contentArray, ok := msg.Content.([]interface{}); ok {
			parsed := parseAnthropicContentElements(contentArray)
			
			for _, tu := range parsed.toolUses {
				toolNameMap[tu.ID] = tu.Name
			}
			for i, tr := range parsed.toolResults {
				if name, ok := toolNameMap[tr.ToolUseID]; ok {
					parsed.toolResults[i].Name = name
				}
			}

			msgs := buildOpenAIMessages(msg.Role, parsed)
			messages = append(messages, msgs...)
		} else if contentStr, ok := msg.Content.(string); ok {
			messages = append(messages, map[string]interface{}{
				"role":    msg.Role,
				"content": contentStr,
			})
		}
	}

	openaiReq := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   req.Stream,
	}

	// 可选参数
	if req.MaxTokens > 0 {
		openaiReq["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		openaiReq["temperature"] = req.Temperature
	}
	if req.TopP > 0 {
		openaiReq["top_p"] = req.TopP
	}
	if len(req.StopSequences) > 0 {
		openaiReq["stop"] = req.StopSequences
	}

	// 转换 tools
	if len(req.Tools) > 0 {
		var openaiTools []map[string]interface{}
		for _, tool := range req.Tools {
			openaiTools = append(openaiTools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.InputSchema,
				},
			})
		}
		openaiReq["tools"] = openaiTools
	}

	return json.Marshal(openaiReq)
}

func parseSystemMessage(system interface{}) []map[string]interface{} {
	if system == nil {
		return nil
	}
	switch v := system.(type) {
	case string:
		if v != "" {
			return []map[string]interface{}{{"role": "system", "content": v}}
		}
	case []interface{}:
		var systemTexts []string
		for _, block := range v {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockType, ok := blockMap["type"].(string); ok && blockType == "text" {
					if text, ok := blockMap["text"].(string); ok {
						systemTexts = append(systemTexts, text)
					}
				}
			}
		}
		if len(systemTexts) > 0 {
			return []map[string]interface{}{{"role": "system", "content": strings.Join(systemTexts, "\n")}}
		}
	}
	return nil
}

type parsedAnthropicContent struct {
	openaiContent   []interface{}
	toolUses        []ToolUse
	toolResults     []ToolResult
	thinkingContent string
}

func parseAnthropicContentElements(contentArray []interface{}) parsedAnthropicContent {
	var parsed parsedAnthropicContent
	for _, block := range contentArray {
		if blockMap, ok := block.(map[string]interface{}); ok {
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				if text, ok := blockMap["text"].(string); ok {
					parsed.openaiContent = append(parsed.openaiContent, map[string]interface{}{
						"type": "text",
						"text": text,
					})
				}
			case "thinking":
				if thinking, ok := blockMap["thinking"].(string); ok {
					parsed.thinkingContent = thinking
				}
			case "image":
				if source, ok := blockMap["source"].(map[string]interface{}); ok {
					mediaType, _ := source["media_type"].(string)
					data, _ := source["data"].(string)
					// Only keep non-empty standard types
					if mediaType != "" && data != "" {
						parsed.openaiContent = append(parsed.openaiContent, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
							},
						})
					}
				}
			case "tool_use":
				toolUse := ToolUse{Type: "tool_use"}
				if id, ok := blockMap["id"].(string); ok {
					toolUse.ID = id
				}
				if name, ok := blockMap["name"].(string); ok {
					toolUse.Name = name
				}
				if input, ok := blockMap["input"]; ok {
					if inputData, err := json.Marshal(input); err == nil {
						toolUse.Input = inputData
					} else {
						toolUse.Input = []byte("{}")
					}
				}
				parsed.toolUses = append(parsed.toolUses, toolUse)
			case "tool_result":
				toolResult := ToolResult{Type: "tool_result"}
				if toolUseID, ok := blockMap["tool_use_id"].(string); ok {
					toolResult.ToolUseID = toolUseID
				}
				if content, ok := blockMap["content"]; ok {
					toolResult.Content = content
				}
				if isError, ok := blockMap["is_error"].(bool); ok {
					toolResult.IsError = isError
				}
				parsed.toolResults = append(parsed.toolResults, toolResult)
			}
		}
	}
	return parsed
}

func buildOpenAIMessages(role string, parsed parsedAnthropicContent) []map[string]interface{} {
	var messages []map[string]interface{}

	if role == "assistant" {
		openaiMsg := map[string]interface{}{
			"role":    "assistant",
			"content": parsed.openaiContent,
		}
		if parsed.thinkingContent != "" {
			openaiMsg["reasoning_content"] = parsed.thinkingContent
		}

		if len(parsed.toolUses) > 0 {
			if len(parsed.openaiContent) == 0 {
				openaiMsg["content"] = nil
			}
			var toolCalls []map[string]interface{}
			for _, toolUse := range parsed.toolUses {
				toolID := toolUse.ID
				if strings.HasPrefix(toolID, "toolu_") {
					toolID = strings.TrimPrefix(toolID, "toolu_")
				}
				tc := map[string]interface{}{
					"id":   toolID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      toolUse.Name,
						"arguments": string(toolUse.Input),
					},
				}
				// 注入缓存的 Gemini thought_signature
				if extra, ok := GetThoughtSignature(toolUse.ID); ok {
					tc["extra_content"] = extra
				}
				toolCalls = append(toolCalls, tc)
			}
			openaiMsg["tool_calls"] = toolCalls
		}
		messages = append(messages, openaiMsg)
	} else if role == "user" && len(parsed.toolResults) > 0 {
		for _, toolResult := range parsed.toolResults {
			toolID := toolResult.ToolUseID
			if strings.HasPrefix(toolID, "toolu_") {
				toolID = strings.TrimPrefix(toolID, "toolu_")
			}
			toolMsg := map[string]interface{}{
				"role":         "tool",
				"content":      convertToolResultContent(toolResult.Content),
				"tool_call_id": toolID,
			}
			if toolResult.Name != "" {
				toolMsg["name"] = toolResult.Name
			}
			messages = append(messages, toolMsg)
		}
		if len(parsed.openaiContent) > 0 {
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": parsed.openaiContent,
			})
		}
	} else {
		openaiMsg := map[string]interface{}{
			"role":    role,
			"content": parsed.openaiContent,
		}
		if len(parsed.openaiContent) == 1 {
			if block, ok := parsed.openaiContent[0].(map[string]interface{}); ok {
				if block["type"] == "text" {
					openaiMsg["content"] = block["text"]
				}
			}
		}
		messages = append(messages, openaiMsg)
	}

	return messages
}

func convertToolResultContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, block := range v {
			if bMap, ok := block.(map[string]interface{}); ok {
				if bType, _ := bMap["type"].(string); bType == "text" {
					if bText, _ := bMap["text"].(string); bText != "" {
						parts = append(parts, bText)
					}
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
	default:
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
	}
	return ""
}

// ConvertFromOpenAI 将 OpenAI 响应转换为 Anthropic 格式
func ConvertFromOpenAI(body []byte, originalReq *MessagesRequest) ([]byte, error) {
	var openaiResp map[string]interface{}
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	// 提取 content
	choices, ok := openaiResp["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("invalid choices in OpenAI response")
	}

	choice := choices[0].(map[string]interface{})
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message in OpenAI response")
	}

	content, _ := message["content"].(string)
	reasoning, _ := message["reasoning_content"].(string)

	// 提取 usage
	var inputTokens, outputTokens int
	if usage, ok := openaiResp["usage"].(map[string]interface{}); ok {
		if pt, ok := usage["prompt_tokens"].(float64); ok {
			inputTokens = int(pt)
		}
		if ct, ok := usage["completion_tokens"].(float64); ok {
			outputTokens = int(ct)
		}
	}

	// 提取 finish_reason
	var stopReason *string
	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		// Gemini 在有 tool_calls 时仍返回 "stop"，需要修正
		if fr == "stop" {
			if toolCalls, ok := message["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
				fr = "tool_calls"
			}
		}
		anthropicStop := convertStopReason(fr)
		stopReason = &anthropicStop
	}

	// 生成 ID
	id, _ := openaiResp["id"].(string)
	if id == "" {
		id = generateID()
	}

	// 构建 content blocks
	var contentBlocks []Block

	// 添加推理内容（如果有）
	if reasoning != "" {
		contentBlocks = append(contentBlocks, Block{
			Type:     "thinking",
			Thinking: reasoning,
		})
	}

	// 添加文本内容（如果有）
	if content != "" {
		contentBlocks = append(contentBlocks, Block{
			Type: "text",
			Text: content,
		})
	}

	// 提取 tool_calls 并转换为 tool_use blocks
	if toolCalls, ok := message["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
		for _, tc := range toolCalls {
			if toolCall, ok := tc.(map[string]interface{}); ok {
				// 提取 tool call 信息
				toolID, _ := toolCall["id"].(string)
				
				// 缓存 Gemini 的 extra_content（含 thought_signature）
				if extraContent, ok := toolCall["extra_content"]; ok {
					CacheThoughtSignature(toolID, extraContent)
				}
				// 提取 function 信息
				if function, ok := toolCall["function"].(map[string]interface{}); ok {
					name, _ := function["name"].(string)
					arguments, _ := function["arguments"].(string)

					// 解析 arguments JSON
					var inputMap map[string]interface{}
					if arguments != "" {
						if err := json.Unmarshal([]byte(arguments), &inputMap); err != nil {
							// 解析失败时使用空对象
							inputMap = make(map[string]interface{})
						}
					}

					// 创建 tool_use block
					toolUseBlock := Block{
						Type: "tool_use",
						ID:   toolID,
						Name: name,
					}

					// 将 input 序列化为 JSON
					if inputData, err := json.Marshal(inputMap); err == nil {
						toolUseBlock.Input = inputData
					} else {
						toolUseBlock.Input = []byte("{}")
					}

					contentBlocks = append(contentBlocks, toolUseBlock)
				}
			}
		}
	}

	// 如果没有 content blocks，添加一个空的 text block
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, Block{
			Type: "text",
			Text: "",
		})
	}

	anthropicResp := MessagesResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      originalReq.Model,
		Content:    contentBlocks,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}

	return json.Marshal(anthropicResp)
}

// ConvertStreamLine 转换流式响应的每一行
type StreamParser struct {
	originalReq *MessagesRequest
	state       map[string]interface{}
	sb          strings.Builder
}

func (p *StreamParser) ParseLine(line string) (string, error) {
	if line == "\n" || line == "" || strings.HasPrefix(line, ":") {
		return line, nil
	}
	if !strings.HasPrefix(line, "data: ") {
		return line, nil
	}

	data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
	if data == "[DONE]" {
		return p.handleDone(), nil
	}

	var openaiStream map[string]interface{}
	if err := json.Unmarshal([]byte(data), &openaiStream); err != nil {
		return line, nil
	}

	choices, ok := openaiStream["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return p.handleUsageOrOther(openaiStream, line), nil
	}

	choice := choices[0].(map[string]interface{})
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return line, nil
	}

	msgID, _ := openaiStream["id"].(string)
	p.handleMessageStart(msgID)
	p.handleThinkingDelta(delta)
	p.handleTextDelta(delta)
	p.handleToolCalls(delta)
	p.handleFinishReason(choice)

	return p.sb.String(), nil
}

func (p *StreamParser) getBlockIndex(key string) int {
	if val, ok := p.state[key].(int); ok {
		return val
	}
	nextIdx, _ := p.state["next_block_index"].(int)
	p.state[key] = nextIdx
	p.state["next_block_index"] = nextIdx + 1
	return nextIdx
}

func (p *StreamParser) getToolBlockIndex(openaiIdx int) int {
	toolMap, ok := p.state["tool_index_map"].(map[int]int)
	if !ok {
		toolMap = make(map[int]int)
		p.state["tool_index_map"] = toolMap
	}
	if anthropicIdx, exists := toolMap[openaiIdx]; exists {
		return anthropicIdx
	}
	nextIdx, _ := p.state["next_block_index"].(int)
	toolMap[openaiIdx] = nextIdx
	p.state["next_block_index"] = nextIdx + 1
	return nextIdx
}

func (p *StreamParser) emitEvent(eventType string, data interface{}) {
	ej, _ := json.Marshal(data)
	p.sb.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(ej)))
}

func (p *StreamParser) stopActiveBlock() {
	if active, ok := p.state["active_block_index"].(int); ok && active >= 0 {
		p.emitEvent("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": active,
		})
		p.state["active_block_index"] = -1
	}
}

func (p *StreamParser) handleDone() string {
	p.stopActiveBlock()
	p.emitEvent("message_stop", map[string]interface{}{"type": "message_stop"})
	return p.sb.String()
}

func (p *StreamParser) handleUsageOrOther(stream map[string]interface{}, originalLine string) string {
	if usage, ok := stream["usage"].(map[string]interface{}); ok {
		var input, output int
		if pt, ok := usage["prompt_tokens"].(float64); ok { input = int(pt) }
		if ct, ok := usage["completion_tokens"].(float64); ok { output = int(ct) }
		p.emitEvent("message_delta", map[string]interface{}{
			"type": "message_delta",
			"usage": map[string]interface{}{
				"output_tokens": output,
				"input_tokens":  input,
			},
		})
		return p.sb.String()
	}
	return originalLine
}

func (p *StreamParser) handleMessageStart(msgID string) {
	if started, _ := p.state["started_message"].(bool); !started {
		p.state["started_message"] = true
		if msgID == "" {
			msgID = "msg_gen_" + generateID()
		}
		p.emitEvent("message_start", map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    msgID,
				"type":  "message",
				"role":  "assistant",
				"model": p.originalReq.Model,
				"usage": map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
			},
		})
	}
}

func (p *StreamParser) handleThinkingDelta(delta map[string]interface{}) {
	if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
		anthropicIdx := p.getBlockIndex("thinking_index")

		if active, ok := p.state["active_block_index"].(int); ok && active >= 0 && active != anthropicIdx {
			p.stopActiveBlock()
		}

		if started, _ := p.state["started_thinking"].(bool); !started {
			p.state["started_thinking"] = true
			p.state["active_block_index"] = anthropicIdx
			p.emitEvent("content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": anthropicIdx,
				"content_block": map[string]interface{}{"type": "thinking", "thinking": ""},
			})
		}
		p.emitEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": anthropicIdx,
			"delta": map[string]interface{}{"type": "thinking_delta", "thinking": reasoning},
		})
	}
}

func (p *StreamParser) handleTextDelta(delta map[string]interface{}) {
	if content, ok := delta["content"].(string); ok && content != "" {
		anthropicIdx := p.getBlockIndex("text_index")

		if active, ok := p.state["active_block_index"].(int); ok && active >= 0 && active != anthropicIdx {
			p.stopActiveBlock()
		}

		if started, _ := p.state["started_text"].(bool); !started {
			p.state["started_text"] = true
			p.state["active_block_index"] = anthropicIdx
			p.emitEvent("content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": anthropicIdx,
				"content_block": map[string]interface{}{"type": "text", "text": ""},
			})
		}
		p.emitEvent("content_block_delta", map[string]interface{}{
			"type":  "content_block_delta",
			"index": anthropicIdx,
			"delta": map[string]interface{}{"type": "text_delta", "text": content},
		})
	}
}

func (p *StreamParser) handleToolCalls(delta map[string]interface{}) {
	if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
		p.state["tool_calls_seen"] = true

		for _, tc := range toolCalls {
			if tcMap, ok := tc.(map[string]interface{}); ok {
				idx, _ := tcMap["index"].(float64)
				openaiIdx := int(idx)
				anthropicIdx := p.getToolBlockIndex(openaiIdx)

				// Stop the active block if we are transitioning from a different block
				// (e.g. from text, thinking, or a previous tool call)
				if active, ok := p.state["active_block_index"].(int); ok && active >= 0 && active != anthropicIdx {
					p.stopActiveBlock()
				}

				if function, ok := tcMap["function"].(map[string]interface{}); ok {
					if name, ok := function["name"].(string); ok && name != "" {
						toolID, _ := tcMap["id"].(string)
						if toolID == "" {
							toolID = "toolu_" + generateID()
						} else if !strings.HasPrefix(toolID, "toolu_") {
							toolID = "toolu_" + toolID
						}
						// 缓存 Gemini 的 extra_content（含 thought_signature）
						if extraContent, ok := tcMap["extra_content"]; ok {
							CacheThoughtSignature(toolID, extraContent)
						}
						p.state["active_block_index"] = anthropicIdx
						p.emitEvent("content_block_start", map[string]interface{}{
							"type":  "content_block_start",
							"index": anthropicIdx,
							"content_block": map[string]interface{}{
								"type":  "tool_use",
								"id":    toolID,
								"name":  name,
								"input": map[string]interface{}{},
							},
						})
					}
					if args, ok := function["arguments"].(string); ok && args != "" {
						p.state["active_block_index"] = anthropicIdx
						p.emitEvent("content_block_delta", map[string]interface{}{
							"type":  "content_block_delta",
							"index": anthropicIdx,
							"delta": map[string]interface{}{
								"type":         "input_json_delta",
								"partial_json": args,
							},
						})
					}
				}
			}
		}
	}
}

func (p *StreamParser) handleFinishReason(choice map[string]interface{}) {
	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		// Gemini 在有 tool_calls 时仍返回 "stop"，需要修正为 "tool_calls"
		if fr == "stop" {
			if seen, _ := p.state["tool_calls_seen"].(bool); seen {
				fr = "tool_calls"
			}
		}
		p.stopActiveBlock()
		p.emitEvent("message_delta", map[string]interface{}{
			"type": "message_delta",
			"delta": map[string]interface{}{"stop_reason": convertStopReason(fr)},
		})
	}
}

// ConvertStreamLine 转换流式响应的每一行
func ConvertStreamLine(line string, originalReq *MessagesRequest, state map[string]interface{}) (string, error) {
	parser := &StreamParser{
		originalReq: originalReq,
		state:       state,
	}
	return parser.ParseLine(line)
}

// convertStopReason 转换 stop reason
func convertStopReason(openaiReason string) string {
	switch openaiReason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "content_filter"
	case "tool_calls":
		return "tool_use"
	default:
		return openaiReason
	}
}

// generateID 生成消息 ID
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

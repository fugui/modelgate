package anthropic

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

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
}

// ConvertToOpenAI 将 Anthropic 请求转换为 OpenAI 格式
func ConvertToOpenAI(req *MessagesRequest) ([]byte, error) {
	// 构建 OpenAI 格式的 messages
	var messages []map[string]interface{}

	// 添加 system 消息（如果有）
	if req.System != nil {
		switch v := req.System.(type) {
		case string:
			if v != "" {
				messages = append(messages, map[string]interface{}{
					"role":    "system",
					"content": v,
				})
			}
		case []interface{}:
			// system 可能是 content blocks 数组
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
				messages = append(messages, map[string]interface{}{
					"role":    "system",
					"content": strings.Join(systemTexts, "\n"),
				})
			}
		}
	}

	// 转换 messages
	for _, msg := range req.Messages {
		role := msg.Role

		// 处理 content 数组格式（包含 text, image, thinking, tool_use 和 tool_result）
		if contentArray, ok := msg.Content.([]interface{}); ok {
			var openaiContent []interface{}
			var toolUses []ToolUse
			var toolResults []ToolResult
			var thinkingContent string

			for _, block := range contentArray {
				if blockMap, ok := block.(map[string]interface{}); ok {
					blockType, _ := blockMap["type"].(string)

					switch blockType {
					case "text":
						if text, ok := blockMap["text"].(string); ok {
							openaiContent = append(openaiContent, map[string]interface{}{
								"type": "text",
								"text": text,
							})
						}
					case "thinking":
						// 处理 Anthropic 思考块
						if thinking, ok := blockMap["thinking"].(string); ok {
							thinkingContent = thinking
						}
					case "image":
						if source, ok := blockMap["source"].(map[string]interface{}); ok {
							mediaType, _ := source["media_type"].(string)
							data, _ := source["data"].(string)
							if mediaType != "" && data != "" {
								openaiContent = append(openaiContent, map[string]interface{}{
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
						toolUses = append(toolUses, toolUse)
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
						toolResults = append(toolResults, toolResult)
					}
				}
			}

			// 根据角色和提取的内容构建 OpenAI 消息
			if role == "assistant" {
				openaiMsg := map[string]interface{}{
					"role":    "assistant",
					"content": openaiContent,
				}
				if thinkingContent != "" {
					openaiMsg["reasoning_content"] = thinkingContent
				}
				
				if len(toolUses) > 0 {
					if len(openaiContent) == 0 {
						openaiMsg["content"] = nil
					}
					var toolCalls []map[string]interface{}
					for _, toolUse := range toolUses {
						toolCall := map[string]interface{}{
							"id":   toolUse.ID,
							"type": "function",
							"function": map[string]interface{}{
								"name":      toolUse.Name,
								"arguments": string(toolUse.Input),
							},
						}
						toolCalls = append(toolCalls, toolCall)
					}
					openaiMsg["tool_calls"] = toolCalls
				}
				messages = append(messages, openaiMsg)
			} else if role == "user" && len(toolResults) > 0 {
				// 关键修复：工具结果必须紧随 assistant 消息，不能被任何 user 消息中断。
				// Anthropic 允许在一个 user 消息中混合文本和工具结果，但 OpenAI 要求必须先回复工具调用。
				for _, toolResult := range toolResults {
					var contentStr string
					switch v := toolResult.Content.(type) {
					case string:
						contentStr = v
					case []interface{}:
						// 处理 Anthropic 的 content 数组格式（常见于 tool_result 包含 text 块）
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
							contentStr = strings.Join(parts, "\n")
						} else {
							// 如果没有文本，回退到 JSON
							if data, err := json.Marshal(v); err == nil {
								contentStr = string(data)
							}
						}
					default:
						if data, err := json.Marshal(v); err == nil {
							contentStr = string(data)
						}
					}

					messages = append(messages, map[string]interface{}{
						"role":         "tool",
						"content":      contentStr,
						"tool_call_id": toolResult.ToolUseID,
					})
				}
				
				// 如果该 user 消息中还有剩余的文本或图片（例如 Claude 的解释或后续提问），
				// 在工具调用完全回复后，作为下一条 user 消息发送。
				if len(openaiContent) > 0 {
					messages = append(messages, map[string]interface{}{
						"role":    "user",
						"content": openaiContent,
					})
				}
			} else {
				openaiMsg := map[string]interface{}{
					"role":    role,
					"content": openaiContent,
				}
				if len(openaiContent) == 1 {
					if block, ok := openaiContent[0].(map[string]interface{}); ok {
						if block["type"] == "text" {
							openaiMsg["content"] = block["text"]
						}
					}
				}
				messages = append(messages, openaiMsg)
			}
		} else if contentStr, ok := msg.Content.(string); ok {
			messages = append(messages, map[string]interface{}{
				"role":    role,
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

	// 转换 tools（如果有）
	if len(req.Tools) > 0 {
		var openaiTools []map[string]interface{}
		for _, tool := range req.Tools {
			openaiTool := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        tool.Name,
					"description": tool.Description,
					"parameters":  tool.InputSchema,
				},
			}
			openaiTools = append(openaiTools, openaiTool)
		}
		openaiReq["tools"] = openaiTools
	}

	return json.Marshal(openaiReq)
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
func ConvertStreamLine(line string, originalReq *MessagesRequest) (string, error) {
	// 处理空行或注释
	if line == "\n" || line == "" || strings.HasPrefix(line, ":") {
		return line, nil
	}

	// OpenAI 格式: data: {...}
	if !strings.HasPrefix(line, "data: ") {
		return line, nil
	}

	data := strings.TrimPrefix(line, "data: ")
	data = strings.TrimSpace(data)

	// 处理 [DONE]
	if data == "[DONE]" {
		return "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", nil
	}

	// 解析 OpenAI 流式响应
	var openaiStream map[string]interface{}
	if err := json.Unmarshal([]byte(data), &openaiStream); err != nil {
		return line, nil
	}

	choices, ok := openaiStream["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		// 处理最后的 Usage 事件
		if usage, ok := openaiStream["usage"].(map[string]interface{}); ok {
			var inputTokens, outputTokens int
			if pt, ok := usage["prompt_tokens"].(float64); ok {
				inputTokens = int(pt)
			}
			if ct, ok := usage["completion_tokens"].(float64); ok {
				outputTokens = int(ct)
			}
			event := map[string]interface{}{
				"type": "message_delta",
				"usage": map[string]interface{}{
					"output_tokens": outputTokens,
					"input_tokens":  inputTokens,
				},
			}
			ej, _ := json.Marshal(event)
			return fmt.Sprintf("event: message_delta\ndata: %s\n\n", ej), nil
		}
		return line, nil
	}

	choice := choices[0].(map[string]interface{})
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return line, nil
	}

	var sb strings.Builder

	// 1. 处理消息开始 (message_start)
	if _, hasRole := delta["role"]; hasRole {
		msgID, _ := openaiStream["id"].(string)
		if msgID == "" {
			msgID = "msg_gen_" + generateID()
		}
		event := map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    msgID,
				"type":  "message",
				"role":  "assistant",
				"model": originalReq.Model,
				"usage": map[string]interface{}{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		}
		ej, _ := json.Marshal(event)
		sb.WriteString(fmt.Sprintf("event: message_start\ndata: %s\n\n", ej))
	}

	// 2. 处理推理内容 (thinking delta)
	if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
		event := map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]interface{}{
				"type":     "thinking_delta",
				"thinking": reasoning,
			},
		}
		ej, _ := json.Marshal(event)
		sb.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", ej))
	}

	// 3. 处理文本内容 (content_block_delta)
	if content, ok := delta["content"].(string); ok && content != "" {
		event := map[string]interface{}{
			"type":  "content_block_delta",
			"index": 1, // 推理通常在 0，文本在 1
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": content,
			},
		}
		ej, _ := json.Marshal(event)
		sb.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", ej))
	}

	// 4. 处理工具调用 (tool_calls)
	if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
		for _, tc := range toolCalls {
			if tcMap, ok := tc.(map[string]interface{}); ok {
				idx, _ := tcMap["index"].(float64)
				anthropicIdx := int(idx) + 2 // 0:thinking, 1:text, 2+:tools

				if function, ok := tcMap["function"].(map[string]interface{}); ok {
					// 4.1 处理工具开始 (content_block_start)
					if name, ok := function["name"].(string); ok && name != "" {
						toolID, _ := tcMap["id"].(string)
						startEvent := map[string]interface{}{
							"type":  "content_block_start",
							"index": anthropicIdx,
							"content_block": map[string]interface{}{
								"type": "tool_use",
								"id":   toolID,
								"name": name,
							},
						}
						ej, _ := json.Marshal(startEvent)
						sb.WriteString(fmt.Sprintf("event: content_block_start\ndata: %s\n\n", ej))
					}

					// 4.2 处理参数增量 (content_block_delta)
					if args, ok := function["arguments"].(string); ok && args != "" {
						deltaEvent := map[string]interface{}{
							"type":  "content_block_delta",
							"index": anthropicIdx,
							"delta": map[string]interface{}{
								"type":         "input_json_delta",
								"partial_json": args,
							},
						}
						ej, _ := json.Marshal(deltaEvent)
						sb.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", ej))
					}
				}
			}
		}
	}

	// 5. 处理结束原因 (message_delta)
	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		stopReason := convertStopReason(fr)
		event := map[string]interface{}{
			"type": "message_delta",
			"delta": map[string]interface{}{
				"stop_reason": stopReason,
			},
		}
		ej, _ := json.Marshal(event)
		sb.WriteString(fmt.Sprintf("event: message_delta\ndata: %s\n\n", ej))
	}

	return sb.String(), nil
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

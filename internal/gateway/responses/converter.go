package responses

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"modelgate/internal/config"
	"modelgate/internal/gateway/proxy"
)

// ConvertToOpenAI 将 ResponsesRequest 转换为 OpenAI ChatCompletions 请求体
func ConvertToOpenAI(req *ResponsesRequest, modelCfg *config.ModelConfig) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	var messages []map[string]interface{}

	// 1. instructions 归一化为 system 消息
	if req.Instructions != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": req.Instructions,
		})
	}

	// 2. 解析 input 字段
	if req.Input != nil {
		parsedMsgs, err := parseInputToMessages(req.Input)
		if err != nil {
			return nil, err
		}
		messages = append(messages, parsedMsgs...)
	}

	openaiReq := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   req.Stream,
	}

	if req.MaxOutputTokens != nil {
		// max_output_tokens 映射策略（§5）：默认 max_tokens，模型配置可切换 max_completion_tokens
		outputTokensField := "max_tokens"
		if modelCfg != nil && modelCfg.Responses.MaxOutputTokensField == "max_completion_tokens" {
			outputTokensField = "max_completion_tokens"
		}
		openaiReq[outputTokensField] = *req.MaxOutputTokens
	}
	if req.Temperature != nil {
		openaiReq["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		openaiReq["top_p"] = *req.TopP
	}

	// 3. 处理 tools 及 tool_choice / parallel_tool_calls
	if len(req.Tools) > 0 {
		var openaiTools []map[string]interface{}
		for _, toolObj := range req.Tools {
			if toolMap, ok := toolObj.(map[string]interface{}); ok {
				if _, hasFunction := toolMap["function"]; hasFunction {
					openaiTools = append(openaiTools, toolMap)
				} else {
					openaiTools = append(openaiTools, map[string]interface{}{
						"type":     "function",
						"function": toolMap,
					})
				}
			}
		}
		openaiReq["tools"] = openaiTools
	}

	if req.ToolChoice != nil {
		if tcStr, ok := req.ToolChoice.(string); ok {
			openaiReq["tool_choice"] = tcStr
		} else if tcMap, ok := req.ToolChoice.(map[string]interface{}); ok {
			if _, hasFn := tcMap["function"]; hasFn {
				openaiReq["tool_choice"] = tcMap
			} else if name, ok := tcMap["name"].(string); ok {
				openaiReq["tool_choice"] = map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name": name,
					},
				}
			}
		}
	}

	if req.ParallelToolCalls != nil {
		openaiReq["parallel_tool_calls"] = *req.ParallelToolCalls
	}

	// 4. 处理 reasoning.effort -> reasoning_effort
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		openaiReq["reasoning_effort"] = req.Reasoning.Effort
	}

	// 5. 处理 text.format -> response_format
	if req.Text != nil && req.Text.Format != nil {
		if fmtStr, ok := req.Text.Format.(string); ok {
			if fmtStr == "json_object" || fmtStr == "json_schema" {
				openaiReq["response_format"] = map[string]interface{}{
					"type": fmtStr,
				}
			}
		} else if fmtMap, ok := req.Text.Format.(map[string]interface{}); ok {
			openaiReq["response_format"] = fmtMap
		}
	}

	return json.Marshal(openaiReq)
}

func parseInputToMessages(input interface{}) ([]map[string]interface{}, error) {
	var messages []map[string]interface{}

	switch v := input.(type) {
	case string:
		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": v,
		})
	case []interface{}:
		for _, item := range v {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			itemType, _ := itemMap["type"].(string)
			switch itemType {
			case "message":
				role, _ := itemMap["role"].(string)
				if role == "" {
					role = "user"
				}
				contentVal, err := parseContentVal(itemMap["content"])
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]interface{}{
					"role":    role,
					"content": contentVal,
				})
			case "function_call":
				name, _ := itemMap["name"].(string)
				args, _ := itemMap["arguments"].(string)
				callID, _ := itemMap["call_id"].(string)
				if callID == "" {
					callID, _ = itemMap["id"].(string)
				}
				if callID == "" {
					callID = "call_" + uuid.New().String()[:8]
				}

				messages = append(messages, map[string]interface{}{
					"role": "assistant",
					"tool_calls": []map[string]interface{}{
						{
							"id":   callID,
							"type": "function",
							"function": map[string]interface{}{
								"name":      name,
								"arguments": args,
							},
						},
					},
				})
			case "function_call_output":
				callID, _ := itemMap["call_id"].(string)
				outputStr, _ := itemMap["output"].(string)
				messages = append(messages, map[string]interface{}{
					"role":         "tool",
					"tool_call_id": callID,
					"content":      outputStr,
				})
			case "input_file", "computer_call", "web_search_call", "local_shell_call":
				return nil, fmt.Errorf("unsupported input item type: %s", itemType)
			default:
				// 如果没有 type，尝试当作常规 message 解析
				if role, ok := itemMap["role"].(string); ok {
					contentVal, err := parseContentVal(itemMap["content"])
					if err != nil {
						return nil, err
					}
					messages = append(messages, map[string]interface{}{
						"role":    role,
						"content": contentVal,
					})
				}
			}
		}
	}

	return messages, nil
}

func parseContentVal(content interface{}) (interface{}, error) {
	if str, ok := content.(string); ok {
		return str, nil
	}
	if arr, ok := content.([]interface{}); ok {
		var parts []map[string]interface{}
		for _, part := range arr {
			if partMap, ok := part.(map[string]interface{}); ok {
				pType, _ := partMap["type"].(string)
				switch pType {
				case "input_text", "text", "output_text":
					if textStr, ok := partMap["text"].(string); ok {
						parts = append(parts, map[string]interface{}{
							"type": "text",
							"text": textStr,
						})
					}
				case "input_image":
					if imageURL, ok := partMap["image_url"].(string); ok {
						parts = append(parts, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": imageURL,
							},
						})
					}
				case "input_file":
					return nil, fmt.Errorf("unsupported content part type: input_file")
				}
			}
		}
		if len(parts) > 0 {
			return parts, nil
		}
	}
	return content, nil
}

// ConvertFromOpenAI 将后端的 ChatCompletions 非流式响应转换为 Responses 响应格式
func ConvertFromOpenAI(backendResp []byte, req *ResponsesRequest) ([]byte, error) {
	var chatResp proxy.OpenAIResponse
	if err := json.Unmarshal(backendResp, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal backend response: %w", err)
	}

	respID := "resp_" + uuid.New().String()
	if chatResp.ID != "" {
		respID = "resp_" + strings.TrimPrefix(chatResp.ID, "chatcmpl-")
	}

	created := chatResp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	var outputItems []ResponseItem
	var fullOutputText string

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]
		if msg, ok := choice["message"].(map[string]interface{}); ok {
			// 提取文本内容
			if content, ok := msg["content"].(string); ok && content != "" {
				fullOutputText = content
				msgItemID := "msg_" + uuid.New().String()[:12]
				outputItems = append(outputItems, ResponseItem{
					ID:     msgItemID,
					Type:   "message",
					Status: "completed",
					Role:   "assistant",
					Content: []ContentPart{
						{
							Type:        "output_text",
							Text:        content,
							Annotations: []interface{}{},
						},
					},
				})
			}

			// 提取 reasoning_content
			if reasoning, ok := msg["reasoning_content"].(string); ok && reasoning != "" {
				reasoningItemID := "rs_" + uuid.New().String()[:12]
				outputItems = append(outputItems, ResponseItem{
					ID:      reasoningItemID,
					Type:    "reasoning",
					Status:  "completed",
					Summary: reasoning,
				})
			}

			// 提取 tool_calls
			if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
				for _, tc := range toolCalls {
					if tcMap, ok := tc.(map[string]interface{}); ok {
						callID, _ := tcMap["id"].(string)
						fcItemID := "fc_" + uuid.New().String()[:12]
						fnName := ""
						fnArgs := ""
						if fn, ok := tcMap["function"].(map[string]interface{}); ok {
							fnName, _ = fn["name"].(string)
							fnArgs, _ = fn["arguments"].(string)
						}
						outputItems = append(outputItems, ResponseItem{
							ID:     fcItemID,
							Type:   "function_call",
							Status: "completed",
							Name:   fnName,
							CallID: callID,
							Args:   fnArgs,
						})
					}
				}
			}
		}
	}

	var usage *Usage
	if chatResp.Usage != nil {
		usage = &Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		}
	}

	resp := ResponsesResponse{
		ID:         respID,
		Object:     "response",
		CreatedAt:  created,
		Status:     "completed",
		Model:      chatResp.Model,
		Output:     outputItems,
		OutputText: fullOutputText,
		Usage:      usage,
	}

	return json.Marshal(resp)
}

package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConvertToOpenAI 将 Anthropic 请求转换为 OpenAI 格式
func ConvertToOpenAI(req *MessagesRequest) ([]byte, error) {
	// 构建 OpenAI 格式的 messages
	var messages []map[string]interface{}

	// 添加 system 消息（如果有）
	if req.System != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": req.System,
		})
	}

	// 转换 messages
	for _, msg := range req.Messages {
		role := msg.Role
		// Anthropic 使用 "assistant"，OpenAI 也是 "assistant"
		// Anthropic 使用 "user"，OpenAI 也是 "user"
		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": msg.Content,
		})
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

	anthropicResp := MessagesResponse{
		ID:    id,
		Type:  "message",
		Role:  "assistant",
		Model: originalReq.Model,
		Content: []Block{
			{
				Type: "text",
				Text: content,
			},
		},
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
		return line, nil // 解析失败时透传
	}

	choices, ok := openaiStream["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return line, nil
	}

	choice := choices[0].(map[string]interface{})

	// 检查是否是消息开始
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return line, nil
	}

	// 处理 role 字段（消息开始）
	if _, hasRole := delta["role"]; hasRole {
		return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"\"}}\n\n", nil
	}

	// 处理 content
	content, hasContent := delta["content"].(string)
	if hasContent && content != "" {
		event := StreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &Delta{
				Type: "text_delta",
				Text: content,
			},
		}
		eventJSON, _ := json.Marshal(event)
		return fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", eventJSON), nil
	}

	// 处理 finish_reason
	if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
		stopReason := convertStopReason(finishReason)
		event := StreamEvent{
			Type:       "message_delta",
			StopReason: &stopReason,
		}
		eventJSON, _ := json.Marshal(event)
		return fmt.Sprintf("event: message_delta\ndata: %s\n\n", eventJSON), nil
	}

	return line, nil
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
	default:
		return openaiReason
	}
}

// generateID 生成消息 ID
func generateID() string {
	// 简单实现，实际可以使用 UUID
	return "msg_" + randomString(12)
}

// randomString 生成随机字符串
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[i%len(charset)]
	}
	return string(result)
}

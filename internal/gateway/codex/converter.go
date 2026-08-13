package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"modelgate/internal/config"
	"modelgate/internal/gateway/proxy"
)

// defaultFIMSystemPrompt 模式 B（通用模型）的默认 System Prompt
const defaultFIMSystemPrompt = "You are an inline code completion engine. Complete the code between <PREFIX> and <SUFFIX>. Output ONLY the missing code at the cursor position. Do NOT output markdown formatting like ```python, do NOT output explanations."

// ConvertToOpenAI 将 CompletionRequest 转换为 OpenAI ChatCompletions 请求
func ConvertToOpenAI(req *CompletionRequest, modelCfg *config.ModelConfig) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	if req.N > 1 {
		return nil, fmt.Errorf("n > 1 is not supported")
	}
	if req.BestOf != nil && *req.BestOf > 1 {
		return nil, fmt.Errorf("best_of > 1 is not supported")
	}

	var promptStr string
	switch p := req.Prompt.(type) {
	case string:
		promptStr = p
	case []interface{}:
		return nil, fmt.Errorf("multiple prompt array is not supported")
	default:
		if req.Prompt != nil {
			promptStr = fmt.Sprintf("%v", req.Prompt)
		}
	}

	var messages []map[string]interface{}
	modelLower := strings.ToLower(req.Model)

	// 处理 FIM (Fill-In-The-Middle) 场景，行为由模型级配置驱动（§5）
	fimEnabled, fimMode := fimEffectiveMode(modelCfg)
	if req.Suffix != "" && fimEnabled && fimMode != "disabled" {
		useNative := fimMode == "native" || (fimMode == "auto" && isNativeFIMModel(modelLower))
		if useNative {
			// 模式 A：原生 FIM 标签
			prefixTag, suffixTag, middleTag := getFIMTags(modelLower, modelCfg)
			fimPrompt := prefixTag + promptStr + suffixTag + req.Suffix + middleTag
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": fimPrompt,
			})
		} else {
			// 模式 B：通用模型的 System Prompt 约束
			systemPrompt := defaultFIMSystemPrompt
			if modelCfg != nil && modelCfg.FIM.SystemPrompt != "" {
				systemPrompt = modelCfg.FIM.SystemPrompt
			}
			messages = append(messages, map[string]interface{}{
				"role":    "system",
				"content": systemPrompt,
			})
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": "<PREFIX>\n" + promptStr + "\n</PREFIX>\n<SUFFIX>\n" + req.Suffix + "\n</SUFFIX>",
			})
		}
	} else {
		// 普通前缀补全（FIM 未启用或 suffix 为空时，suffix 忽略）
		messages = append(messages, map[string]interface{}{
			"role":    "user",
			"content": promptStr,
		})
	}

	openaiReq := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   req.Stream,
	}

	if req.MaxTokens != nil {
		openaiReq["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		openaiReq["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		openaiReq["top_p"] = *req.TopP
	}
	if req.Stop != nil {
		openaiReq["stop"] = req.Stop
	}
	if req.PresencePenalty != nil {
		openaiReq["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		openaiReq["frequency_penalty"] = *req.FrequencyPenalty
	}

	return json.Marshal(openaiReq)
}

// ConvertFromOpenAI 将后端的 ChatCompletions 非流式响应转换为 Text Completions 格式
func ConvertFromOpenAI(backendResp []byte, req *CompletionRequest, modelCfg *config.ModelConfig) ([]byte, error) {
	var chatResp proxy.OpenAIResponse
	if err := json.Unmarshal(backendResp, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal backend response: %w", err)
	}

	var completionText string
	var finishReason *string

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]
		if msg, ok := choice["message"].(map[string]interface{}); ok {
			if content, ok := msg["content"].(string); ok {
				completionText = content
			}
		}
		if fr, ok := choice["finish_reason"].(string); ok {
			finishReason = &fr
		}
	}

	// 剥离 Markdown 围栏
	completionText = CleanCodeFilterNonStream(completionText)

	// 可选：裁剪输出首尾空白（fim.trim_whitespace）
	if modelCfg != nil && modelCfg.FIM.TrimWhitespace {
		completionText = strings.TrimSpace(completionText)
	}

	// 如果开启了 Echo
	if req != nil && req.Echo {
		if pStr, ok := req.Prompt.(string); ok {
			completionText = pStr + completionText
		}
	}

	respID := "cmpl-" + uuid.New().String()
	if chatResp.ID != "" {
		respID = "cmpl-" + strings.TrimPrefix(chatResp.ID, "chatcmpl-")
	}

	created := chatResp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	var usage *Usage
	if chatResp.Usage != nil {
		usage = &Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		}
	}

	compResp := CompletionResponse{
		ID:      respID,
		Object:  "text_completion",
		Created: created,
		Model:   chatResp.Model,
		Choices: []CompletionChoice{
			{
				Text:         completionText,
				Index:        0,
				Logprobs:     nil,
				FinishReason: finishReason,
			},
		},
		Usage: usage,
	}

	return json.Marshal(compResp)
}

// ConvertStreamLine 将后端的 Chat SSE 文本转换为 Text Completions SSE 文本
func ConvertStreamLine(line string, req *CompletionRequest, state map[string]interface{}, modelCfg *config.ModelConfig) (string, error) {
	var sb strings.Builder
	trimWS := modelCfg != nil && modelCfg.FIM.TrimWhitespace

	segments := strings.Split(line, "\n")
	for _, segment := range segments {
		segmentTrim := strings.TrimSpace(segment)
		if segmentTrim == "" {
			continue
		}

		if segmentTrim == "data: [DONE]" || segmentTrim == "data:[DONE]" {
			// [DONE] 兜底：先冲刷尾部缓冲（剥离残留围栏/空白）
			if pending := flushTailText(state, trimWS); pending != "" {
				sb.WriteString("data: " + string(buildStreamChunk(state, pending, nil, nil)) + "\n\n")
			}
			sb.WriteString("data: [DONE]\n\n")
			continue
		}

		var jsonStr string
		if strings.HasPrefix(segmentTrim, "data: ") {
			jsonStr = strings.TrimPrefix(segmentTrim, "data: ")
		} else if strings.HasPrefix(segmentTrim, "data:") {
			jsonStr = strings.TrimPrefix(segmentTrim, "data:")
		} else {
			continue
		}

		jsonStr = strings.TrimSpace(jsonStr)
		if jsonStr == "" {
			continue
		}

		var streamResp proxy.StreamResponse
		if err := json.Unmarshal([]byte(jsonStr), &streamResp); err != nil {
			sb.WriteString(segment + "\n\n")
			continue
		}

		// 缓存流元数据（ID/Created/Model 跨 chunk 保持一致）
		if id, _ := state["cmpl_id"].(string); id == "" && streamResp.ID != "" {
			state["cmpl_id"] = "cmpl-" + strings.TrimPrefix(streamResp.ID, "chatcmpl-")
		}
		if m, _ := state["cmpl_model"].(string); m == "" && streamResp.Model != "" {
			state["cmpl_model"] = streamResp.Model
		}
		if ct, _ := state["cmpl_created"].(int64); ct == 0 && streamResp.Created != 0 {
			state["cmpl_created"] = streamResp.Created
		}

		var deltaText string
		var finishReason *string
		var usage *Usage

		if len(streamResp.Choices) > 0 {
			choice := streamResp.Choices[0]
			if content, ok := choice.Delta["content"].(string); ok {
				deltaText = content
			}
			finishReason = choice.FinishReason
		}
		if streamResp.Usage != nil {
			usage = &Usage{
				PromptTokens:     streamResp.Usage.PromptTokens,
				CompletionTokens: streamResp.Usage.CompletionTokens,
				TotalTokens:      streamResp.Usage.TotalTokens,
			}
		}

		// 流式 Echo：首增量前拼接 prompt
		if req != nil && req.Echo {
			if echoDone, _ := state["echo_done"].(bool); !echoDone {
				state["echo_done"] = true
				if pStr, ok := req.Prompt.(string); ok {
					deltaText = pStr + deltaText
				}
			}
		}

		// 过滤流式中的首部 Markdown 围栏
		deltaText = CleanCodeFilterStream(deltaText, state)

		// 尾部缓冲：finish 时统一剥离残留围栏并（可选）裁剪空白
		if finishReason != nil {
			deltaText = flushTailTextWith(state, deltaText, trimWS)
		} else {
			deltaText = bufferTailText(state, deltaText)
			// 可选：裁剪输出首部空白
			if trimWS {
				if headTrimmed, _ := state["trim_head_done"].(bool); !headTrimmed && deltaText != "" {
					state["trim_head_done"] = true
					deltaText = strings.TrimLeft(deltaText, " \t\r\n")
				}
			}
		}

		// 空增量且未结束时跳过，避免发送空包
		if deltaText == "" && finishReason == nil {
			continue
		}

		sb.WriteString("data: " + string(buildStreamChunk(state, deltaText, finishReason, usage)) + "\n\n")
	}

	return sb.String(), nil
}

// bufferTailText 保留末尾若干字符，以便流结束时剥离残留的 Markdown 围栏
func bufferTailText(state map[string]interface{}, delta string) string {
	const tailHold = 8
	pending, _ := state["tail_buf"].(string)
	pending += delta
	if len(pending) > tailHold {
		state["tail_buf"] = pending[len(pending)-tailHold:]
		return pending[:len(pending)-tailHold]
	}
	state["tail_buf"] = pending
	return ""
}

// flushTailTextWith 在流结束时冲刷尾部缓冲并剥离残留围栏（可选裁剪空白）
func flushTailTextWith(state map[string]interface{}, delta string, trimWS bool) string {
	if headBuf, _ := state["head_fence_buf"].(string); headBuf != "" {
		delta = headBuf + delta
		state["head_fence_buf"] = ""
	}
	pending, _ := state["tail_buf"].(string)
	pending += delta
	state["tail_buf"] = ""

	pending = TrimTrailingFence(pending)
	if trimWS {
		pending = strings.TrimSpace(pending)
	}
	return pending
}

// flushTailText 在 [DONE] 兜底时冲刷尾部缓冲（无新增量）
func flushTailText(state map[string]interface{}, trimWS bool) string {
	return flushTailTextWith(state, "", trimWS)
}

// buildStreamChunk 构造 Text Completions 流式响应块（ID/Created/Model 跨 chunk 保持一致）
func buildStreamChunk(state map[string]interface{}, deltaText string, finishReason *string, usage *Usage) []byte {
	respID, _ := state["cmpl_id"].(string)
	if respID == "" {
		respID = "cmpl-" + uuid.New().String()
		state["cmpl_id"] = respID
	}

	created, _ := state["cmpl_created"].(int64)
	if created == 0 {
		created = time.Now().Unix()
		state["cmpl_created"] = created
	}

	model, _ := state["cmpl_model"].(string)

	outResp := CompletionStreamResponse{
		ID:      respID,
		Object:  "text_completion",
		Created: created,
		Model:   model,
		Choices: []CompletionChoice{
			{
				Text:         deltaText,
				Index:        0,
				Logprobs:     nil,
				FinishReason: finishReason,
			},
		},
		Usage: usage,
	}

	b, _ := json.Marshal(outResp)
	return b
}

// fimEffectiveMode 返回 FIM 是否启用及生效模式
// 缺省不启用（与设计文档 §5 一致：fim.enabled 缺省 false，仅显式开启）
func fimEffectiveMode(modelCfg *config.ModelConfig) (bool, string) {
	if modelCfg == nil || !modelCfg.FIM.Enabled {
		return false, ""
	}
	mode := modelCfg.FIM.Mode
	if mode == "" {
		mode = "auto"
	}
	return true, mode
}

func isNativeFIMModel(modelLower string) bool {
	return strings.Contains(modelLower, "qwen") && strings.Contains(modelLower, "coder") ||
		strings.Contains(modelLower, "deepseek") && strings.Contains(modelLower, "coder") ||
		strings.Contains(modelLower, "starcoder") ||
		strings.Contains(modelLower, "codellama") ||
		strings.Contains(modelLower, "code-llama")
}

func getFIMTags(modelLower string, modelCfg *config.ModelConfig) (prefixTag, suffixTag, middleTag string) {
	if strings.Contains(modelLower, "deepseek") {
		return "<｜fim▁begin｜>", "<｜fim▁end｜>", "<｜fim▁hole｜>"
	}
	if strings.Contains(modelLower, "codellama") || strings.Contains(modelLower, "code-llama") {
		return "<PRE>", "<SUF>", "<MID>"
	}
	if strings.Contains(modelLower, "starcoder") {
		return "<fim_prefix>", "<fim_suffix>", "<fim_middle>"
	}
	if prefixTag == "" {
		// 默认使用 Qwen 格式
		prefixTag, suffixTag, middleTag = "<|fim_prefix|>", "<|fim_suffix|>", "<|fim_middle|>"
	}

	// 模型级配置覆盖默认占位符（§5）
	if modelCfg != nil {
		if modelCfg.FIM.Prefix != "" {
			prefixTag = modelCfg.FIM.Prefix
		}
		if modelCfg.FIM.Suffix != "" {
			suffixTag = modelCfg.FIM.Suffix
		}
		if modelCfg.FIM.Middle != "" {
			middleTag = modelCfg.FIM.Middle
		}
	}
	return prefixTag, suffixTag, middleTag
}

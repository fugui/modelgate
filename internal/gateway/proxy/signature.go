package proxy

import (
	"encoding/json"
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
	if val, ok := thoughtSignatureCache.Load(toolCallID); ok {
		return val, true
	}

	// 尝试去掉 toolu_ 前缀（如果存在）
	if strings.HasPrefix(toolCallID, "toolu_") {
		trimmed := strings.TrimPrefix(toolCallID, "toolu_")
		if val, ok := thoughtSignatureCache.Load(trimmed); ok {
			return val, true
		}
	}
	// 尝试加上 toolu_ 前缀（如果不存在）
	if !strings.HasPrefix(toolCallID, "toolu_") {
		padded := "toolu_" + toolCallID
		if val, ok := thoughtSignatureCache.Load(padded); ok {
			return val, true
		}
	}
	return nil, false
}

// ClearThoughtSignatureCache 清空签名缓存
func ClearThoughtSignatureCache() {
	thoughtSignatureCache.Range(func(key, value interface{}) bool {
		thoughtSignatureCache.Delete(key)
		return true
	})
}

// RangeThoughtSignatureCache 遍历签名缓存（用于测试与调试）
func RangeThoughtSignatureCache(f func(key, value interface{}) bool) {
	thoughtSignatureCache.Range(f)
}

// injectOpenAIThoughtSignatures 在向后端发送请求前，为助理的 tool_calls 自动注入缓存的 Gemini thought_signature
func injectOpenAIThoughtSignatures(payload *OpenAIRequestHeader) {
	if payload == nil {
		return
	}
	for i, msg := range payload.Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			mutated := false
			for j, tc := range msg.ToolCalls {
				id, _ := tc["id"].(string)
				if id == "" {
					continue
				}

				// 检查是否已经有 thought_signature。如果已经有了，就不需要注入了。
				hasSig := false
				if _, ok := tc["extra_content"]; ok {
					hasSig = true
				}
				if !hasSig {
					if fn, ok := tc["function"].(map[string]interface{}); ok {
						if _, ok := fn["thought_signature"]; ok {
							hasSig = true
						}
					}
				}

				if !hasSig {
					// 尝试从缓存中获取签名并注入
					if extra, ok := GetThoughtSignature(id); ok {
						// 1. 注入到 extra_content 根级别
						tc["extra_content"] = extra

						// 提取签名字符串
						var sig string
						if extraStr, ok := extra.(string); ok {
							sig = extraStr
						} else if extraMap, ok := extra.(map[string]interface{}); ok {
							if google, ok := extraMap["google"].(map[string]interface{}); ok {
								if s, ok := google["thought_signature"].(string); ok {
									sig = s
								} else if s, ok := google["thoughtSignature"].(string); ok {
									sig = s
								}
							}
							if sig == "" {
								if s, ok := extraMap["thought_signature"].(string); ok {
									sig = s
								} else if s, ok := extraMap["thoughtSignature"].(string); ok {
									sig = s
								}
							}
						}

						// 注入到所有可能的字段格式中，确保对各类适配层的 100% 兼容性
						if sig != "" {
							// 2. 注入到 function 内部 (snake_case / camelCase)
							if fnMap, ok := tc["function"].(map[string]interface{}); ok {
								fnMap["thought_signature"] = sig
								fnMap["thoughtSignature"] = sig
							}
							// 3. 注入到 tool_call 根节点 (snake_case / camelCase)
							tc["thought_signature"] = sig
							tc["thoughtSignature"] = sig
							// 4. 注入到 provider_specific_fields (LiteLLM 兼容)
							tc["provider_specific_fields"] = map[string]interface{}{
								"thought_signature": sig,
								"thoughtSignature":  sig,
							}
						}
						msg.ToolCalls[j] = tc
						mutated = true
					}
				}
			}
			if mutated {
				payload.Messages[i] = msg
			}
		}
	}
}

// cacheThoughtSignaturesFromResponse 从非流式响应体中解析并缓存 Gemini thought_signature
func cacheThoughtSignaturesFromResponse(respBody []byte) {
	if len(respBody) == 0 {
		return
	}
	var normalResp map[string]interface{}
	if err := json.Unmarshal(respBody, &normalResp); err != nil {
		return
	}

	// 提取全局 extra_content / extraContent
	var globalExtraContent interface{}
	if ec, exists := normalResp["extra_content"]; exists {
		globalExtraContent = ec
	} else if ec, exists := normalResp["extraContent"]; exists {
		globalExtraContent = ec
	}

	choices, _ := normalResp["choices"].([]interface{})
	for _, ch := range choices {
		choiceMap, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}

		var choiceExtra interface{}
		if ec, exists := choiceMap["extra_content"]; exists {
			choiceExtra = ec
		} else if ec, exists := choiceMap["extraContent"]; exists {
			choiceExtra = ec
		}

		message, _ := choiceMap["message"].(map[string]interface{})
		if message == nil {
			continue
		}

		if choiceExtra == nil {
			if ec, exists := message["extra_content"]; exists {
				choiceExtra = ec
			} else if ec, exists := message["extraContent"]; exists {
				choiceExtra = ec
			}
		}

		if choiceExtra == nil {
			choiceExtra = globalExtraContent
		}

		var toolCalls []interface{}
		if tc, exists := message["tool_calls"]; exists {
			toolCalls, _ = tc.([]interface{})
		} else if tc, exists := message["toolCalls"]; exists {
			toolCalls, _ = tc.([]interface{})
		}

		for _, tcObj := range toolCalls {
			tcMap, ok := tcObj.(map[string]interface{})
			if !ok {
				continue
			}

			var toolID string
			if idVal, exists := tcMap["id"]; exists {
				toolID, _ = idVal.(string)
			}

			if toolID != "" {
				cacheID := toolID
				if !strings.HasPrefix(cacheID, "toolu_") {
					cacheID = "toolu_" + cacheID
				}

				var tcExtra interface{}
				if ec, exists := tcMap["extra_content"]; exists {
					tcExtra = ec
				} else if ec, exists := tcMap["extraContent"]; exists {
					tcExtra = ec
				}

				if tcExtra != nil {
					CacheThoughtSignature(cacheID, tcExtra)
				} else if choiceExtra != nil {
					CacheThoughtSignature(cacheID, choiceExtra)
				}
			}
		}
	}
}

// cacheThoughtSignaturesFromStreamLine 从流式响应行（SSE）中解析并缓存 Gemini thought_signature
func cacheThoughtSignaturesFromStreamLine(line string, state map[string]interface{}) {
	if line == "" || !strings.HasPrefix(line, "data: ") {
		return
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
	if data == "[DONE]" || data == "" {
		return
	}

	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}

	choices, _ := chunk["choices"].([]interface{})
	for _, ch := range choices {
		choiceMap, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}
		delta, ok := choiceMap["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// 提取 extra_content / extraContent
		var extraContent interface{}
		if ec, exists := delta["extra_content"]; exists {
			extraContent = ec
		} else if ec, exists := delta["extraContent"]; exists {
			extraContent = ec
		}

		if extraContent != nil {
			state["pending_extra_content"] = extraContent
			if lastID, ok := state["last_tool_id"].(string); ok && lastID != "" {
				CacheThoughtSignature(lastID, extraContent)
			}
		}

		// 提取 tool_calls / toolCalls
		var toolCalls []interface{}
		if tc, exists := delta["tool_calls"]; exists {
			toolCalls, _ = tc.([]interface{})
		} else if tc, exists := delta["toolCalls"]; exists {
			toolCalls, _ = tc.([]interface{})
		}

		for _, tcObj := range toolCalls {
			tcMap, ok := tcObj.(map[string]interface{})
			if !ok {
				continue
			}

			// 提取 index
			var tcIndex int
			if idxVal, exists := tcMap["index"]; exists {
				if floatVal, ok := idxVal.(float64); ok {
					tcIndex = int(floatVal)
				}
			}

			// 提取 toolID
			var toolID string
			if idVal, exists := tcMap["id"]; exists {
				toolID, _ = idVal.(string)
			}

			toolMap, ok := state["tool_id_map"].(map[int]string)
			if !ok {
				toolMap = make(map[int]string)
				state["tool_id_map"] = toolMap
			}

			// 获取并锁定 ID，处理延迟 ID 分配情况，避免 ID 错位
			existingID := toolMap[tcIndex]
			if existingID != "" {
				toolID = existingID
				state["last_tool_id"] = toolID
			} else if toolID != "" {
				if !strings.HasPrefix(toolID, "toolu_") {
					toolID = "toolu_" + toolID
				}
				toolMap[tcIndex] = toolID
				state["last_tool_id"] = toolID
			}

			if toolID != "" {
				var tcExtra interface{}
				if ec, exists := tcMap["extra_content"]; exists {
					tcExtra = ec
				} else if ec, exists := tcMap["extraContent"]; exists {
					tcExtra = ec
				}

				if tcExtra != nil {
					CacheThoughtSignature(toolID, tcExtra)
				} else if pending, ok := state["pending_extra_content"]; ok {
					CacheThoughtSignature(toolID, pending)
				}
			}
		}
	}
}

package responses

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"modelgate/internal/config"
	"modelgate/internal/gateway/proxy"
)

// ConvertStreamLineToResponsesEvents 将后端的 Chat SSE 文本转译为 Responses API 的命名事件包
// 支持处理可能在一行/单次读取中包含多个 data: 事件的粘包场景
func ConvertStreamLineToResponsesEvents(line string, req *ResponsesRequest, state map[string]interface{}, modelCfg *config.ModelConfig) (string, error) {
	var sb strings.Builder
	// 推理事件族（§5）：summary（默认）| text
	if _, ok := state["reasoning_event"].(string); !ok {
		reasoningEvent := "summary"
		if modelCfg != nil && modelCfg.Responses.ReasoningEvent == "text" {
			reasoningEvent = "text"
		}
		state["reasoning_event"] = reasoningEvent
	}

	// 针对粘包处理：可能单次调用传入多行或包含多个 data: 事件
	segments := strings.Split(line, "\n")
	for _, segment := range segments {
		segmentTrim := strings.TrimSpace(segment)
		if segmentTrim == "" {
			continue
		}

		// 处理 [DONE] 行
		if segmentTrim == "data: [DONE]" || segmentTrim == "data:[DONE]" {
			if emitted, _ := state["completed_emitted"].(bool); !emitted {
				state["completed_emitted"] = true
				sb.WriteString(buildCompletionEvents(state, nil))
			}
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

		// P0-2: 处理后端流中返回的错误 JSON
		var errResp struct {
			Error map[string]interface{} `json:"error"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &errResp); err == nil && len(errResp.Error) > 0 {
			if emitted, _ := state["completed_emitted"].(bool); !emitted {
				state["completed_emitted"] = true

				respID, _ := state["resp_id"].(string)
				if respID == "" {
					respID = "resp_" + uuid.New().String()[:12]
				}

				failedData, _ := json.Marshal(map[string]interface{}{
					"id":     respID,
					"status": "failed",
					"error":  errResp.Error,
				})
				sb.WriteString("event: response.failed\ndata: " + string(failedData) + "\n\n")

				errData, _ := json.Marshal(map[string]interface{}{
					"error": errResp.Error,
				})
				sb.WriteString("event: error\ndata: " + string(errData) + "\n\n")
			}
			continue
		}

		var streamResp proxy.StreamResponse
		if err := json.Unmarshal([]byte(jsonStr), &streamResp); err != nil {
			continue
		}

		// 1. 首包触发生命周期事件 (response.created, response.in_progress, output_item.added, content_part.added)
		createdEmitted, _ := state["created_emitted"].(bool)
		if !createdEmitted {
			state["created_emitted"] = true

			respID := "resp_" + uuid.New().String()[:12]
			msgItemID := "msg_" + uuid.New().String()[:12]
			state["resp_id"] = respID
			state["msg_item_id"] = msgItemID
			state["created_at"] = streamResp.Created
			if streamResp.Created == 0 {
				state["created_at"] = time.Now().Unix()
			}

			// response.created
			createdData, _ := json.Marshal(map[string]interface{}{
				"id":         respID,
				"object":     "response",
				"status":     "in_progress",
				"created_at": state["created_at"],
				"model":      streamResp.Model,
			})
			sb.WriteString("event: response.created\ndata: " + string(createdData) + "\n\n")

			// response.in_progress
			inProgressData, _ := json.Marshal(map[string]interface{}{
				"id":     respID,
				"status": "in_progress",
			})
			sb.WriteString("event: response.in_progress\ndata: " + string(inProgressData) + "\n\n")

			// response.output_item.added (message)
			itemAddedData, _ := json.Marshal(map[string]interface{}{
				"type":         "response.output_item.added",
				"output_index": 0,
				"item": map[string]interface{}{
					"id":     msgItemID,
					"type":   "message",
					"role":   "assistant",
					"status": "in_progress",
				},
			})
			sb.WriteString("event: response.output_item.added\ndata: " + string(itemAddedData) + "\n\n")

			// response.content_part.added
			partAddedData, _ := json.Marshal(map[string]interface{}{
				"type":          "response.content_part.added",
				"item_id":       msgItemID,
				"output_index":  0,
				"content_index": 0,
				"part": map[string]interface{}{
					"type": "output_text",
					"text": "",
				},
			})
			sb.WriteString("event: response.content_part.added\ndata: " + string(partAddedData) + "\n\n")
		}

		msgItemID, _ := state["msg_item_id"].(string)

		// 2. 解析 choices[0] 的 delta
		if len(streamResp.Choices) > 0 {
			choice := streamResp.Choices[0]

			// 文本增量
			if content, ok := choice.Delta["content"].(string); ok && content != "" {
				deltaData, _ := json.Marshal(map[string]interface{}{
					"type":          "response.output_text.delta",
					"item_id":       msgItemID,
					"output_index":  0,
					"content_index": 0,
					"delta":         content,
				})
				sb.WriteString("event: response.output_text.delta\ndata: " + string(deltaData) + "\n\n")
			}

			// 推理文本增量
			if reasoning, ok := choice.Delta["reasoning_content"].(string); ok && reasoning != "" {
				reasoningEvent, _ := state["reasoning_event"].(string)
				reasoningAdded, _ := state["reasoning_added"].(bool)
				rsItemID, _ := state["rs_item_id"].(string)
				if !reasoningAdded {
					state["reasoning_added"] = true
					rsItemID = "rs_" + uuid.New().String()[:12]
					state["rs_item_id"] = rsItemID

					rsAddedData, _ := json.Marshal(map[string]interface{}{
						"type":         "response.output_item.added",
						"output_index": 1,
						"item": map[string]interface{}{
							"id":     rsItemID,
							"type":   "reasoning",
							"status": "in_progress",
						},
					})
					sb.WriteString("event: response.output_item.added\ndata: " + string(rsAddedData) + "\n\n")
				}

				// 聚合推理文本，供 done 事件携带完整内容
				reasoningText, _ := state["reasoning_text"].(string)
				state["reasoning_text"] = reasoningText + reasoning

				rsDeltaType := "response.reasoning_summary_text.delta"
				if reasoningEvent == "text" {
					rsDeltaType = "response.reasoning_text.delta"
				}
				rsDeltaData, _ := json.Marshal(map[string]interface{}{
					"type":    rsDeltaType,
					"item_id": rsItemID,
					"delta":   reasoning,
				})
				sb.WriteString("event: " + rsDeltaType + "\ndata: " + string(rsDeltaData) + "\n\n")
			}

			// 工具调用增量
			if toolCalls, ok := choice.Delta["tool_calls"].([]interface{}); ok {
				for _, tc := range toolCalls {
					if tcMap, ok := tc.(map[string]interface{}); ok {
						idxVal, _ := tcMap["index"].(float64)
						tcIdx := int(idxVal)

						tcStateMap, _ := state["tc_state_map"].(map[int]map[string]string)
						if tcStateMap == nil {
							tcStateMap = make(map[int]map[string]string)
							state["tc_state_map"] = tcStateMap
						}

						tcInfo, exists := tcStateMap[tcIdx]
						if !exists {
							callID, _ := tcMap["id"].(string)
							fcItemID := "fc_" + uuid.New().String()[:12]
							tcInfo = map[string]string{
								"call_id":   callID,
								"item_id":   fcItemID,
								"arguments": "",
								"name":      "",
							}
							tcStateMap[tcIdx] = tcInfo

							fnName := ""
							if fn, ok := tcMap["function"].(map[string]interface{}); ok {
								fnName, _ = fn["name"].(string)
								tcInfo["name"] = fnName
							}

							fcAddedData, _ := json.Marshal(map[string]interface{}{
								"type":         "response.output_item.added",
								"output_index": tcIdx + 2,
								"item": map[string]interface{}{
									"id":      fcItemID,
									"type":    "function_call",
									"call_id": callID,
									"name":    fnName,
									"status":  "in_progress",
								},
							})
							sb.WriteString("event: response.output_item.added\ndata: " + string(fcAddedData) + "\n\n")
						}

						if fn, ok := tcMap["function"].(map[string]interface{}); ok {
							if fnName, ok := fn["name"].(string); ok && fnName != "" {
								tcInfo["name"] = fnName
							}
							if argsDelta, ok := fn["arguments"].(string); ok && argsDelta != "" {
								tcInfo["arguments"] += argsDelta
								fcArgsData, _ := json.Marshal(map[string]interface{}{
									"type":    "response.function_call_arguments.delta",
									"item_id": tcInfo["item_id"],
									"delta":   argsDelta,
								})
								sb.WriteString("event: response.function_call_arguments.delta\ndata: " + string(fcArgsData) + "\n\n")
							}
						}
					}
				}
			}

			// 检查 finish_reason 收尾
			if choice.FinishReason != nil {
				if emitted, _ := state["completed_emitted"].(bool); !emitted {
					state["completed_emitted"] = true
					sb.WriteString(buildCompletionEvents(state, streamResp.Usage))
				}
			}
		}
	}

	return sb.String(), nil
}

// buildCompletionEvents 构建完整收尾事件链（P0-1）
func buildCompletionEvents(state map[string]interface{}, usageVal interface{}) string {
	var sb strings.Builder
	msgItemID, _ := state["msg_item_id"].(string)
	respID, _ := state["resp_id"].(string)

	// 1. 消息文本 done 链
	if msgItemID != "" {
		textDoneData, _ := json.Marshal(map[string]interface{}{
			"type":          "response.output_text.done",
			"item_id":       msgItemID,
			"output_index":  0,
			"content_index": 0,
		})
		sb.WriteString("event: response.output_text.done\ndata: " + string(textDoneData) + "\n\n")

		partDoneData, _ := json.Marshal(map[string]interface{}{
			"type":          "response.content_part.done",
			"item_id":       msgItemID,
			"output_index":  0,
			"content_index": 0,
		})
		sb.WriteString("event: response.content_part.done\ndata: " + string(partDoneData) + "\n\n")

		itemDoneData, _ := json.Marshal(map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": 0,
			"item": map[string]interface{}{
				"id":     msgItemID,
				"type":   "message",
				"status": "completed",
			},
		})
		sb.WriteString("event: response.output_item.done\ndata: " + string(itemDoneData) + "\n\n")
	}

	// 2. 推理条目 done 链 (P0-1)
	if reasoningAdded, _ := state["reasoning_added"].(bool); reasoningAdded {
		if rsDoneEmitted, _ := state["rs_done_emitted"].(bool); !rsDoneEmitted {
			state["rs_done_emitted"] = true
			rsItemID, _ := state["rs_item_id"].(string)
			reasoningEvent, _ := state["reasoning_event"].(string)
			reasoningText, _ := state["reasoning_text"].(string)

			rsDoneType := "response.reasoning_summary_text.done"
			rsDonePayload := map[string]interface{}{
				"type":    rsDoneType,
				"item_id": rsItemID,
			}
			if reasoningEvent == "text" {
				rsDoneType = "response.reasoning_text.done"
				rsDonePayload["type"] = rsDoneType
				rsDonePayload["text"] = reasoningText
			} else {
				rsDonePayload["summary"] = reasoningText
			}
			rsDoneData, _ := json.Marshal(rsDonePayload)
			sb.WriteString("event: " + rsDoneType + "\ndata: " + string(rsDoneData) + "\n\n")

			rsItemDoneData, _ := json.Marshal(map[string]interface{}{
				"type":         "response.output_item.done",
				"output_index": 1,
				"item": map[string]interface{}{
					"id":     rsItemID,
					"type":   "reasoning",
					"status": "completed",
				},
			})
			sb.WriteString("event: response.output_item.done\ndata: " + string(rsItemDoneData) + "\n\n")
		}
	}

	// 3. 工具调用条目 done 链 (P0-1)
	if tcStateMap, ok := state["tc_state_map"].(map[int]map[string]string); ok {
		// 按 index 升序输出，保证多工具调用的 done 事件顺序稳定
		indices := make([]int, 0, len(tcStateMap))
		for tcIdx := range tcStateMap {
			indices = append(indices, tcIdx)
		}
		sort.Ints(indices)

		for _, tcIdx := range indices {
			tcInfo := tcStateMap[tcIdx]
			if tcInfo["done_emitted"] != "true" {
				tcInfo["done_emitted"] = "true"
				fcDoneData, _ := json.Marshal(map[string]interface{}{
					"type":      "response.function_call_arguments.done",
					"item_id":   tcInfo["item_id"],
					"arguments": tcInfo["arguments"],
				})
				sb.WriteString("event: response.function_call_arguments.done\ndata: " + string(fcDoneData) + "\n\n")

				fcItemDoneData, _ := json.Marshal(map[string]interface{}{
					"type":         "response.output_item.done",
					"output_index": tcIdx + 2,
					"item": map[string]interface{}{
						"id":        tcInfo["item_id"],
						"type":      "function_call",
						"call_id":   tcInfo["call_id"],
						"name":      tcInfo["name"],
						"arguments": tcInfo["arguments"],
						"status":    "completed",
					},
				})
				sb.WriteString("event: response.output_item.done\ndata: " + string(fcItemDoneData) + "\n\n")
			}
		}
	}

	// 4. response.completed
	completedPayload := map[string]interface{}{
		"id":     respID,
		"status": "completed",
	}

	if usageVal != nil {
		if uMap, ok := usageVal.(*Usage); ok && uMap != nil {
			completedPayload["usage"] = uMap
		} else {
			bUsage, _ := json.Marshal(usageVal)
			var u Usage
			if err := json.Unmarshal(bUsage, &u); err == nil {
				var rawMap map[string]int
				_ = json.Unmarshal(bUsage, &rawMap)
				if inTok, exists := rawMap["prompt_tokens"]; exists {
					u.InputTokens = inTok
				}
				if outTok, exists := rawMap["completion_tokens"]; exists {
					u.OutputTokens = outTok
				}
				completedPayload["usage"] = u
			}
		}
	}
	completedData, _ := json.Marshal(completedPayload)
	sb.WriteString("event: response.completed\ndata: " + string(completedData) + "\n\n")

	return sb.String()
}

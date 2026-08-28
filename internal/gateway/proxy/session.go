package proxy

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SessionExtractionHeaders 用于粘性路由的 HTTP Header 列表（按优先级）
// 注：Go 的 http.Header 查询是大小写不敏感的，此处统一声明标准形式
var SessionExtractionHeaders = []string{
	"X-Session-ID",
	"X-Conversation-ID",
	"Session-ID",
	"Conversation-ID",
	"X-Chat-ID",
	"Chat-ID",
}

// TraceIDHeaders 用于提取请求链路追踪 ID 的 HTTP Header 列表（按优先级）
var TraceIDHeaders = []string{
	"X-Request-ID",
	"Request-ID",
	"X-Trace-ID",
	"X-Correlation-ID",
}

// SessionInfo 包含会话特征提取结果及来源诊断信息
type SessionInfo struct {
	Key      string `json:"key"`       // 用于负载均衡哈希的唯一 Key（为空表示无显式会话）
	Source   string `json:"source"`    // 来源类型（例如 "header:X-Session-ID", "body:conversation_id", "none"）
	RawValue string `json:"raw_value"` // 提取到的原始会话 ID 值
}

// ExtractTraceID 提取客户端传入的 Trace ID，若无则自动生成，并返回获取来源
func ExtractTraceID(c *gin.Context) (traceID string, source string) {
	if c != nil {
		for _, headerName := range TraceIDHeaders {
			if val := strings.TrimSpace(c.GetHeader(headerName)); val != "" {
				return val, "header:" + headerName
			}
		}
	}
	return "req-" + uuid.New().String(), "generated"
}

// lightweightSessionBody 用于快速提取 JSON 请求体中的显式会话标识
type lightweightSessionBody struct {
	SessionID      string `json:"session_id"`
	ConversationID string `json:"conversation_id"`
	ChatID         string `json:"chat_id"`
}

// ExtractSessionInfo 提取显式会话标识及其来源详情（用于日志审计与 KV Cache 亲和性路由）
//
// 策略说明：
// 1. 优先提取显式 HTTP Headers (X-Session-ID, X-Conversation-ID 等)；
// 2. 其次提取请求体中的显式会话字段 (session_id, conversation_id, chat_id)；
// 3. 若未显式提供会话标识，则 Source 为 "none"，Key 为空，让请求走纯加权最少连接负载均衡。
func ExtractSessionInfo(c *gin.Context, bodyBytes []byte) SessionInfo {
	// 1. 检查显式 HTTP Headers
	if c != nil {
		for _, headerName := range SessionExtractionHeaders {
			if val := strings.TrimSpace(c.GetHeader(headerName)); val != "" {
				return SessionInfo{
					Key:      "hdr:" + val,
					Source:   "header:" + headerName,
					RawValue: val,
				}
			}
		}
	}

	// 2. 检查显式 Body JSON 字段
	if len(bodyBytes) > 0 && bytes.HasPrefix(bytes.TrimSpace(bodyBytes), []byte("{")) {
		var sBody lightweightSessionBody
		if err := json.Unmarshal(bodyBytes, &sBody); err == nil {
			if s := strings.TrimSpace(sBody.SessionID); s != "" {
				return SessionInfo{
					Key:      "body:session:" + s,
					Source:   "body:session_id",
					RawValue: s,
				}
			}
			if s := strings.TrimSpace(sBody.ConversationID); s != "" {
				return SessionInfo{
					Key:      "body:conv:" + s,
					Source:   "body:conversation_id",
					RawValue: s,
				}
			}
			if s := strings.TrimSpace(sBody.ChatID); s != "" {
				return SessionInfo{
					Key:      "body:chat:" + s,
					Source:   "body:chat_id",
					RawValue: s,
				}
			}
		}
	}

	return SessionInfo{
		Key:      "",
		Source:   "none",
		RawValue: "",
	}
}

// ExtractSessionKey 仅提取显式会话的 Key 字符串（兼容旧调用接口）
func ExtractSessionKey(c *gin.Context, bodyBytes []byte) string {
	return ExtractSessionInfo(c, bodyBytes).Key
}

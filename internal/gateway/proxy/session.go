package proxy

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
)

// SessionExtractionHeaders 用于粘性路由的 HTTP Header 列表（按优先级）
var SessionExtractionHeaders = []string{
	"X-Session-ID",
	"X-Session-Id",
	"X-Conversation-ID",
	"X-Conversation-Id",
	"Session-ID",
	"Session-Id",
	"Conversation-ID",
	"Conversation-Id",
}

// lightweightSessionBody 用于快速提取 JSON 请求体中的显式会话标识
type lightweightSessionBody struct {
	SessionID      string `json:"session_id"`
	ConversationID string `json:"conversation_id"`
	ChatID         string `json:"chat_id"`
}

// ExtractSessionKey 仅提取显式的会话标识（用于 Prompt/KV Cache 亲和性路由）
//
// 策略说明：
// 1. 优先提取显式 HTTP Headers (X-Session-ID, X-Conversation-ID 等)；
// 2. 其次提取请求体中的显式会话字段 (session_id, conversation_id, chat_id)；
// 3. 若未显式提供会话标识，则返回空字符串，让请求走纯加权最少连接负载均衡（Weighted Least-Connections）。
//
// 注意：严禁降级到 User UUID、Client IP 或首条消息哈希。将用户级或 IP 级当作会话会导致单用户/单设备下的
// 所有并发请求被持续绑定到单一后端，形成“满载级联溢出（1/9/9）”而破坏负载均衡。
func ExtractSessionKey(c *gin.Context, bodyBytes []byte) string {
	// 1. 检查显式 HTTP Headers
	if c != nil {
		for _, headerName := range SessionExtractionHeaders {
			if val := strings.TrimSpace(c.GetHeader(headerName)); val != "" {
				return "hdr:" + val
			}
		}
	}

	// 2. 检查显式 Body JSON 字段
	if len(bodyBytes) > 0 && bytes.HasPrefix(bytes.TrimSpace(bodyBytes), []byte("{")) {
		var sBody lightweightSessionBody
		if err := json.Unmarshal(bodyBytes, &sBody); err == nil {
			if s := strings.TrimSpace(sBody.SessionID); s != "" {
				return "body:session:" + s
			}
			if s := strings.TrimSpace(sBody.ConversationID); s != "" {
				return "body:conv:" + s
			}
			if s := strings.TrimSpace(sBody.ChatID); s != "" {
				return "body:chat:" + s
			}
		}
	}

	return ""
}

package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

// lightweightSessionBody 用于快速提取 JSON 请求体中的会话标识或首条消息指纹
type lightweightSessionBody struct {
	SessionID      string            `json:"session_id"`
	ConversationID string            `json:"conversation_id"`
	ChatID         string            `json:"chat_id"`
	User           string            `json:"user"`
	Messages       []json.RawMessage `json:"messages"`
}

// ExtractSessionKey 按照多级优先级提取请求的会话粘性特征 Key
// 优先级设计：
// 1. HTTP Headers (X-Session-ID, X-Conversation-ID 等显式会话头)
// 2. JSON 请求体中的显式会话字段 (session_id, conversation_id, chat_id, user)
// 3. 隐式多轮消息指纹 (messages[0] 内容哈希，完美适配 OpenCode / Cursor / 多轮 Chat)
// 4. 内网客户端真实 IP (局域网直连环境下的物理开发机/工位机粘性)
// 5. 已鉴权的用户 UUID (同一用户的全局粘性)
func ExtractSessionKey(c *gin.Context, bodyBytes []byte, uid uuid.UUID, clientIP string) string {
	// 1. 检查显式 HTTP Headers
	if c != nil {
		for _, headerName := range SessionExtractionHeaders {
			if val := strings.TrimSpace(c.GetHeader(headerName)); val != "" {
				return "hdr:" + val
			}
		}
	}

	var sBody lightweightSessionBody
	hasParsedBody := false
	if len(bodyBytes) > 0 && bytes.HasPrefix(bytes.TrimSpace(bodyBytes), []byte("{")) {
		if err := json.Unmarshal(bodyBytes, &sBody); err == nil {
			hasParsedBody = true
			// 2. 显式 Body 字段
			if s := strings.TrimSpace(sBody.SessionID); s != "" {
				return "body:session:" + s
			}
			if s := strings.TrimSpace(sBody.ConversationID); s != "" {
				return "body:conv:" + s
			}
			if s := strings.TrimSpace(sBody.ChatID); s != "" {
				return "body:chat:" + s
			}
			if s := strings.TrimSpace(sBody.User); s != "" {
				return "body:user:" + s
			}
		}
	}

	// 3. 已鉴权的用户 UUID (优先使用用户级粘性)
	if uid != uuid.Nil {
		return "uid:" + uid.String()
	}

	// 4. 内网客户端真实 IP（局域网直连时最可靠的物理设备级标识）
	cleanIP := strings.TrimSpace(clientIP)
	if cleanIP != "" && cleanIP != "127.0.0.1" && cleanIP != "::1" {
		return "ip:" + cleanIP
	}

	// 5. 隐式多轮消息指纹 (提取 messages[0] 的哈希，跨多轮保持一致)
	// 警告: 该策略由于大量应用的首条消息为 System Prompt，极易造成全局哈希碰撞热点，因此排在靠后优先级
	if hasParsedBody && len(sBody.Messages) > 0 && len(bytes.TrimSpace(sBody.Messages[0])) > 0 {
		h := sha256.Sum256(bytes.TrimSpace(sBody.Messages[0]))
		return "msg0:" + hex.EncodeToString(h[:8])
	}

	// 6. 本地开发/回环 IP 作为最后的备选
	if cleanIP != "" {
		return "ip:" + cleanIP
	}

	return ""
}

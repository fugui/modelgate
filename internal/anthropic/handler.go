// Package anthropic 提供 Anthropic API 协议支持
package anthropic

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/proxy"
)

// Handler 处理 Anthropic API 请求
type Handler struct {
	proxy *proxy.Proxy
}

// NewHandler 创建 Anthropic Handler
func NewHandler(proxy *proxy.Proxy) *Handler {
	return &Handler{proxy: proxy}
}

// RegisterRoutes 注册 Anthropic 路由
func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc) {
	v1 := r.Group("/v1")
	v1.Use(authMiddleware)
	{
		v1.POST("/messages", h.HandleMessages)
	}
}

// HandleMessages 处理 /v1/messages 请求
func (h *Handler) HandleMessages(c *gin.Context) {
	// 获取认证信息
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	uid := userID.(uuid.UUID)

	// 解析 Anthropic 请求
	var anthropicReq MessagesRequest
	if err := c.ShouldBindJSON(&anthropicReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format: " + err.Error()})
		return
	}

	// 验证必需字段
	if anthropicReq.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
		return
	}

	if len(anthropicReq.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages is required"})
		return
	}

	// 转换为 OpenAI 请求
	openaiBody, err := ConvertToOpenAI(&anthropicReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert request: " + err.Error()})
		return
	}

	// 执行核心工作流
	backendReq := &proxy.BackendRequest{
		ModelID:     anthropicReq.Model,
		UserID:      uid,
		RequestBody: openaiBody,
		IsStream:    anthropicReq.Stream,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	}

	h.proxy.ExecuteCoreWorkflow(
		c,
		backendReq,
		// 响应转换器
		func(body []byte) ([]byte, error) {
			return ConvertFromOpenAI(body, &anthropicReq)
		},
		// 流式行转换器
		func(line string) (string, error) {
			return ConvertStreamLine(line, &anthropicReq)
		},
	)
}

// MessagesRequest Anthropic 消息请求
type MessagesRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	System      string    `json:"system,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	TopP        float64   `json:"top_p,omitempty"`
	TopK        int       `json:"top_k,omitempty"`
}

// Message Anthropic 消息
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MessagesResponse Anthropic 非流式响应
type MessagesResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	Role         string  `json:"role"`
	Model        string  `json:"model"`
	Content      []Block `json:"content"`
	StopReason   *string `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
	Usage        Usage   `json:"usage"`
}

// Block 内容块
type Block struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Usage 使用量
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent 流式事件
type StreamEvent struct {
	Type         string      `json:"type"`
	Message      *Message    `json:"message,omitempty"`
	Index        int         `json:"index,omitempty"`
	ContentBlock *Block      `json:"content_block,omitempty"`
	Delta        *Delta      `json:"delta,omitempty"`
	Usage        *Usage      `json:"usage,omitempty"`
	StopReason   *string     `json:"stop_reason,omitempty"`
	StopSequence *string     `json:"stop_sequence,omitempty"`
}

// Delta 增量更新
type Delta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

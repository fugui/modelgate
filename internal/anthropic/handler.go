// Package anthropic 提供 Anthropic API 协议支持
package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/middleware"
	"modelgate/internal/proxy"
)

// UsageService 访问日志服务接口
type UsageService interface {
	RecordAccess(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64)
	RecordAccessDetailed(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, requestHeaders map[string]string, requestBody string, responseHeaders map[string]string, responseBody string)
}

// Handler 处理 Anthropic API 请求
type Handler struct {
	proxy        *proxy.Proxy
	usageService UsageService
}

// NewHandler 创建 Anthropic Handler
func NewHandler(proxy *proxy.Proxy, usageService UsageService) *Handler {
	return &Handler{proxy: proxy, usageService: usageService}
}

// RegisterRoutes 注册 Anthropic 路由
func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc) {
	v1 := r.Group("/v1")
	v1.Use(authMiddleware)
	{
		v1.POST("/messages", middleware.AccessLogMiddleware(h.usageService), h.HandleMessages)
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

	// 读取原始请求体用于调试
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body: " + err.Error()})
		return
	}
	// 重新设置 body 以便后续处理
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 解析 Anthropic 请求
	var anthropicReq MessagesRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
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
		// Anthropic-compliant ping/keep-alive message
		"event: ping\ndata: {\"type\": \"ping\"}\n\n",
	)
}

// Tool 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// MessagesRequest Anthropic 消息请求
type MessagesRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	System      interface{} `json:"system,omitempty"` // 支持字符串或数组格式
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
	TopP        float64     `json:"top_p,omitempty"`
	TopK        int         `json:"top_k,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
	Tools       []Tool      `json:"tools,omitempty"` // 工具定义
}

// Message Anthropic 消息
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // 支持字符串或数组格式
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
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`     // Anthropic 思考块内容
	Signature    string          `json:"signature,omitempty"`    // Anthropic 思考块签名
	ID           string          `json:"id,omitempty"`           // 用于 tool_use 的唯一标识
	ToolUseID    string          `json:"tool_use_id,omitempty"`  // 用于 tool_result 指向对应的 tool_use
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	Content      interface{}     `json:"content,omitempty"`      // 用于 tool_result 的内容块
	IsError      bool            `json:"is_error,omitempty"`     // 用于 tool_result
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
}

// Delta 增量更新
type Delta struct {
	Type             string  `json:"type,omitempty"`
	Text             string  `json:"text,omitempty"`
	Thinking         string  `json:"thinking,omitempty"`          // Anthropic 思考增量
	Signature        string  `json:"signature,omitempty"`         // Anthropic 签名增量
	PartialJSON      string  `json:"partial_json,omitempty"`      // 用于 tool_use 参数增量
	ReasoningContent string  `json:"reasoning_content,omitempty"` // 用于兼容某些 OpenAI 后端
	StopReason       *string `json:"stop_reason,omitempty"`
	StopSequence     *string `json:"stop_sequence,omitempty"`
}

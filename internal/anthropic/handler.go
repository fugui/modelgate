// Package anthropic 提供 Anthropic API 协议支持
package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/concurrency"
	"modelgate/internal/middleware"
	"modelgate/internal/proxy"
	"modelgate/internal/utils"
)

// UsageService 访问日志服务接口
type UsageService interface {
	RecordAccess(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, durationMs int64)
	RecordAccessDetailed(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, requestHeaders map[string]string, requestBody string, responseHeaders map[string]string, responseBody string, inputTokens int, outputTokens int, durationMs int64)
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
func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter) {
	v1 := r.Group("/v1")
	v1.Use(authMiddleware)
	{
		v1.POST("/messages", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.TrafficLogMiddleware(), middleware.AccessLogMiddleware(h.usageService), h.HandleMessages)
		v1.POST("/messages/count_tokens", h.HandleCountTokens)
	}
}

// HandleMessages 处理 /v1/messages 请求
func (h *Handler) HandleMessages(c *gin.Context) {
	// 获取认证信息
	userID, exists := c.Get("user_id")
	if !exists {
		sendAnthropicError(c, http.StatusUnauthorized, "authentication_error", "unauthorized")
		return
	}
	uid := userID.(uuid.UUID)

	// 读取原始请求体用于调试
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		sendAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "failed to read request body: "+err.Error())
		return
	}
	// 重新设置 body 以便后续处理
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// 解析 Anthropic 请求
	var anthropicReq MessagesRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		sendAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "invalid request format: "+err.Error())
		return
	}

	// 验证必需字段
	if anthropicReq.Model == "" {
		sendAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	if len(anthropicReq.Messages) == 0 {
		sendAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	// 转换为 OpenAI 请求
	openaiBody, err := ConvertToOpenAI(&anthropicReq)
	if err != nil {
		sendAnthropicError(c, http.StatusInternalServerError, "api_error", "failed to convert request: "+err.Error())
		return
	}

	// 获取 API Key ID（由认证中间件设置）
	var akid uuid.UUID
	if apiKeyID, exists := c.Get("api_key_id"); exists {
		if id, ok := apiKeyID.(uuid.UUID); ok {
			akid = id
		}
	}

	// 执行核心工作流
	backendReq := &proxy.BackendRequest{
		ModelID:     anthropicReq.Model,
		UserID:      uid,
		APIKeyID:    akid,
		RequestBody: openaiBody,
		IsStream:    anthropicReq.Stream,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	}

	h.proxy.ExecuteCoreWorkflow(
		c,
		backendReq,
		&Protocol{ClientReq: &anthropicReq},
	)
}

// Protocol 实现了 proxy.Protocol 接口
type Protocol struct {
	ClientReq *MessagesRequest
}

func (p *Protocol) FormatResponse(backendResp []byte) ([]byte, int, int, error) {
	// 提前解析原始 backendResp 获取精确 Token
	var normalResp proxy.OpenAIResponse
	var preciseInput, preciseOutput int
	if err := json.Unmarshal(backendResp, &normalResp); err == nil && normalResp.Usage != nil {
		preciseInput = normalResp.Usage.PromptTokens
		preciseOutput = normalResp.Usage.CompletionTokens
	}

	clientResp, err := ConvertFromOpenAI(backendResp, p.ClientReq)
	return clientResp, preciseInput, preciseOutput, err
}

func (p *Protocol) FormatStreamLine(line string, state map[string]interface{}) (string, int, int, string, error) {
	clientLine, err := ConvertStreamLine(line, p.ClientReq, state)
	if err != nil {
		return "", 0, 0, "", err
	}

	// 文本内容和精确 Token 的提取直接从原始 OpenAI 行完成，简单且准确
	content, preciseInput, preciseOutput := proxy.ParseOpenAISSE(line)

	return clientLine, preciseInput, preciseOutput, content, nil
}

func (p *Protocol) PingMessage() string {
	return "event: ping\ndata: {\"type\": \"ping\"}\n\n"
}

func (p *Protocol) BuildErrorResponse(errType, message string) []byte {
	resp := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errType,
			"message": message,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func sendAnthropicError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// HandleCountTokens 处理 /v1/messages/count_tokens 请求
func (h *Handler) HandleCountTokens(c *gin.Context) {
	// 获取认证信息
	_, exists := c.Get("user_id")
	if !exists {
		sendAnthropicError(c, http.StatusUnauthorized, "authentication_error", "unauthorized")
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		sendAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "failed to read request body: "+err.Error())
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var anthropicReq MessagesRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		sendAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "invalid request format: "+err.Error())
		return
	}

	// 统一转换为 OpenAI 格式，复用底层的标准 Token 估算器
	openaiBody, err := ConvertToOpenAI(&anthropicReq)
	if err != nil {
		sendAnthropicError(c, http.StatusInternalServerError, "api_error", "failed to convert request for token counting: "+err.Error())
		return
	}

	// 估算总 Token
	inputTokens := utils.EstimateTokensFromOpenAIRequest(openaiBody)

	// Anthropic Count Tokens Response 结构
	c.JSON(http.StatusOK, gin.H{
		"input_tokens": inputTokens,
	})
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

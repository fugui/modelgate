// Package anthropic 提供 Anthropic API 协议支持
package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"modelgate/internal/domain/usage"
	"modelgate/internal/gateway/proxy"
	"modelgate/internal/infra/concurrency"
	"modelgate/internal/infra/middleware"
	"modelgate/internal/infra/utils"
)

// Handler 处理 Anthropic API 请求
type Handler struct {
	proxy        *proxy.Proxy
	usageService *usage.Service
}

// NewHandler 创建 Anthropic Handler
func NewHandler(proxyInst *proxy.Proxy, usageService *usage.Service) *Handler {
	return &Handler{proxy: proxyInst, usageService: usageService}
}

// RegisterRoutes 注册 Anthropic 路由
func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter) {
	v1 := r.Group("/v1")
	v1.Use(middleware.ProtocolInjectionMiddleware(&Protocol{}))
	v1.Use(authMiddleware)
	{
		v1.POST("/messages", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.AccessLogMiddleware(h.usageService), h.HandleMessages)
		v1.POST("/messages/count_tokens", h.HandleCountTokens)
	}
}

// HandleMessages 处理 /v1/messages 请求
func (h *Handler) HandleMessages(c *gin.Context) {
	var anthropicReq MessagesRequest
	h.proxy.HandleProxyRequest(c, &Protocol{ClientReq: &anthropicReq}, func(bodyBytes []byte) (string, bool, []byte, error) {
		if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
			return "", false, nil, err
		}

		if anthropicReq.Model == "" {
			return "", false, nil, fmt.Errorf("model is required")
		}

		if len(anthropicReq.Messages) == 0 {
			return "", false, nil, fmt.Errorf("messages is required")
		}

		openaiBody, err := ConvertToOpenAI(&anthropicReq)
		if err != nil {
			return "", false, nil, fmt.Errorf("failed to convert request: %w", err)
		}

		return anthropicReq.Model, anthropicReq.Stream, openaiBody, nil
	})
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

// HandleCountTokens 处理 /v1/messages/count_tokens 请求
func (h *Handler) HandleCountTokens(c *gin.Context) {
	proto := &Protocol{}
	// 获取认证信息
	_, exists := c.Get("user_id")
	if !exists {
		c.Data(http.StatusUnauthorized, "application/json", proto.BuildErrorResponse("authentication_error", "unauthorized"))
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", "failed to read request body: "+err.Error()))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var anthropicReq MessagesRequest
	if err := json.Unmarshal(bodyBytes, &anthropicReq); err != nil {
		c.Data(http.StatusBadRequest, "application/json", proto.BuildErrorResponse("invalid_request_error", "invalid request format: "+err.Error()))
		return
	}

	// 统一转换为 OpenAI 格式，复用底层的标准 Token 估算器
	openaiBody, err := ConvertToOpenAI(&anthropicReq)
	if err != nil {
		c.Data(http.StatusInternalServerError, "application/json", proto.BuildErrorResponse("api_error", "failed to convert request for token counting: "+err.Error()))
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
	Model         string      `json:"model"`
	Messages      []Message   `json:"messages"`
	System        interface{} `json:"system,omitempty"` // 支持字符串或数组格式
	MaxTokens     int         `json:"max_tokens,omitempty"`
	Temperature   float64     `json:"temperature,omitempty"`
	Stream        bool        `json:"stream,omitempty"`
	TopP          float64     `json:"top_p,omitempty"`
	TopK          int         `json:"top_k,omitempty"`
	StopSequences []string    `json:"stop_sequences,omitempty"`
	Tools         []Tool      `json:"tools,omitempty"` // 工具定义
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
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`    // Anthropic 思考块内容
	Signature string          `json:"signature,omitempty"`   // Anthropic 思考块签名
	ID        string          `json:"id,omitempty"`          // 用于 tool_use 的唯一标识
	ToolUseID string          `json:"tool_use_id,omitempty"` // 用于 tool_result 指向对应的 tool_use
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   interface{}     `json:"content,omitempty"`  // 用于 tool_result 的内容块
	IsError   bool            `json:"is_error,omitempty"` // 用于 tool_result
}

// Usage 使用量
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent 流式事件
type StreamEvent struct {
	Type         string   `json:"type"`
	Message      *Message `json:"message,omitempty"`
	Index        int      `json:"index,omitempty"`
	ContentBlock *Block   `json:"content_block,omitempty"`
	Delta        *Delta   `json:"delta,omitempty"`
	Usage        *Usage   `json:"usage,omitempty"`
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

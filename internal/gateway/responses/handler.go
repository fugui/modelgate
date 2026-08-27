package responses

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"modelgate/internal/domain/usage"
	"modelgate/internal/gateway/proxy"
	"modelgate/internal/infra/concurrency"
	"modelgate/internal/infra/middleware"
)

// Handler 处理 Responses API 请求（直通代理模式）
type Handler struct {
	proxy        *proxy.Proxy
	usageService *usage.Service
}

// NewHandler 创建 Responses Handler
func NewHandler(proxyInst *proxy.Proxy, usageService *usage.Service) *Handler {
	return &Handler{
		proxy:        proxyInst,
		usageService: usageService,
	}
}

// RegisterRoutes 注册 Responses API 路由
func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter, clientFilter gin.HandlerFunc) {
	v1 := r.Group("/v1")
	v1.Use(middleware.ProtocolInjectionMiddleware(&Protocol{}))
	v1.Use(clientFilter)
	v1.Use(authMiddleware)
	{
		v1.POST("/responses", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.AccessLogMiddleware(h.usageService), h.HandleResponses)
	}
}

// HandleResponses 处理 /v1/responses 请求
// 直通代理模式：仅提取 model 和 stream 用于路由和流式处理判断，请求体原样转发到后端
func (h *Handler) HandleResponses(c *gin.Context) {
	proto := &Protocol{}
	h.proxy.HandleProxyRequest(c, proto, true, func(bodyBytes []byte) (string, bool, []byte, error) {
		// 仅解析 model 和 stream 字段，不做完整 unmarshal
		var header struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.Unmarshal(bodyBytes, &header); err != nil {
			return "", false, nil, err
		}

		if header.Model == "" {
			return "", false, nil, nil // 空 model 会被上层检查并报错
		}

		return header.Model, header.Stream, bodyBytes, nil
	})
}

// Protocol 实现了 proxy.Protocol 接口（Responses 协议）
type Protocol struct{}

// BackendPath 返回 Responses API 的后端路径
func (p *Protocol) BackendPath() string {
	return "/v1/responses"
}

// ExtractUsage 从 Responses 非流式响应中提取 Usage
// Responses 格式: {"usage": {"input_tokens": N, "output_tokens": N, "total_tokens": N}}
func (p *Protocol) ExtractUsage(respBody []byte) (int, int) {
	var resp struct {
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &resp); err == nil && resp.Usage != nil {
		return resp.Usage.InputTokens, resp.Usage.OutputTokens
	}
	return 0, 0
}

// ExtractStreamUsage 从 Responses 流式 SSE 行中提取 Usage
// Responses 流式的 usage 在 response.completed 事件中
func (p *Protocol) ExtractStreamUsage(line string) (int, int) {
	line = strings.TrimSpace(line)

	// 检查是否是 data: 行
	var jsonStr string
	if strings.HasPrefix(line, "data: ") {
		jsonStr = strings.TrimPrefix(line, "data: ")
	} else if strings.HasPrefix(line, "data:") {
		jsonStr = strings.TrimPrefix(line, "data:")
	} else {
		return 0, 0
	}

	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "[DONE]" {
		return 0, 0
	}

	// 尝试从 response.completed 事件中提取 usage
	var event struct {
		Type     string `json:"type"`
		Response *struct {
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &event); err == nil {
		if event.Response != nil && event.Response.Usage != nil {
			return event.Response.Usage.InputTokens, event.Response.Usage.OutputTokens
		}
	}

	return 0, 0
}

// PingMessage 返回 SSE keep-alive 消息
func (p *Protocol) PingMessage() string {
	return ": ping\n\n"
}

// BuildErrorResponse 构造 Responses API 格式的错误响应
func (p *Protocol) BuildErrorResponse(errType, message string) []byte {
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errType,
			"message": message,
			"code":    "invalid_parameter",
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

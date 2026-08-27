package openai

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"modelgate/internal/domain/usage"
	"modelgate/internal/gateway/proxy"
	"modelgate/internal/infra/concurrency"
	"modelgate/internal/infra/middleware"
)

// Handler 用于 OpenAI 兼容代理接口
type Handler struct {
	proxy        *proxy.Proxy
	usageService *usage.Service
}

func NewHandler(proxyInst *proxy.Proxy, usageService *usage.Service) *Handler {
	return &Handler{
		proxy:        proxyInst,
		usageService: usageService,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter, clientFilter gin.HandlerFunc) {
	v1 := r.Group("/v1")
	v1.Use(middleware.ProtocolInjectionMiddleware(&Protocol{}))
	v1.Use(clientFilter)
	v1.Use(authMiddleware)
	{
		v1.GET("/models", middleware.AccessLogMiddleware(h.usageService), h.ListModels)
		v1.POST("/chat/completions", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.AccessLogMiddleware(h.usageService), h.ChatCompletions)
	}
}

func (h *Handler) ListModels(c *gin.Context) {
	h.proxy.HandleListModels(c)
}

// Protocol 实现了 proxy.Protocol 接口（OpenAI Chat Completions 协议）
type Protocol struct{}

// BackendPath 返回 Chat Completions 的后端路径
func (p *Protocol) BackendPath() string {
	return "/v1/chat/completions"
}

// ExtractUsage 从 OpenAI Chat 非流式响应中提取 Usage
// OpenAI 格式: {"usage": {"prompt_tokens": N, "completion_tokens": N}}
func (p *Protocol) ExtractUsage(respBody []byte) (int, int) {
	var resp struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &resp); err == nil && resp.Usage != nil {
		return resp.Usage.PromptTokens, resp.Usage.CompletionTokens
	}
	return 0, 0
}

// ExtractStreamUsage 从 OpenAI Chat 流式 SSE 行中提取 Usage
func (p *Protocol) ExtractStreamUsage(line string) (int, int) {
	line = strings.TrimSpace(line)

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

	var chunk struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &chunk); err == nil && chunk.Usage != nil {
		return chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
	}

	return 0, 0
}

func (p *Protocol) BuildErrorResponse(errType, message string) []byte {
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errType,
			"message": message,
			"param":   nil,
			"code":    nil,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	proto := &Protocol{}
	// OpenAI Chat 不是 passthrough：需要解析请求体以便注入模型参数
	h.proxy.HandleProxyRequest(c, proto, false, func(bodyBytes []byte) (string, bool, []byte, error) {
		var req proxy.OpenAIRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			return "", false, nil, err
		}
		return req.Model, req.Stream, bodyBytes, nil
	})
}

func (p *Protocol) PingMessage() string {
	return ""
}

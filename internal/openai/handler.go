package openai

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"modelgate/internal/concurrency"
	"modelgate/internal/middleware"
	"modelgate/internal/proxy"
	"modelgate/internal/usage"
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

func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter) {
	// OpenAI 兼容接口
	v1 := r.Group("/v1")
	v1.Use(middleware.ProtocolInjectionMiddleware(&Protocol{}))
	v1.Use(authMiddleware)
	{
		v1.GET("/models", middleware.AccessLogMiddleware(h.usageService), h.ListModels)
		v1.POST("/chat/completions", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.AccessLogMiddleware(h.usageService), h.ChatCompletions)
	}
}

func (h *Handler) ListModels(c *gin.Context) {
	h.proxy.HandleListModels(c)
}

// Protocol 实现了 proxy.Protocol 接口，暴露给外部供 auth middleware 使用
type Protocol struct{}

func (p *Protocol) FormatResponse(backendResp []byte) ([]byte, int, int, error) {
	var normalResp proxy.OpenAIResponse
	var preciseInput, preciseOutput int
	if err := json.Unmarshal(backendResp, &normalResp); err == nil && normalResp.Usage != nil {
		preciseInput = normalResp.Usage.PromptTokens
		preciseOutput = normalResp.Usage.CompletionTokens
	}
	return backendResp, preciseInput, preciseOutput, nil
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
	h.proxy.HandleProxyRequest(c, proto, func(bodyBytes []byte) (string, bool, []byte, error) {
		var req proxy.OpenAIRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			return "", false, nil, err
		}
		return req.Model, req.Stream, bodyBytes, nil
	})
}

func (p *Protocol) FormatStreamLine(line string, state map[string]interface{}) (string, int, int, string, error) {
	content, preciseInput, preciseOutput := proxy.ParseOpenAISSE(line)
	return line, preciseInput, preciseOutput, content, nil
}

func (p *Protocol) PingMessage() string {
	return ""
}

package responses

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"modelgate/internal/config"
	"modelgate/internal/domain/usage"
	"modelgate/internal/gateway/proxy"
	"modelgate/internal/infra/concurrency"
	"modelgate/internal/infra/middleware"
)

type Handler struct {
	proxy        *proxy.Proxy
	usageService *usage.Service
	cfgManager   *config.ConfigManager
}

func NewHandler(proxyInst *proxy.Proxy, usageService *usage.Service, cfgManager *config.ConfigManager) *Handler {
	return &Handler{
		proxy:        proxyInst,
		usageService: usageService,
		cfgManager:   cfgManager,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter, clientFilter gin.HandlerFunc) {
	v1 := r.Group("/v1")
	v1.Use(middleware.ProtocolInjectionMiddleware(&Protocol{}))
	v1.Use(clientFilter)
	v1.Use(authMiddleware)
	{
		v1.POST("/responses", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.AccessLogMiddleware(h.usageService), h.HandleResponses)
	}
}

func (h *Handler) HandleResponses(c *gin.Context) {
	var req ResponsesRequest
	proto := &Protocol{ClientReq: &req}
	h.proxy.HandleProxyRequest(c, proto, func(bodyBytes []byte) (string, bool, []byte, error) {
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			return "", false, nil, err
		}

		if req.Model == "" {
			return "", false, nil, fmt.Errorf("model is required")
		}

		if h.cfgManager != nil {
			proto.ModelConfig = h.cfgManager.GetModelByID(req.Model)
		}

		openaiBody, err := ConvertToOpenAI(&req, proto.ModelConfig)
		if err != nil {
			return "", false, nil, fmt.Errorf("failed to convert request: %w", err)
		}

		return req.Model, req.Stream, openaiBody, nil
	})
}

// Protocol 实现了 proxy.Protocol 接口
type Protocol struct {
	ClientReq   *ResponsesRequest
	ModelConfig *config.ModelConfig
}

func (p *Protocol) FormatResponse(backendResp []byte) ([]byte, int, int, error) {
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
	clientEvents, err := ConvertStreamLineToResponsesEvents(line, p.ClientReq, state, p.ModelConfig)
	if err != nil {
		return "", 0, 0, "", err
	}

	content, preciseInput, preciseOutput := proxy.ParseOpenAISSE(line)
	return clientEvents, preciseInput, preciseOutput, content, nil
}

func (p *Protocol) PingMessage() string {
	return "event: response.ping\ndata: {}\n\n"
}

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

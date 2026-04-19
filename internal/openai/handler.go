package openai

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
)

// UsageService 访问日志服务接口
type UsageService interface {
	RecordAccess(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, durationMs int64)
	RecordAccessDetailed(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, requestHeaders map[string]string, requestBody string, responseHeaders map[string]string, responseBody string, inputTokens int, outputTokens int, durationMs int64)
}

// Proxy 代理接口
type Proxy interface {
	HandleListModels(c *gin.Context)
	ExecuteCoreWorkflow(c *gin.Context, req *proxy.BackendRequest, proto proxy.Protocol)
}

// Handler 用于 OpenAI 兼容代理接口
type Handler struct {
	proxy        Proxy
	usageService UsageService
}

func NewHandler(proxy Proxy, usageService UsageService) *Handler {
	return &Handler{
		proxy:        proxy,
		usageService: usageService,
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine, authMiddleware gin.HandlerFunc, concurrencyLimiter *concurrency.Limiter) {
	// OpenAI 兼容接口
	v1 := r.Group("/v1")
	v1.Use(authMiddleware)
	{
		v1.GET("/models", middleware.AccessLogMiddleware(h.usageService), h.ListModels)
		v1.POST("/chat/completions", middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.TrafficLogMiddleware(), middleware.AccessLogMiddleware(h.usageService), h.ChatCompletions)
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

func sendOpenAIError(c *gin.Context, statusCode int, errType, message string) {
	c.AbortWithStatusJSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
			"param":   nil,
			"code":    nil,
		},
	})
}

func (p *Protocol) FormatStreamLine(line string, state map[string]interface{}) (string, int, int, string, error) {
	content, preciseInput, preciseOutput := proxy.ParseOpenAISSE(line)
	return line, preciseInput, preciseOutput, content, nil
}

func (p *Protocol) PingMessage() string {
	return ""
}

func (h *Handler) ChatCompletions(c *gin.Context) {
	userID, _ := c.Get(middleware.ContextKeyUserID)
	apiKeyID, hasAPIKey := c.Get(middleware.ContextKeyAPIKeyID)

	uid := userID.(uuid.UUID)

	// 如果是 JWT 认证，apiKeyID 可能为空，使用零值 UUID
	var akid uuid.UUID
	if hasAPIKey {
		akid = apiKeyID.(uuid.UUID)
	} else {
		akid = uuid.Nil
	}

	// 读取请求体
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		sendOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req proxy.OpenAIRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		sendOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "invalid request format")
		return
	}

	modelID := req.Model
	if modelID == "" {
		sendOpenAIError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	backendReq := &proxy.BackendRequest{
		ModelID:     modelID,
		UserID:      uid,
		APIKeyID:    akid,
		RequestBody: bodyBytes,
		IsStream:    req.Stream,
		ClientIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
	}

	h.proxy.ExecuteCoreWorkflow(
		c,
		backendReq,
		&Protocol{},
	)
}

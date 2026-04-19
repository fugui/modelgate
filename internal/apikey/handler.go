// Package apikey 提供 API Key 管理相关的 HTTP 接口
package apikey

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/auth"
	"modelgate/internal/concurrency"
	"modelgate/internal/entity"
	"modelgate/internal/middleware"
	"modelgate/internal/proxy"
)

// contextKey 用于在 gin 上下文中存储认证信息
type contextKey string

const (
	contextKeyUserID    contextKey = "user_id"
	contextKeyAPIKeyID  contextKey = "api_key_id"
	contextKeyAuthType  contextKey = "auth_type"
)

type authType string

const (
	authTypeAPIKey authType = "apikey"
	authTypeJWT    authType = "jwt"
)

// Handler 处理 API Key 管理相关的 HTTP 请求
type Handler struct {
	service   *Service
	userStore *entity.UserStore
}

// NewHandler 创建 API Key HTTP 处理器
func NewHandler(service *Service, userStore *entity.UserStore) *Handler {
	return &Handler{service: service, userStore: userStore}
}

// RegisterRoutes 注册 API Key 管理路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup, jwtManager *auth.JWTManager) {
	keys := r.Group("/user/keys")
	keys.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, h.userStore))
	{
		keys.GET("", h.List)
		keys.POST("", h.Create)
		keys.DELETE("/:id", h.Delete)
	}
}

// List 获取当前用户的所有 API Key
// GET /api/v1/user/keys
func (h *Handler) List(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keys, err := h.service.GetUserKeys(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var responses []entity.APIKeyResponse
	for _, key := range keys {
		responses = append(responses, key.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{"data": responses})
}

func (h *Handler) Create(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req entity.APIKeyCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	key, err := h.service.GenerateKey(user.UserID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": key})
}

func (h *Handler) Delete(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	if err := h.service.DeleteKey(keyID, user.UserID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "key deleted"})
}

// ProxyHandler 用于代理接口的 API Key 验证
type ProxyHandler struct {
	service      *Service
	proxy        Proxy
	jwtManager   *auth.JWTManager
	userStore    *entity.UserStore
	usageService UsageService
}

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

func NewProxyHandler(service *Service, proxy Proxy, jwtManager *auth.JWTManager, userStore *entity.UserStore, usageService UsageService) *ProxyHandler {
	return &ProxyHandler{
		service:      service,
		proxy:        proxy,
		jwtManager:   jwtManager,
		userStore:    userStore,
		usageService: usageService,
	}
}

func (h *ProxyHandler) RegisterRoutes(r *gin.Engine, concurrencyLimiter *concurrency.Limiter) {
	// OpenAI 兼容接口
	v1 := r.Group("/v1")
	{
		v1.GET("/models", h.AuthMiddleware(), middleware.AccessLogMiddleware(h.usageService), h.ListModels)
		v1.POST("/chat/completions", h.AuthMiddleware(), middleware.ConcurrencyLimitMiddleware(concurrencyLimiter), middleware.TrafficLogMiddleware(), middleware.AccessLogMiddleware(h.usageService), h.ChatCompletions)
	}
}

func (h *ProxyHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		// 支持 Bearer Token 格式
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		// 先尝试作为 API Key 验证
		key, _, err := h.service.ValidateKey(token)
		if err == nil {
			// API Key 验证成功
			c.Set(string(contextKeyAPIKeyID), key.ID)
			c.Set(string(contextKeyUserID), key.UserID)
			c.Set(string(contextKeyAuthType), authTypeAPIKey)
			c.Next()
			return
		}

		// 如果不是有效的 API Key，尝试作为 JWT Token 验证
		claims, err := h.jwtManager.Validate(token)
		if err != nil {
			if err == auth.ErrExpiredToken {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization"})
			return
		}

		// 验证用户是否存在于数据库且未禁用
		user, err := h.userStore.GetByID(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to verify user"})
			return
		}
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if !user.Enabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user disabled"})
			return
		}

		// JWT 验证成功
		c.Set(string(contextKeyUserID), claims.UserID)
		c.Set(string(contextKeyAuthType), authTypeJWT)
		c.Next()
	}
}

func (h *ProxyHandler) ListModels(c *gin.Context) {
	h.proxy.HandleListModels(c)
}

// openAIProtocol 实现了 proxy.Protocol 接口
type openAIProtocol struct{}

func (p *openAIProtocol) FormatResponse(backendResp []byte) ([]byte, int, int, error) {
	var normalResp proxy.OpenAIResponse
	var preciseInput, preciseOutput int
	if err := json.Unmarshal(backendResp, &normalResp); err == nil && normalResp.Usage != nil {
		preciseInput = normalResp.Usage.PromptTokens
		preciseOutput = normalResp.Usage.CompletionTokens
	}
	return backendResp, preciseInput, preciseOutput, nil
}

func (p *openAIProtocol) FormatStreamLine(line string, state map[string]interface{}) (string, int, int, string, error) {
	content, preciseInput, preciseOutput := proxy.ParseOpenAISSE(line)
	return line, preciseInput, preciseOutput, content, nil
}

func (p *openAIProtocol) PingMessage() string {
	return ""
}

func (h *ProxyHandler) ChatCompletions(c *gin.Context) {
	userID, _ := c.Get(string(contextKeyUserID))
	apiKeyID, hasAPIKey := c.Get(string(contextKeyAPIKeyID))

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req proxy.OpenAIRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	modelID := req.Model
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model is required"})
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
		&openAIProtocol{},
	)
}

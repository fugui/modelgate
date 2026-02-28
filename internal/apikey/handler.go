// Package apikey 提供 API Key 管理相关的 HTTP 接口
package apikey

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"llmgate/internal/auth"
	"llmgate/internal/concurrency"
	"llmgate/internal/middleware"
	"llmgate/internal/models"
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
	userStore *models.UserStore
}

// NewHandler 创建 API Key HTTP 处理器
func NewHandler(service *Service, userStore *models.UserStore) *Handler {
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

	var responses []models.APIKeyResponse
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

	var req models.APIKeyCreateRequest
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
	service    *Service
	proxy      Proxy
	jwtManager *auth.JWTManager
	userStore  *models.UserStore
}

// Proxy 代理接口
type Proxy interface {
	HandleChatCompletions(c *gin.Context, userID uuid.UUID, apiKeyID uuid.UUID)
	HandleListModels(c *gin.Context)
}

func NewProxyHandler(service *Service, proxy Proxy, jwtManager *auth.JWTManager, userStore *models.UserStore) *ProxyHandler {
	return &ProxyHandler{
		service:    service,
		proxy:      proxy,
		jwtManager: jwtManager,
		userStore:  userStore,
	}
}

func (h *ProxyHandler) RegisterRoutes(r *gin.Engine, concurrencyLimiter *concurrency.Limiter) {
	// OpenAI 兼容接口
	v1 := r.Group("/v1")
	{
		v1.GET("/models", h.AuthMiddleware(), h.ListModels)
		v1.POST("/chat/completions", h.AuthMiddleware(), h.ChatCompletionsMiddleware(concurrencyLimiter), h.ChatCompletions)
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

// ChatCompletionsMiddleware 并发限制中间件
func (h *ProxyHandler) ChatCompletionsMiddleware(limiter *concurrency.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}

		userID, exists := c.Get(string(contextKeyUserID))
		if !exists {
			c.Next()
			return
		}

		uid := userID.(uuid.UUID)

		if !limiter.Acquire(uid.String()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "concurrency limit exceeded",
				"message": "too many concurrent requests, please try again later",
			})
			return
		}

		// 确保在请求结束后释放
		defer limiter.Release(uid.String())

		c.Next()
	}
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

	h.proxy.HandleChatCompletions(c, uid, akid)
}

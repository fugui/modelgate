// Package apikey 提供 API Key 管理相关的 HTTP 接口
package apikey

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"llmgate/internal/auth"
	"llmgate/internal/middleware"
	"llmgate/internal/models"
)

// Handler 处理 API Key 管理相关的 HTTP 请求
type Handler struct {
	service *Service
}

// NewHandler 创建 API Key HTTP 处理器
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes 注册 API Key 管理路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup, jwtManager *auth.JWTManager) {
	keys := r.Group("/user/keys")
	keys.Use(middleware.AuthMiddleware(jwtManager))
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
	jwtManager interface{}
}

// Proxy 代理接口
type Proxy interface {
	HandleChatCompletions(c *gin.Context, userID uuid.UUID, apiKeyID uuid.UUID)
	HandleListModels(c *gin.Context)
}

func NewProxyHandler(service *Service, proxy Proxy) *ProxyHandler {
	return &ProxyHandler{
		service: service,
		proxy:   proxy,
	}
}

func (h *ProxyHandler) RegisterRoutes(r *gin.Engine) {
	// OpenAI 兼容接口
	v1 := r.Group("/v1")
	{
		v1.GET("/models", h.AuthMiddleware(), h.ListModels)
		v1.POST("/chat/completions", h.AuthMiddleware(), h.ChatCompletions)
	}
}

func (h *ProxyHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		// 支持 Bearer 和直接 API Key 格式
		var apiKey string
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			apiKey = authHeader
		}

		key, _, err := h.service.ValidateKey(apiKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}

		c.Set("api_key_id", key.ID)
		c.Set("user_id", key.UserID)
		c.Next()
	}
}

func (h *ProxyHandler) ListModels(c *gin.Context) {
	h.proxy.HandleListModels(c)
}

func (h *ProxyHandler) ChatCompletions(c *gin.Context) {
	userID, _ := c.Get("user_id")
	apiKeyID, _ := c.Get("api_key_id")

	uid := userID.(uuid.UUID)
	akid := apiKeyID.(uuid.UUID)

	h.proxy.HandleChatCompletions(c, uid, akid)
}

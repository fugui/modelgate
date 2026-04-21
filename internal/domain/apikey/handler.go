// Package apikey 提供 API Key 管理相关的 HTTP 接口
package apikey

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/infra/auth"
	"modelgate/internal/infra/middleware"
	"modelgate/internal/repository"
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

package model

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"llmgate/internal/auth"
	"llmgate/internal/middleware"
	"llmgate/internal/models"
	"llmgate/internal/proxy"
)

type LoadBalancer interface {
	GetHealthStatus() map[string]proxy.BackendHealth
	GetModelBackends(modelID string) []proxy.BackendHealth
}

type Handler struct {
	store        *models.ModelStore
	userStore    *models.UserStore
	loadBalancer LoadBalancer
}

func NewHandler(store *models.ModelStore, lb LoadBalancer, userStore *models.UserStore) *Handler {
	return &Handler{
		store:        store,
		loadBalancer: lb,
		userStore:    userStore,
	}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup, jwtManager *auth.JWTManager) {
	// 公开接口 - 列出可用模型（用于 LLM 代理）
	r.GET("/v1/models", h.ListForProxy)

	// 需要认证的接口
	auth := r.Group("")
	auth.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, h.userStore))
	{
		auth.GET("/admin/models/health", h.GetHealthStatus)
	}

	// 管理员接口
	admin := r.Group("/admin/models")
	admin.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, h.userStore))
	admin.Use(middleware.AdminRequired())
	{
		admin.GET("", h.List)
		admin.POST("", h.Create)
		admin.PUT("/:id", h.Update)
		admin.DELETE("/:id", h.Delete)
		admin.GET("/:id/backends", h.GetModelBackends)
	}
}

func (h *Handler) List(c *gin.Context) {
	models, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": models})
}

func (h *Handler) ListForProxy(c *gin.Context) {
	models, err := h.store.ListEnabled()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// OpenAI 兼容格式
	var data []map[string]interface{}
	for _, m := range models {
		data = append(data, map[string]interface{}{
			"id":       m.ID,
			"object":   "model",
			"created":  m.CreatedAt.Unix(),
			"owned_by": "llmgate",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

func (h *Handler) Create(c *gin.Context) {
	var req models.ModelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model := &models.Model{
		ID:          req.ID,
		Name:        req.Name,
		BackendURL:  req.BackendURL,
		Enabled:     true,
		Weight:      req.Weight,
		Description: req.Description,
	}

	if model.Weight == 0 {
		model.Weight = 1
	}

	if err := h.store.Create(model); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": model})
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")

	model, err := h.store.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if model == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var req models.ModelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		model.Name = req.Name
	}
	if req.BackendURL != "" {
		model.BackendURL = req.BackendURL
	}
	if req.Enabled != nil {
		model.Enabled = *req.Enabled
	}
	if req.Weight > 0 {
		model.Weight = req.Weight
	}
	if req.Description != "" {
		model.Description = req.Description
	}

	if err := h.store.Update(model); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": model})
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

// GetHealthStatus 获取所有后端的健康状态
func (h *Handler) GetHealthStatus(c *gin.Context) {
	if h.loadBalancer == nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	status := h.loadBalancer.GetHealthStatus()
	c.JSON(http.StatusOK, gin.H{"data": status})
}

// GetModelBackends 获取指定模型的后端健康状态
func (h *Handler) GetModelBackends(c *gin.Context) {
	id := c.Param("id")

	if h.loadBalancer == nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	backends := h.loadBalancer.GetModelBackends(id)
	c.JSON(http.StatusOK, gin.H{"data": backends})
}

// AdminHandler 管理员配额策略管理
type AdminHandler struct {
	quotaStore *models.QuotaStore
	userStore  *models.UserStore
}

func NewAdminHandler(quotaStore *models.QuotaStore, userStore *models.UserStore) *AdminHandler {
	return &AdminHandler{quotaStore: quotaStore, userStore: userStore}
}

func (h *AdminHandler) RegisterRoutes(r *gin.RouterGroup, jwtManager *auth.JWTManager) {
	admin := r.Group("/admin/policies")
	admin.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, h.userStore))
	admin.Use(middleware.AdminRequired())
	{
		admin.GET("", h.ListPolicies)
		admin.POST("", h.CreatePolicy)
		admin.PUT("/:name", h.UpdatePolicy)
		admin.DELETE("/:name", h.DeletePolicy)
	}
}

func (h *AdminHandler) ListPolicies(c *gin.Context) {
	policies, err := h.quotaStore.ListPolicies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": policies})
}

func (h *AdminHandler) CreatePolicy(c *gin.Context) {
	var policy models.QuotaPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if policy.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if err := h.quotaStore.CreateOrUpdatePolicy(&policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": policy})
}

func (h *AdminHandler) UpdatePolicy(c *gin.Context) {
	name := c.Param("name")

	policy, err := h.quotaStore.GetPolicy(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if policy == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	var req models.QuotaPolicy
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.RateLimit > 0 {
		policy.RateLimit = req.RateLimit
	}
	if req.RateLimitWindow > 0 {
		policy.RateLimitWindow = req.RateLimitWindow
	}
	if req.TokenQuotaDaily > 0 {
		policy.TokenQuotaDaily = req.TokenQuotaDaily
	}
	if req.Models != nil {
		policy.Models = req.Models
	}
	if req.Description != "" {
		policy.Description = req.Description
	}

	if err := h.quotaStore.CreateOrUpdatePolicy(policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": policy})
}

func (h *AdminHandler) DeletePolicy(c *gin.Context) {
	name := c.Param("name")

	if err := h.quotaStore.DeletePolicy(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "policy deleted"})
}

// Ensure uuid is used
var _ = uuid.UUID{}

package model

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/auth"
	"modelgate/internal/middleware"
	"modelgate/internal/models"
	"modelgate/internal/proxy"
)

type LoadBalancer interface {
	GetHealthStatus() map[string]proxy.BackendHealth
	GetModelBackends(modelID string) []proxy.BackendHealth
}

type Handler struct {
	store        *models.ModelStore
	backendStore *models.BackendStore
	userStore    *models.UserStore
	loadBalancer LoadBalancer
}

func NewHandler(store *models.ModelStore, backendStore *models.BackendStore, lb LoadBalancer, userStore *models.UserStore) *Handler {
	return &Handler{
		store:        store,
		backendStore: backendStore,
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
		admin.POST("/:id/backends", h.CreateBackend)
		admin.PUT("/:id/backends/:backend_id", h.UpdateBackend)
		admin.DELETE("/:id/backends/:backend_id", h.DeleteBackend)
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
			"owned_by": "modelgate",
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
		Description: req.Description,
		Enabled:     req.Enabled,
	}

	if err := h.store.Create(model); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 创建关联的 backends
	for _, backendInput := range req.Backends {
		if backendInput.ID == "" {
			continue
		}
		backend := &models.Backend{
			ID:        backendInput.ID,
			ModelID:   model.ID,
			Name:      backendInput.Name,
			BaseURL:   backendInput.BaseURL,
			ModelName: backendInput.ModelName,
			Weight:    backendInput.Weight,
			Region:    backendInput.Region,
			Enabled:   backendInput.Enabled,
		}
		if backend.Weight == 0 {
			backend.Weight = 1
		}
		if err := h.backendStore.Create(backend); err != nil {
			// 记录错误但不阻止模型创建
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backend: " + err.Error()})
			return
		}
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
	if req.Enabled != nil {
		model.Enabled = *req.Enabled
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

// GetModelBackends 获取指定模型的后端列表
func (h *Handler) GetModelBackends(c *gin.Context) {
	id := c.Param("id")

	// 检查模型是否存在
	model, err := h.store.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if model == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	// 从数据库获取后端列表
	backends, err := h.backendStore.ListByModel(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": backends})
}

// CreateBackend 为模型创建后端
func (h *Handler) CreateBackend(c *gin.Context) {
	modelID := c.Param("id")

	// 检查模型是否存在
	model, err := h.store.GetByID(modelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if model == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var req models.BackendCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	backend := &models.Backend{
		ID:        req.ID,
		ModelID:   modelID,
		Name:      req.Name,
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		ModelName: req.ModelName,
		Weight:    req.Weight,
		Region:    req.Region,
		Enabled:   req.Enabled,
	}

	if backend.Weight == 0 {
		backend.Weight = 1
	}

	if err := h.backendStore.Create(backend); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": backend})
}

// UpdateBackend 更新后端
func (h *Handler) UpdateBackend(c *gin.Context) {
	backendID := c.Param("backend_id")

	backend, err := h.backendStore.GetByID(backendID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if backend == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backend not found"})
		return
	}

	var req models.BackendUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		backend.Name = req.Name
	}
	if req.BaseURL != "" {
		backend.BaseURL = req.BaseURL
	}
	if req.APIKey != "" {
		backend.APIKey = req.APIKey
	}
	if req.ModelName != "" {
		backend.ModelName = req.ModelName
	}
	if req.Weight > 0 {
		backend.Weight = req.Weight
	}
	if req.Region != "" {
		backend.Region = req.Region
	}
	if req.Enabled != nil {
		backend.Enabled = *req.Enabled
	}

	if err := h.backendStore.Update(backend); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": backend})
}

// DeleteBackend 删除后端
func (h *Handler) DeleteBackend(c *gin.Context) {
	backendID := c.Param("backend_id")

	if err := h.backendStore.Delete(backendID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backend deleted"})
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
	if req.RequestQuotaDaily > 0 {
		policy.RequestQuotaDaily = req.RequestQuotaDaily
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

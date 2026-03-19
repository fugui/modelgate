package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/auth"
	"modelgate/internal/entity"
	"modelgate/internal/middleware"
	"modelgate/internal/proxy"
)

type LoadBalancer interface {
	GetHealthStatus() map[string]proxy.BackendHealth
	GetModelBackends(modelID string) []proxy.BackendHealth
	String() string
}

type ModelHandler struct {
	store        *entity.ModelStore
	backendStore *entity.BackendStore
	userStore    *entity.UserStore
	loadBalancer LoadBalancer
}

func NewModelHandler(store *entity.ModelStore, backendStore *entity.BackendStore, lb LoadBalancer, userStore *entity.UserStore) *ModelHandler {
	return &ModelHandler{
		store:        store,
		backendStore: backendStore,
		loadBalancer: lb,
		userStore:    userStore,
	}
}

func (h *ModelHandler) RegisterRoutes(r *gin.RouterGroup, jwtManager *auth.JWTManager) {
	// Health and status endpoints (require auth)
	auth := r.Group("")
	auth.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, h.userStore))
	{
		auth.GET("/admin/models/health", h.GetHealthStatus)
		auth.GET("/admin/loadbalancer/status", h.GetLoadBalancerStatus)
	}

	// Model CRUD endpoints (require admin)
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

func (h *ModelHandler) List(c *gin.Context) {
	models, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": models})
}

func (h *ModelHandler) Create(c *gin.Context) {
	var req entity.ModelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model := &entity.Model{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Enabled:     req.Enabled,
	}

	if err := h.store.Create(model); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create associated backends
	for _, backendInput := range req.Backends {
		if backendInput.ID == "" {
			continue
		}
		backend := &entity.Backend{
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backend: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": model})
}

func (h *ModelHandler) Update(c *gin.Context) {
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

	var req entity.ModelUpdateRequest
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

func (h *ModelHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "model deleted"})
}

func (h *ModelHandler) GetHealthStatus(c *gin.Context) {
	if h.loadBalancer == nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	status := h.loadBalancer.GetHealthStatus()
	c.JSON(http.StatusOK, gin.H{"data": status})
}

func (h *ModelHandler) GetModelBackends(c *gin.Context) {
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

	backends, err := h.backendStore.ListByModel(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": backends})
}

func (h *ModelHandler) CreateBackend(c *gin.Context) {
	modelID := c.Param("id")

	model, err := h.store.GetByID(modelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if model == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
		return
	}

	var req entity.BackendCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	backend := &entity.Backend{
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

func (h *ModelHandler) UpdateBackend(c *gin.Context) {
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

	var req entity.BackendUpdateRequest
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

func (h *ModelHandler) DeleteBackend(c *gin.Context) {
	backendID := c.Param("backend_id")

	if err := h.backendStore.Delete(backendID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "backend deleted"})
}

func (h *ModelHandler) GetLoadBalancerStatus(c *gin.Context) {
	if h.loadBalancer == nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "load balancer not initialized",
		})
		return
	}

	healthStatus := h.loadBalancer.GetHealthStatus()
	modelStats := make(map[string]interface{})

	c.JSON(http.StatusOK, gin.H{
		"load_balancer": h.loadBalancer.String(),
		"health_status": healthStatus,
		"models":        modelStats,
	})
}

// PolicyHandler handles admin quota policy endpoints
type PolicyHandler struct {
	quotaStore *entity.QuotaStore
	userStore  *entity.UserStore
}

func NewPolicyHandler(quotaStore *entity.QuotaStore, userStore *entity.UserStore) *PolicyHandler {
	return &PolicyHandler{quotaStore: quotaStore, userStore: userStore}
}

func (h *PolicyHandler) RegisterRoutes(r *gin.RouterGroup, jwtManager *auth.JWTManager) {
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

func (h *PolicyHandler) ListPolicies(c *gin.Context) {
	policies, err := h.quotaStore.ListPolicies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": policies})
}

func (h *PolicyHandler) CreatePolicy(c *gin.Context) {
	var policy entity.QuotaPolicy
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

func (h *PolicyHandler) UpdatePolicy(c *gin.Context) {
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

	var req entity.QuotaPolicy
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
	
	// Allow updating default model (including clearing it if req.DefaultModel is empty)
	policy.DefaultModel = req.DefaultModel

	// 始终同步 available_time_ranges（包括清空）
	policy.AvailableTimeRanges = req.AvailableTimeRanges

	if err := h.quotaStore.CreateOrUpdatePolicy(policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": policy})
}

func (h *PolicyHandler) DeletePolicy(c *gin.Context) {
	name := c.Param("name")

	if err := h.quotaStore.DeletePolicy(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "policy deleted"})
}

// Ensure uuid is used
var _ = uuid.UUID{}

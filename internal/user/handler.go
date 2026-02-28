package user

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"llmgate/internal/auth"
	"llmgate/internal/middleware"
	"llmgate/internal/models"
)

type QuotaService interface {
	GetQuotaStats(userID uuid.UUID) (map[string]interface{}, error)
}

type Handler struct {
	store          *models.UserStore
	jwtManager     *auth.JWTManager
	quotaService   QuotaService
	feedbackURL    string
	devManualURL   string
}

func NewHandler(store *models.UserStore, jwtManager *auth.JWTManager, quotaService QuotaService, feedbackURL, devManualURL string) *Handler {
	return &Handler{
		store:        store,
		jwtManager:   jwtManager,
		quotaService: quotaService,
		feedbackURL:  feedbackURL,
		devManualURL: devManualURL,
	}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// 公开接口
	r.POST("/auth/login", h.Login)
	r.POST("/auth/register", h.Register)
	r.GET("/config/frontend", h.GetFrontendConfig)

	// 需要认证的接口
	auth := r.Group("")
	auth.Use(middleware.AuthMiddlewareWithUserValidation(h.jwtManager, h.store))
	{
		auth.GET("/user/profile", h.Profile)
		auth.GET("/user/quota", h.GetQuota)
		auth.GET("/user/usage", h.GetUsage)
	}

	// 管理员接口
	admin := r.Group("/admin/users")
	admin.Use(middleware.AuthMiddlewareWithUserValidation(h.jwtManager, h.store))
	admin.Use(middleware.AdminRequired())
	{
		admin.GET("", h.List)
		admin.POST("", h.Create)
		admin.PUT("/:id", h.Update)
		admin.DELETE("/:id", h.Delete)
	}
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.store.GetByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if user == nil || !auth.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !user.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "account disabled"})
		return
	}

	// 更新最后登录时间
	_ = h.store.UpdateLastLogin(user.ID)

	token, err := h.jwtManager.Generate(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"token": token,
			"user":  user.ToResponse(),
		},
	})
}

func (h *Handler) Register(c *gin.Context) {
	var req models.UserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查邮箱是否已存在
	existing, err := h.store.GetByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}

	// 默认角色为 user
	if req.Role == "" {
		req.Role = models.RoleUser
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		Role:         req.Role,
		Department:   req.Department,
		QuotaPolicy:  req.QuotaPolicy,
		Models:       req.Models,
		Enabled:      true,
	}

	if user.QuotaPolicy == "" {
		user.QuotaPolicy = "default"
	}

	if err := h.store.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	token, err := h.jwtManager.Generate(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"token": token,
			"user":  user.ToResponse(),
		},
	})
}

func (h *Handler) Profile(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	u, err := h.store.GetByID(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if u == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": u.ToResponse()})
}

func (h *Handler) List(c *gin.Context) {
	// 支持分页
	limit := 100
	offset := 0

	users, err := h.store.List(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var responses []models.UserResponse
	for _, u := range users {
		responses = append(responses, u.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{"data": responses})
}

func (h *Handler) Create(c *gin.Context) {
	var req models.UserCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查邮箱是否已存在
	existing, err := h.store.GetByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}

	if req.Role == "" {
		req.Role = models.RoleUser
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		Role:         req.Role,
		Department:   req.Department,
		QuotaPolicy:  req.QuotaPolicy,
		Models:       req.Models,
		Enabled:      true,
	}

	if user.QuotaPolicy == "" {
		user.QuotaPolicy = "default"
	}

	if err := h.store.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": user.ToResponse()})
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	user, err := h.store.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req models.UserUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Department != "" {
		user.Department = req.Department
	}
	if req.QuotaPolicy != "" {
		user.QuotaPolicy = req.QuotaPolicy
	}
	if req.Models != nil {
		user.Models = req.Models
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}

	if err := h.store.Update(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user.ToResponse()})
}

func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func (h *Handler) GetQuota(c *gin.Context) {
	user := middleware.GetCurrentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	stats, err := h.quotaService.GetQuotaStats(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (h *Handler) GetUsage(c *gin.Context) {
	// TODO: 实现使用记录查询
	c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
}

func (h *Handler) GetFrontendConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"feedback_url":  h.feedbackURL,
			"dev_manual_url": h.devManualURL,
		},
	})
}

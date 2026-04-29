package dashboard

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler 仪表板HTTP处理器
type Handler struct {
	service *Service
}

// NewHandler 创建仪表板处理器
func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

// RegisterRoutes 注册仪表板路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/stats", h.GetDashboardStats)
	r.GET("/top-users", h.GetTopUsers)
	r.GET("/top-users-7d", h.GetTopUsers7Days)
	r.GET("/hourly", h.GetHourlyStats)
	r.GET("/departments", h.GetDepartmentStats)
	r.GET("/models", h.GetModelStats)
	r.GET("/metrics", h.GetMetrics)
	r.GET("/backend-metrics", h.GetBackendMetrics)
}

// GetDashboardStats 获取系统概览数据
func (h *Handler) GetDashboardStats(c *gin.Context) {
	stats, err := h.service.GetDashboardStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": stats,
	})
}

// GetTopUsers 获取今日TOP10用户
func (h *Handler) GetTopUsers(c *gin.Context) {
	// 支持自定义limit，默认10
	limit := 10
	if l := c.Query("limit"); l != "" {
		// 可以添加limit解析，目前使用默认值
	}

	users, err := h.service.GetTopUsers(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": users,
	})
}

// GetTopUsers7Days 获取最近7天TOP20用户
func (h *Handler) GetTopUsers7Days(c *gin.Context) {
	limit := 20
	// 简单实现支持自定义limit
	users, err := h.service.GetTopUsers7Days(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": users,
	})
}

// GetHourlyStats 获取最近24小时每小时请求数
func (h *Handler) GetHourlyStats(c *gin.Context) {
	stats := h.service.GetHourlyStats()
	c.JSON(http.StatusOK, gin.H{
		"data": stats,
	})
}

// GetDepartmentStats 获取部门使用统计
func (h *Handler) GetDepartmentStats(c *gin.Context) {
	stats, err := h.service.GetDepartmentStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": stats,
	})
}

// GetModelStats 获取模型使用分布
func (h *Handler) GetModelStats(c *gin.Context) {
	stats, err := h.service.GetModelStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": stats,
	})
}

// GetMetrics 获取最近24小时的并发数&时延指标
func (h *Handler) GetMetrics(c *gin.Context) {
	metrics := h.service.GetMetricsHistory()
	if metrics == nil {
		metrics = []MetricsSnapshot{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data": metrics,
	})
}

// GetBackendMetrics 获取按后端分组的5分钟级时延指标
func (h *Handler) GetBackendMetrics(c *gin.Context) {
	metrics := h.service.GetBackendMetricsHistory()
	if metrics == nil {
		metrics = make(map[string][]BackendMetricsSnapshot)
	}
	c.JSON(http.StatusOK, gin.H{
		"data": metrics,
	})
}

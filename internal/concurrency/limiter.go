// Package concurrency 提供并发请求限制功能
package concurrency

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Limiter 并发限制器
type Limiter struct {
	globalLimit int                   // 全局并发限制
	userLimit   int                   // 每个用户的并发限制
	globalSem   chan struct{}         // 全局信号量
	userSemMap  map[string]chan struct{} // 用户信号量映射
	mu          sync.RWMutex
}

// NewLimiter 创建新的并发限制器
// globalLimit: 全局最大并发数 (0 表示不限制)
// userLimit: 每个用户最大并发数 (0 表示不限制)
func NewLimiter(globalLimit, userLimit int) *Limiter {
	l := &Limiter{
		globalLimit: globalLimit,
		userLimit:   userLimit,
		userSemMap:  make(map[string]chan struct{}),
	}

	// 初始化全局信号量
	if globalLimit > 0 {
		l.globalSem = make(chan struct{}, globalLimit)
	}

	return l
}

// Acquire 尝试获取并发许可
// 返回 true 表示获取成功，false 表示被拒绝
func (l *Limiter) Acquire(userID string) bool {
	// 尝试获取全局许可
	if l.globalSem != nil {
		select {
		case l.globalSem <- struct{}{}:
			// 获取成功
		default:
			// 全局并发已满
			return false
		}
	}

	// 尝试获取用户许可
	if l.userLimit > 0 {
		userSem := l.getUserSemaphore(userID)
		select {
		case userSem <- struct{}{}:
			// 获取成功
		default:
			// 用户并发已满，释放全局许可
			if l.globalSem != nil {
				<-l.globalSem
			}
			return false
		}
	}

	return true
}

// Release 释放并发许可
func (l *Limiter) Release(userID string) {
	// 释放用户许可
	if l.userLimit > 0 {
		userSem := l.getUserSemaphore(userID)
		select {
		case <-userSem:
		default:
			// 信号量为空，忽略
		}
	}

	// 释放全局许可
	if l.globalSem != nil {
		select {
		case <-l.globalSem:
		default:
			// 信号量为空，忽略
		}
	}
}

// getUserSemaphore 获取或创建用户的信号量
func (l *Limiter) getUserSemaphore(userID string) chan struct{} {
	l.mu.RLock()
	sem, exists := l.userSemMap[userID]
	l.mu.RUnlock()

	if exists {
		return sem
	}

	// 创建新的信号量
	l.mu.Lock()
	defer l.mu.Unlock()

	// 双重检查
	if sem, exists := l.userSemMap[userID]; exists {
		return sem
	}

	sem = make(chan struct{}, l.userLimit)
	l.userSemMap[userID] = sem
	return sem
}

// GetStats 获取当前并发统计
func (l *Limiter) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	globalCurrent := 0
	if l.globalSem != nil {
		globalCurrent = len(l.globalSem)
	}

	userStats := make(map[string]int)
	for userID, sem := range l.userSemMap {
		count := len(sem)
		if count > 0 {
			userStats[userID] = count
		}
	}

	return map[string]interface{}{
		"global_limit":   l.globalLimit,
		"global_current": globalCurrent,
		"user_limit":     l.userLimit,
		"active_users":   len(userStats),
		"user_stats":     userStats,
	}
}

// Middleware 创建 Gin 中间件
// 注意：这个中间件需要在认证中间件之后使用，因为需要 userID
func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只限制 LLM 代理接口
		if c.Request.URL.Path != "/v1/chat/completions" {
			c.Next()
			return
		}

		// 从上下文中获取用户ID
		userID := getUserIDFromContext(c)
		if userID == "" {
			// 没有用户ID，可能是未认证的请求
			c.Next()
			return
		}

		// 尝试获取许可
		if !l.Acquire(userID) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "concurrency limit exceeded",
				"message": "too many concurrent requests, please try again later",
			})
			return
		}

		// 确保在请求结束后释放
		defer l.Release(userID)

		c.Next()
	}
}

// getUserIDFromContext 从 Gin 上下文中获取用户ID
func getUserIDFromContext(c *gin.Context) string {
	// 尝试从 context 中获取 user_id（由认证中间件设置）
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uuid.UUID); ok {
			return id.String()
		}
		if id, ok := userID.(string); ok {
			return id
		}
	}

	// 尝试获取 currentUser（JWT claims）
	if claims, exists := c.Get("currentUser"); exists {
		if c, ok := claims.(*struct{ UserID uuid.UUID }); ok {
			return c.UserID.String()
		}
	}

	return ""
}

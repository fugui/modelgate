package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"modelgate/internal/infra/concurrency"
)

// ConcurrencyLimitMiddleware 创建通用的并发限制中间件
// 从 context 中获取 user_id，调用 Limiter 的 Acquire/Release
// 适用于所有需要并发限制的代理端点（OpenAI、Anthropic 等）
func ConcurrencyLimitMiddleware(limiter *concurrency.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter == nil {
			c.Next()
			return
		}

		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		var uidStr string
		switch v := userID.(type) {
		case uuid.UUID:
			uidStr = v.String()
		case string:
			uidStr = v
		default:
			c.Next()
			return
		}

		if !limiter.Acquire(uidStr) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "concurrency limit exceeded",
				"message": "too many concurrent requests, please try again later",
			})
			return
		}

		defer limiter.Release(uidStr)

		c.Next()
	}
}

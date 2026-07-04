package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"modelgate/internal/config"
)

// ClientFilterMiddleware 根据 User-Agent 封禁特定客户端类型。
// 规则从 ConfigManager 动态读取，支持热重载。
// 中间件应在 ProtocolInjectionMiddleware 之后、ProxyAuthMiddleware 之前注册，
// 以便使用协议适配的错误格式。
func ClientFilterMiddleware(cm *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		ua := c.Request.UserAgent()
		if ua == "" {
			c.Next()
			return
		}

		cfg := cm.GetConfig()
		uaLower := strings.ToLower(ua)

		for _, rule := range cfg.ClientFilter.Rules {
			if !rule.Enabled || rule.Pattern == "" {
				continue
			}
			if strings.Contains(uaLower, strings.ToLower(rule.Pattern)) {
				// 尝试从上下文获取协议适配器以生成统一格式的错误响应
				if proto, exists := c.Get(ContextKeyProtocol); exists {
					if builder, ok := proto.(ErrorResponseBuilder); ok {
						c.Data(http.StatusForbidden, "application/json",
							builder.BuildErrorResponse("permission_denied",
								"client type '"+rule.Name+"' is not allowed to access this service"))
						c.Abort()
						return
					}
				}
				// 回退：直接返回 JSON
				c.JSON(http.StatusForbidden, gin.H{
					"error": gin.H{
						"type":    "permission_denied",
						"message": "client type '" + rule.Name + "' is not allowed to access this service",
					},
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"modelgate/internal/auth"
	"modelgate/internal/entity"
)

const ContextKeyUser = "currentUser"

// AuthMiddleware JWT 认证中间件（仅验证 token 签名和过期时间）
func AuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		claims, err := jwtManager.Validate(parts[1])
		if err != nil {
			if err == auth.ErrExpiredToken {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set(ContextKeyUser, claims)
		c.Next()
	}
}

// AuthMiddlewareWithUserValidation JWT 认证中间件（同时验证用户存在于数据库，并注入完整用户信息）
func AuthMiddlewareWithUserValidation(jwtManager *auth.JWTManager, userStore *entity.UserStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		claims, err := jwtManager.Validate(parts[1])
		if err != nil {
			if err == auth.ErrExpiredToken {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// 验证用户是否存在于数据库且未禁用
		user, err := userStore.GetByID(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to verify user"})
			return
		}
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if !user.Enabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user disabled"})
			return
		}

		c.Set(ContextKeyUser, claims)
		// 注入完整的 User 对象供后续 Handler 使用
		c.Set("fullUser", user)
		c.Next()
	}
}

// GetCurrentUser 从上下文中获取当前用户的 Token Claims
func GetCurrentUser(c *gin.Context) *auth.Claims {
	user, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil
	}
	return user.(*auth.Claims)
}

// GetCurrentFullUser 从上下文中获取当前完整的 User 实体
func GetCurrentFullUser(c *gin.Context) *entity.User {
	user, exists := c.Get("fullUser")
	if !exists {
		return nil
	}
	return user.(*entity.User)
}

// AdminRequired 管理员权限检查
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetCurrentUser(c)
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		if user.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}

		c.Next()
	}
}

// ManagerOrAdminRequired 管理员或经理权限检查
func ManagerOrAdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetCurrentUser(c)
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		if user.Role != "admin" && user.Role != "manager" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "manager or admin access required"})
			return
		}

		c.Next()
	}
}

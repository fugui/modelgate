// Package cache 提供简单的本地内存缓存
package cache

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"modelgate/internal/entity"
)

// Cache 简单的内存缓存
type Cache struct {
	mu       sync.RWMutex
	apiKeys  map[string]*APIKeyCacheItem  // key: key_prefix
	users    map[string]*UserCacheItem    // key: user_id
	quit     chan struct{}
}

// APIKeyCacheItem API Key 缓存项
type APIKeyCacheItem struct {
	Key      *entity.APIKey  // 完整的 API Key 对象
	UserInfo *entity.User    // 嵌入用户信息，减少二次查询
	CachedAt time.Time
}

// UserCacheItem 用户缓存项
type UserCacheItem struct {
	User     *entity.User
	CachedAt time.Time
}

// New 创建缓存实例
func New() *Cache {
	c := &Cache{
		apiKeys: make(map[string]*APIKeyCacheItem),
		users:   make(map[string]*UserCacheItem),
		quit:    make(chan struct{}),
	}
	
	// 启动过期清理任务
	go c.cleanupLoop()
	
	return c
}

// Stop 停止缓存清理任务
func (c *Cache) Stop() {
	close(c.quit)
}

// GetAPIKey 从缓存获取 API Key
func (c *Cache) GetAPIKey(keyPrefix string) *APIKeyCacheItem {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	item, exists := c.apiKeys[keyPrefix]
	if !exists {
		return nil
	}
	
	// 检查是否过期（5分钟）
	if time.Since(item.CachedAt) > 5*time.Minute {
		return nil
	}
	
	// 检查 API Key 是否过期
	if item.Key.ExpiresAt != nil && item.Key.ExpiresAt.Before(time.Now()) {
		return nil
	}
	
	return item
}

// SetAPIKey 缓存 API Key
func (c *Cache) SetAPIKey(keyPrefix string, key *entity.APIKey, user *entity.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.apiKeys[keyPrefix] = &APIKeyCacheItem{
		Key:      key,
		UserInfo: user,
		CachedAt: time.Now(),
	}
}

// DeleteAPIKey 删除 API Key 缓存
func (c *Cache) DeleteAPIKey(keyPrefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.apiKeys, keyPrefix)
}

// DeleteAPIKeysByUser 删除用户的所有 API Key 缓存
func (c *Cache) DeleteAPIKeysByUser(userID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for prefix, item := range c.apiKeys {
		if item.Key.UserID == userID {
			delete(c.apiKeys, prefix)
		}
	}
}

// GetUser 从缓存获取用户
func (c *Cache) GetUser(userID string) *entity.User {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	item, exists := c.users[userID]
	if !exists {
		return nil
	}
	
	// 检查是否过期（5分钟）
	if time.Since(item.CachedAt) > 5*time.Minute {
		return nil
	}
	
	return item.User
}

// SetUser 缓存用户
func (c *Cache) SetUser(userID string, user *entity.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.users[userID] = &UserCacheItem{
		User:     user,
		CachedAt: time.Now(),
	}
}

// DeleteUser 删除用户缓存
func (c *Cache) DeleteUser(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.users, userID)
}

// cleanupLoop 定期清理过期缓存
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.quit:
			return
		}
	}
}

// cleanup 清理过期项
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	
	// 清理 API Key 缓存
	for prefix, item := range c.apiKeys {
		if now.Sub(item.CachedAt) > 5*time.Minute {
			delete(c.apiKeys, prefix)
		}
	}
	
	// 清理用户缓存
	for id, item := range c.users {
		if now.Sub(item.CachedAt) > 5*time.Minute {
			delete(c.users, id)
		}
	}
}

// Stats 获取缓存统计（用于调试）
func (c *Cache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return map[string]interface{}{
		"api_keys_cached": len(c.apiKeys),
		"users_cached":    len(c.users),
	}
}

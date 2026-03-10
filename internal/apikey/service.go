// Package apikey 提供 API Key 的生成、验证和管理功能
package apikey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"modelgate/internal/cache"
	"modelgate/internal/entity"
	"modelgate/internal/logger"
)

const (
	keyPrefix = "llm-"   // API Key 前缀，用于识别
	keyLength = 32       // 随机部分长度（字节）
	prefixLen = 12       // 前缀显示长度（含 "llm-")
)

// Service 提供 API Key 业务逻辑
type Service struct {
	store     *entity.APIKeyStore
	userStore *entity.UserStore
	cache     *cache.Cache
}

// NewService 创建 API Key 服务实例
func NewService(store *entity.APIKeyStore, userStore *entity.UserStore, c *cache.Cache) *Service {
	return &Service{
		store:     store,
		userStore: userStore,
		cache:     c,
	}
}

// GenerateKey 为用户生成新的 API Key
// 返回包含明文的 API Key（仅创建时可获取）
func (s *Service) GenerateKey(userID uuid.UUID, req *entity.APIKeyCreateRequest) (*entity.APIKeyWithSecret, error) {
	// 生成随机 key
	randomBytes := make([]byte, keyLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	plainKey := keyPrefix + hex.EncodeToString(randomBytes)
	keyHash, err := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash key: %w", err)
	}

	key := &entity.APIKey{
		UserID:    userID,
		Name:      req.Name,
		KeyHash:   string(keyHash),
		KeyPrefix: plainKey[:prefixLen],
		Enabled:   true,
		ExpiresAt: req.ExpiresAt,
	}

	if err := s.store.Create(key); err != nil {
		return nil, fmt.Errorf("failed to create key: %w", err)
	}

	return &entity.APIKeyWithSecret{
		APIKeyResponse: key.ToResponse(),
		Key:            plainKey,
	}, nil
}

// ValidateKey 验证 API Key 的有效性
// 检查：格式、hash、过期时间、启用状态、所属用户状态
// 返回验证通过的 API Key 和用户信息
func (s *Service) ValidateKey(plainKey string) (*entity.APIKey, *entity.User, error) {
	if !strings.HasPrefix(plainKey, keyPrefix) {
		return nil, nil, fmt.Errorf("invalid key format")
	}

	if len(plainKey) < prefixLen {
		return nil, nil, fmt.Errorf("invalid key format")
	}

	keyPrefixStr := plainKey[:prefixLen]

	// 1. 尝试从缓存获取
	if cached := s.cache.GetAPIKey(keyPrefixStr); cached != nil {
		// 验证 hash
		if err := bcrypt.CompareHashAndPassword([]byte(cached.Key.KeyHash), []byte(plainKey)); err != nil {
			return nil, nil, fmt.Errorf("invalid key")
		}
		// 检查是否过期（缓存命中也要检查）
		if cached.Key.ExpiresAt != nil && cached.Key.ExpiresAt.Before(time.Now()) {
			s.cache.DeleteAPIKey(keyPrefixStr) // 清除已过期的缓存
			return nil, nil, fmt.Errorf("key expired")
		}
		// 检查是否启用
		if !cached.Key.Enabled {
			return nil, nil, fmt.Errorf("key disabled")
		}
		// 检查用户是否启用（重新查询 DB 以获取最新状态）
		user, err := s.userStore.GetByID(cached.Key.UserID)
		if err != nil {
			// DB 查询失败，不能确定用户状态，使用缓存信息降级
			if !cached.UserInfo.Enabled {
				return nil, nil, fmt.Errorf("user disabled")
			}
			go s.updateLastUsed(cached.Key.ID)
			return cached.Key, cached.UserInfo, nil
		}
		if user == nil || !user.Enabled {
			s.cache.DeleteAPIKey(keyPrefixStr) // 用户已被禁用，清除缓存
			return nil, nil, fmt.Errorf("user disabled")
		}
		// 异步更新最后使用时间
		go s.updateLastUsed(cached.Key.ID)
		return cached.Key, user, nil
	}

	// 2. 缓存未命中，从数据库查找
	key, err := s.store.GetByKeyPrefix(keyPrefixStr)
	if err != nil {
		return nil, nil, err
	}
	if key == nil {
		return nil, nil, fmt.Errorf("invalid key")
	}

	// 验证 hash
	if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(plainKey)); err != nil {
		return nil, nil, fmt.Errorf("invalid key")
	}

	// 检查是否过期
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return nil, nil, fmt.Errorf("key expired")
	}

	// 检查是否启用
	if !key.Enabled {
		return nil, nil, fmt.Errorf("key disabled")
	}

	// 检查所属用户状态
	user, err := s.userStore.GetByID(key.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, nil, fmt.Errorf("user not found")
	}
	if !user.Enabled {
		return nil, nil, fmt.Errorf("user disabled")
	}

	// 3. 写入缓存
	s.cache.SetAPIKey(keyPrefixStr, key, user)

	// 4. 异步更新最后使用时间（不阻塞请求）
	go s.updateLastUsed(key.ID)

	return key, user, nil
}

// updateLastUsed 异步更新 API Key 最后使用时间
func (s *Service) updateLastUsed(keyID uuid.UUID) {
	// 使用独立的 goroutine，设置超时避免资源泄漏
	done := make(chan struct{})
	go func() {
		if err := s.store.UpdateLastUsed(keyID); err != nil {
			// 记录错误但不影响主流程
			logger.Warnw("Failed to update API key last_used_at", "key_id", keyID, "error", err)
		}
		close(done)
	}()

	// 等待最多 100ms，超时则放弃
	select {
	case <-done:
		// 成功更新
	case <-time.After(100 * time.Millisecond):
		// 超时，不阻塞
	}
}

// GetUserKeys 获取指定用户的所有 API Key
func (s *Service) GetUserKeys(userID uuid.UUID) ([]*entity.APIKey, error) {
	return s.store.ListByUser(userID)
}

// DeleteKey 删除用户自己的 API Key
func (s *Service) DeleteKey(keyID uuid.UUID, userID uuid.UUID) error {
	key, err := s.store.GetByID(keyID)
	if err != nil {
		return err
	}
	if key == nil || key.UserID != userID {
		return fmt.Errorf("key not found")
	}

	// 删除缓存
	s.cache.DeleteAPIKey(key.KeyPrefix)

	return s.store.Delete(keyID)
}

// DeleteKeyAdmin 管理员删除任意 Key
func (s *Service) DeleteKeyAdmin(keyID uuid.UUID) error {
	// 先查询 key 获取 prefix
	key, err := s.store.GetByID(keyID)
	if err != nil {
		return err
	}
	if key != nil {
		s.cache.DeleteAPIKey(key.KeyPrefix)
	}

	return s.store.Delete(keyID)
}

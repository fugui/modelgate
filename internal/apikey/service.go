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
	"llmgate/internal/models"
)

const (
	keyPrefix = "llm-"   // API Key 前缀，用于识别
	keyLength = 32       // 随机部分长度（字节）
	prefixLen = 12       // 前缀显示长度（含 "llm-"）
)

// Service 提供 API Key 业务逻辑
type Service struct {
	store     *models.APIKeyStore
	userStore *models.UserStore
}

// NewService 创建 API Key 服务实例
func NewService(store *models.APIKeyStore, userStore *models.UserStore) *Service {
	return &Service{
		store:     store,
		userStore: userStore,
	}
}

// GenerateKey 为用户生成新的 API Key
// 返回包含明文的 API Key（仅创建时可获取）
func (s *Service) GenerateKey(userID uuid.UUID, req *models.APIKeyCreateRequest) (*models.APIKeyWithSecret, error) {
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

	key := &models.APIKey{
		UserID:          userID,
		Name:            req.Name,
		KeyHash:         string(keyHash),
		KeyPrefix:       plainKey[:prefixLen],
		Models:          req.Models,
		RateLimit:       req.RateLimit,
		RateLimitWindow: req.RateLimitWindow,
		Enabled:         true,
		ExpiresAt:       req.ExpiresAt,
	}

	if err := s.store.Create(key); err != nil {
		return nil, fmt.Errorf("failed to create key: %w", err)
	}

	return &models.APIKeyWithSecret{
		APIKeyResponse: key.ToResponse(),
		Key:            plainKey,
	}, nil
}

// ValidateKey 验证 API Key 的有效性
// 检查：格式、hash、过期时间、启用状态、所属用户状态
// 返回验证通过的 API Key 和用户信息
func (s *Service) ValidateKey(plainKey string) (*models.APIKey, *models.User, error) {
	if !strings.HasPrefix(plainKey, keyPrefix) {
		return nil, nil, fmt.Errorf("invalid key format")
	}

	// 从数据库查找匹配的 key
	key, err := s.store.GetByKeyPrefix(plainKey[:prefixLen])
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

	return key, user, nil
}

// GetUserKeys 获取指定用户的所有 API Key
func (s *Service) GetUserKeys(userID uuid.UUID) ([]*models.APIKey, error) {
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

	return s.store.Delete(keyID)
}

// DeleteKeyAdmin 管理员删除任意 Key
func (s *Service) DeleteKeyAdmin(keyID uuid.UUID) error {
	return s.store.Delete(keyID)
}

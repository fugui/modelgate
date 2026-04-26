package entity

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	Name            string     `json:"name"`
	KeyHash         string     `json:"-"`
	KeyPrefix       string     `json:"key_prefix"`
	Enabled         bool       `json:"enabled"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	TotalTokensUsed int        `json:"total_tokens_used"`
}

type APIKeyCreateRequest struct {
	Name      string     `json:"name" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type APIKeyResponse struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	Name            string     `json:"name"`
	KeyPrefix       string     `json:"key_prefix"`
	Enabled         bool       `json:"enabled"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	TotalTokensUsed int        `json:"total_tokens_used"`
}

type APIKeyWithSecret struct {
	APIKeyResponse
	Key string `json:"key"` // 仅创建时返回一次
}

func (k *APIKey) ToResponse() APIKeyResponse {
	return APIKeyResponse{
		ID:              k.ID,
		UserID:          k.UserID,
		Name:            k.Name,
		KeyPrefix:       k.KeyPrefix,
		Enabled:         k.Enabled,
		ExpiresAt:       k.ExpiresAt,
		LastUsedAt:      k.LastUsedAt,
		CreatedAt:       k.CreatedAt,
		TotalTokensUsed: k.TotalTokensUsed,
	}
}

// APIKeyStore API Key 数据访问层
type APIKeyStore struct {
	db *sql.DB
}

func NewAPIKeyStore(db *sql.DB) *APIKeyStore {
	return &APIKeyStore{db: db}
}

func scanAPIKey(s scanner) (*APIKey, error) {
	key := &APIKey{}
	err := s.Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyHash, &key.KeyPrefix,
		&key.Enabled, &key.ExpiresAt, &key.LastUsedAt, &key.CreatedAt, &key.UpdatedAt,
		&key.TotalTokensUsed,
	)
	return key, err
}

func (s *APIKeyStore) Create(key *APIKey) error {
	key.ID = uuid.New()
	query := `
		INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix, enabled, expires_at, total_tokens_used)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		RETURNING created_at, updated_at`

	return s.db.QueryRow(query,
		key.ID.String(), key.UserID.String(), key.Name, key.KeyHash, key.KeyPrefix,
		key.Enabled, key.ExpiresAt,
	).Scan(&key.CreatedAt, &key.UpdatedAt)
}

func (s *APIKeyStore) GetByID(id uuid.UUID) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_hash, key_prefix,
		       enabled, expires_at, last_used_at, created_at, updated_at, total_tokens_used
		FROM api_keys WHERE id = ?`

	key, err := scanAPIKey(s.db.QueryRow(query, id.String()))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *APIKeyStore) GetByHash(hash string) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_hash, key_prefix,
		       enabled, expires_at, last_used_at, created_at, updated_at, total_tokens_used
		FROM api_keys WHERE key_hash = ? AND enabled = 1`

	key, err := scanAPIKey(s.db.QueryRow(query, hash))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *APIKeyStore) GetByKeyPrefix(prefix string) (*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_hash, key_prefix,
		       enabled, expires_at, last_used_at, created_at, updated_at, total_tokens_used
		FROM api_keys WHERE key_prefix = ? AND enabled = 1`

	key, err := scanAPIKey(s.db.QueryRow(query, prefix))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *APIKeyStore) ListByUser(userID uuid.UUID) ([]*APIKey, error) {
	query := `
		SELECT id, user_id, name, key_hash, key_prefix,
		       enabled, expires_at, last_used_at, created_at, updated_at, total_tokens_used
		FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`

	rows, err := s.db.Query(query, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (s *APIKeyStore) Update(key *APIKey) error {
	query := `
		UPDATE api_keys SET
			name = ?, enabled = ?, expires_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	_, err := s.db.Exec(query,
		key.Name, key.Enabled, key.ExpiresAt, key.ID.String(),
	)
	return err
}

func (s *APIKeyStore) UpdateLastUsed(id uuid.UUID) error {
	_, err := s.db.Exec("UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?", id.String())
	return err
}

func (s *APIKeyStore) AddTokensUsed(id uuid.UUID, tokens int) error {
	_, err := s.db.Exec("UPDATE api_keys SET total_tokens_used = total_tokens_used + ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", tokens, id.String())
	return err
}

func (s *APIKeyStore) Delete(id uuid.UUID) error {
	_, err := s.db.Exec("DELETE FROM api_keys WHERE id = ?", id.String())
	return err
}

func (s *APIKeyStore) CountByUser(userID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM api_keys WHERE user_id = ?", userID.String()).Scan(&count)
	return count, err
}

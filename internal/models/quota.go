package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type QuotaPolicy struct {
	Name            string      `json:"name"`
	RateLimit       int         `json:"rate_limit"`
	RateLimitWindow int         `json:"rate_limit_window"`
	TokenQuotaDaily int64       `json:"token_quota_daily"`
	Models          StringArray `json:"models"`
	Description     string      `json:"description"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type QuotaUsageDaily struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	Date         time.Time `json:"date"`
	ModelID      string    `json:"model_id"`
	RequestCount int       `json:"request_count"`
	TokenCount   int64     `json:"token_count"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
}

type QuotaCheckResult struct {
	Allowed       bool   `json:"allowed"`
	Reason        string `json:"reason,omitempty"`
	DailyTokens   int64  `json:"daily_tokens"`
	DailyLimit    int64  `json:"daily_limit"`
	RateRemaining int    `json:"rate_remaining"`
	RateLimit     int    `json:"rate_limit"`
}

type UsageStats struct {
	TotalRequests int   `json:"total_requests"`
	TotalTokens   int64 `json:"total_tokens"`
	InputTokens   int64 `json:"input_tokens"`
	OutputTokens  int64 `json:"output_tokens"`
	AvgLatencyMs  int   `json:"avg_latency_ms"`
	ErrorCount    int   `json:"error_count"`
}

// QuotaStore 配额数据访问层
type QuotaStore struct {
	db *sql.DB
}

func NewQuotaStore(db *sql.DB) *QuotaStore {
	return &QuotaStore{db: db}
}

func (s *QuotaStore) GetPolicy(name string) (*QuotaPolicy, error) {
	policy := &QuotaPolicy{}
	query := `
		SELECT name, rate_limit, rate_limit_window, token_quota_daily, models, description, created_at, updated_at
		FROM quota_policies WHERE name = ?`

	var modelsJSON string
	err := s.db.QueryRow(query, name).Scan(
		&policy.Name, &policy.RateLimit, &policy.RateLimitWindow,
		&policy.TokenQuotaDaily, &modelsJSON, &policy.Description,
		&policy.CreatedAt, &policy.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(modelsJSON), &policy.Models)
	return policy, nil
}

func (s *QuotaStore) ListPolicies() ([]*QuotaPolicy, error) {
	query := `
		SELECT name, rate_limit, rate_limit_window, token_quota_daily, models, description, created_at, updated_at
		FROM quota_policies ORDER BY name`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*QuotaPolicy
	for rows.Next() {
		policy := &QuotaPolicy{}
		var modelsJSON string
		err := rows.Scan(
			&policy.Name, &policy.RateLimit, &policy.RateLimitWindow,
			&policy.TokenQuotaDaily, &modelsJSON, &policy.Description,
			&policy.CreatedAt, &policy.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(modelsJSON), &policy.Models)
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (s *QuotaStore) CreateOrUpdatePolicy(policy *QuotaPolicy) error {
	query := `
		INSERT INTO quota_policies (name, rate_limit, rate_limit_window, token_quota_daily, models, description)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			rate_limit = excluded.rate_limit,
			rate_limit_window = excluded.rate_limit_window,
			token_quota_daily = excluded.token_quota_daily,
			models = excluded.models,
			description = excluded.description,
			updated_at = CURRENT_TIMESTAMP
		RETURNING created_at, updated_at`

	modelsJSON, _ := json.Marshal(policy.Models)
	return s.db.QueryRow(query,
		policy.Name, policy.RateLimit, policy.RateLimitWindow,
		policy.TokenQuotaDaily, string(modelsJSON), policy.Description,
	).Scan(&policy.CreatedAt, &policy.UpdatedAt)
}

func (s *QuotaStore) DeletePolicy(name string) error {
	_, err := s.db.Exec("DELETE FROM quota_policies WHERE name = ?", name)
	return err
}

// GetDailyUsage 获取用户当天的 Token 使用量
func (s *QuotaStore) GetDailyUsage(userID uuid.UUID, date time.Time) (int64, error) {
	var total int64
	query := `
		SELECT COALESCE(SUM(token_count), 0)
		FROM quota_usage_daily
		WHERE user_id = ? AND date = ?`

	err := s.db.QueryRow(query, userID.String(), date.Format("2006-01-02")).Scan(&total)
	return total, err
}

// IncrementUsage 增加使用统计
func (s *QuotaStore) IncrementUsage(userID uuid.UUID, modelID string, inputTokens, outputTokens int) error {
	query := `
		INSERT INTO quota_usage_daily (id, user_id, date, model_id, request_count, token_count, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(user_id, date, model_id) DO UPDATE SET
			request_count = request_count + 1,
			token_count = token_count + excluded.token_count,
			input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens`

	id := uuid.New().String()
	totalTokens := inputTokens + outputTokens
	_, err := s.db.Exec(query, id, userID.String(), time.Now().Format("2006-01-02"), modelID, totalTokens, inputTokens, outputTokens)
	return err
}

// GetUsageStats 获取使用统计
func (s *QuotaStore) GetUsageStats(userID uuid.UUID, startDate, endDate time.Time) (*UsageStats, error) {
	stats := &UsageStats{}
	query := `
		SELECT
			COALESCE(SUM(request_count), 0),
			COALESCE(SUM(token_count), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0)
		FROM quota_usage_daily
		WHERE user_id = ? AND date BETWEEN ? AND ?`

	err := s.db.QueryRow(query, userID.String(), startDate.Format("2006-01-02"), endDate.Format("2006-01-02")).Scan(
		&stats.TotalRequests, &stats.TotalTokens, &stats.InputTokens, &stats.OutputTokens,
	)
	return stats, err
}
